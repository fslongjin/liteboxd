package liteboxd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/gorilla/websocket"
)

// ExecSession represents an interactive exec session over WebSocket.
// Implements io.Reader (stdout) and io.Writer (stdin).
type ExecSession struct {
	ws *websocket.Conn

	// Read buffering
	readBuf bytes.Buffer
	readMu  sync.Mutex
	readCh  chan struct{} // signals new data available

	// Exit state
	exitCode int
	exitCh   chan struct{}
	exitOnce sync.Once

	// Close state
	closeCh   chan struct{}
	closeOnce sync.Once
}

func newExecSession(ws *websocket.Conn) *ExecSession {
	s := &ExecSession{
		ws:      ws,
		readCh:  make(chan struct{}, 1),
		exitCh:  make(chan struct{}),
		closeCh: make(chan struct{}),
	}
	go s.readLoop()
	return s
}

// readLoop reads WebSocket messages and dispatches them.
func (s *ExecSession) readLoop() {
	defer s.exitOnce.Do(func() { close(s.exitCh) })

	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		_, message, err := s.ws.ReadMessage()
		if err != nil {
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "output":
			s.readMu.Lock()
			s.readBuf.WriteString(msg.Data)
			s.readMu.Unlock()
			// Signal that data is available
			select {
			case s.readCh <- struct{}{}:
			default:
			}
		case "exit":
			s.exitCode = msg.ExitCode
			return
		case "error":
			s.exitCode = 1
			s.readMu.Lock()
			s.readBuf.WriteString(fmt.Sprintf("\r\nError: %s\r\n", msg.Message))
			s.readMu.Unlock()
			select {
			case s.readCh <- struct{}{}:
			default:
			}
			return
		}
	}
}

// Read reads stdout data from the session. Blocks until data is available or
// the session ends.
func (s *ExecSession) Read(p []byte) (int, error) {
	for {
		s.readMu.Lock()
		n, _ := s.readBuf.Read(p)
		s.readMu.Unlock()

		if n > 0 {
			return n, nil
		}

		// Wait for data or session end
		select {
		case <-s.readCh:
			// New data may be available, loop back to try reading
			continue
		case <-s.exitCh:
			// Session ended; drain any remaining data
			s.readMu.Lock()
			n, _ = s.readBuf.Read(p)
			s.readMu.Unlock()
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		case <-s.closeCh:
			return 0, io.EOF
		}
	}
}

// Write sends stdin data to the session.
func (s *ExecSession) Write(p []byte) (int, error) {
	msg, _ := json.Marshal(WSMessage{Type: "input", Data: string(p)})
	err := s.ws.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize sends a terminal resize event.
func (s *ExecSession) Resize(cols, rows int) error {
	msg, _ := json.Marshal(WSMessage{Type: "resize", Cols: cols, Rows: rows})
	return s.ws.WriteMessage(websocket.TextMessage, msg)
}

// Wait blocks until the session ends. Returns the exit code.
func (s *ExecSession) Wait() int {
	<-s.exitCh
	return s.exitCode
}

// Close closes the session and the underlying WebSocket connection.
func (s *ExecSession) Close() error {
	s.closeOnce.Do(func() { close(s.closeCh) })
	return s.ws.Close()
}
