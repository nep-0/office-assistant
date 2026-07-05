package chat

import (
	"context"
	"sync"
)

type CancelRegistry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewCancelRegistry() *CancelRegistry {
	return &CancelRegistry{cancels: make(map[string]context.CancelFunc)}
}

func (r *CancelRegistry) Register(sessionID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[sessionID] = cancel
}

func (r *CancelRegistry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, sessionID)
}

func (r *CancelRegistry) Cancel(sessionID string) bool {
	r.mu.Lock()
	cancel := r.cancels[sessionID]
	r.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}
