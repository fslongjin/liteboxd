package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var errDrainTimeout = errors.New("timeout waiting for websocket sessions to drain")

// DrainManager tracks draining state and active websocket sessions.
type DrainManager struct {
	draining atomic.Bool
	wsActive atomic.Int64
	wsWG     sync.WaitGroup
}

func NewDrainManager() *DrainManager {
	return &DrainManager{}
}

func (m *DrainManager) StartDraining() {
	m.draining.Store(true)
}

func (m *DrainManager) IsDraining() bool {
	return m.draining.Load()
}

func (m *DrainManager) ActiveWebSockets() int64 {
	return m.wsActive.Load()
}

// TrackWebSocket registers a websocket session and returns a release callback.
func (m *DrainManager) TrackWebSocket() func() {
	m.wsWG.Add(1)
	m.wsActive.Add(1)

	var once sync.Once
	return func() {
		once.Do(func() {
			m.wsActive.Add(-1)
			m.wsWG.Done()
		})
	}
}

func (m *DrainManager) WaitWebSockets(ctx context.Context) error {
	waitDone := make(chan struct{})
	go func() {
		m.wsWG.Wait()
		close(waitDone)
	}()

	select {
	case <-ctx.Done():
		return errDrainTimeout
	case <-waitDone:
		return nil
	}
}
