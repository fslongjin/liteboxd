package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
)

type wsTestPair struct {
	client *websocket.Conn
	server *websocket.Conn
	close  func()
}

func newWSTestPair(t *testing.T) wsTestPair {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)
	errCh := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial websocket: %v", err)
	}

	select {
	case err := <-errCh:
		clientConn.Close()
		srv.Close()
		t.Fatalf("upgrade websocket: %v", err)
	case serverConn := <-serverConnCh:
		return wsTestPair{
			client: clientConn,
			server: serverConn,
			close: func() {
				_ = clientConn.Close()
				_ = serverConn.Close()
				srv.Close()
			},
		}
	case <-time.After(2 * time.Second):
		clientConn.Close()
		srv.Close()
		t.Fatal("timed out waiting for websocket server connection")
	}
	return wsTestPair{}
}

func TestRunWebSocketProxySessionPropagatesBackendClose(t *testing.T) {
	front := newWSTestPair(t)
	defer front.close()
	back := newWSTestPair(t)
	defer back.close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWebSocketProxySession(context.Background(), logger, "test-backend-close", front.server, back.client)
	}()

	if err := back.server.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "backend_bye"),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("send backend close: %v", err)
	}

	closeErr := readCloseError(t, front.client)
	if closeErr.Code != websocket.CloseNormalClosure {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseNormalClosure)
	}
	if closeErr.Text != "backend_bye" {
		t.Fatalf("close text = %q, want %q", closeErr.Text, "backend_bye")
	}

	waitDone(t, done)
}

func TestRunWebSocketProxySessionPropagatesClientClose(t *testing.T) {
	front := newWSTestPair(t)
	defer front.close()
	back := newWSTestPair(t)
	defer back.close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWebSocketProxySession(context.Background(), logger, "test-client-close", front.server, back.client)
	}()

	if err := front.client.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseGoingAway, "client_left"),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("send client close: %v", err)
	}

	closeErr := readCloseError(t, back.server)
	if closeErr.Code != websocket.CloseGoingAway {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseGoingAway)
	}
	if closeErr.Text != "client_left" {
		t.Fatalf("close text = %q, want %q", closeErr.Text, "client_left")
	}

	waitDone(t, done)
}

func TestRunWebSocketProxySessionConvertsBackendDropToSyntheticClose(t *testing.T) {
	front := newWSTestPair(t)
	defer front.close()
	back := newWSTestPair(t)
	defer back.close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWebSocketProxySession(context.Background(), logger, "test-backend-drop", front.server, back.client)
	}()

	if err := back.client.NetConn().Close(); err != nil {
		t.Fatalf("close proxy backend net conn: %v", err)
	}

	closeErr := readCloseError(t, front.client)
	if closeErr.Code != websocket.CloseInternalServerErr {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseInternalServerErr)
	}
	if closeErr.Text != "upstream_read_failed" {
		t.Fatalf("close text = %q, want %q", closeErr.Text, "upstream_read_failed")
	}

	waitDone(t, done)
}

func TestProxyCloseForUnexpectedUsesFailureSide(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   wsRelayResult
		wantCode int
		wantText string
	}{
		{
			name:     "backend read failure",
			result:   wsRelayResult{Source: "backend", Target: "client", Stage: "read"},
			wantCode: websocket.CloseInternalServerErr,
			wantText: "upstream_read_failed",
		},
		{
			name:     "client read failure",
			result:   wsRelayResult{Source: "client", Target: "backend", Stage: "read"},
			wantCode: websocket.CloseGoingAway,
			wantText: "client_read_failed",
		},
		{
			name:     "backend write failure",
			result:   wsRelayResult{Source: "client", Target: "backend", Stage: "write"},
			wantCode: websocket.CloseInternalServerErr,
			wantText: "upstream_write_failed",
		},
		{
			name:     "client write failure",
			result:   wsRelayResult{Source: "backend", Target: "client", Stage: "write"},
			wantCode: websocket.CloseGoingAway,
			wantText: "client_write_failed",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotCode, gotText := proxyCloseForUnexpected(tc.result)
			if gotCode != tc.wantCode {
				t.Fatalf("close code = %d, want %d", gotCode, tc.wantCode)
			}
			if gotText != tc.wantText {
				t.Fatalf("close text = %q, want %q", gotText, tc.wantText)
			}
		})
	}
}

func TestDialK8sBackendWebSocketUsesClientGoWrappers(t *testing.T) {
	t.Parallel()

	var proxyCalled atomic.Bool
	upgrader := websocket.Upgrader{
		CheckOrigin:  func(r *http.Request) bool { return true },
		Subprotocols: []string{"trace"},
	}
	serverErrCh := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k8s-token" {
			http.Error(w, "missing k8s auth", http.StatusUnauthorized)
			serverErrCh <- errors.New("authorization header mismatch: " + got)
			return
		}
		if got := r.Header.Get("X-Test-Header"); got != "present" {
			http.Error(w, "missing passthrough header", http.StatusBadRequest)
			serverErrCh <- errors.New("x-test-header mismatch: " + got)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}))
	defer srv.Close()

	backendURL, err := url.Parse(srv.URL + "/ws")
	if err != nil {
		t.Fatalf("parse backend url: %v", err)
	}

	header := http.Header{
		"Authorization":          []string{"Bearer app-token"},
		"Sec-WebSocket-Protocol": []string{"chat, trace"},
		"X-Test-Header":          []string{"present"},
	}
	config := &rest.Config{
		BearerToken: "k8s-token",
		Proxy: func(req *http.Request) (*url.URL, error) {
			proxyCalled.Store(true)
			return nil, nil
		},
	}

	conn, resp, err := dialK8sBackendWebSocket(context.Background(), config, backendURL, header)
	if err != nil {
		t.Fatalf("dial k8s websocket: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	defer conn.Close()

	if !proxyCalled.Load() {
		t.Fatal("expected proxy function to be used")
	}
	if got := conn.Subprotocol(); got != "trace" {
		t.Fatalf("subprotocol = %q, want %q", got, "trace")
	}
	if _, message, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read websocket message: %v", err)
	} else if string(message) != "hello" {
		t.Fatalf("message = %q, want %q", string(message), "hello")
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("server validation failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket server validation")
	}
}

func readCloseError(t *testing.T, conn *websocket.Conn) *websocket.CloseError {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				return closeErr
			}
			t.Fatalf("read websocket: %v", err)
		}
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for proxy session to exit")
	}
}
