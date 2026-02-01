package k8s

import (
	"k8s.io/client-go/tools/remotecommand"
)

// SizeQueue implements remotecommand.TerminalSizeQueue
// for receiving terminal resize events from WebSocket clients.
type SizeQueue struct {
	resizeChan chan remotecommand.TerminalSize
}

// NewSizeQueue creates a new SizeQueue.
func NewSizeQueue() *SizeQueue {
	return &SizeQueue{
		resizeChan: make(chan remotecommand.TerminalSize, 1),
	}
}

// Next returns the next terminal size. Blocks until a resize event is available.
// Returns nil when the queue is closed (session ended).
func (sq *SizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-sq.resizeChan
	if !ok {
		return nil
	}
	return &size
}

// Push sends a resize event to the queue. Non-blocking; drops old events if full.
func (sq *SizeQueue) Push(width, height uint16) {
	select {
	case sq.resizeChan <- remotecommand.TerminalSize{Width: width, Height: height}:
	default:
		// Drop old event and push new one
		select {
		case <-sq.resizeChan:
		default:
		}
		sq.resizeChan <- remotecommand.TerminalSize{Width: width, Height: height}
	}
}

// Close closes the size queue.
func (sq *SizeQueue) Close() {
	close(sq.resizeChan)
}
