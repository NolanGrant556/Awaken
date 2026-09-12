package mcpclient

import "context"

func (m *Manager) initLifetimeLocked() {
	if m.stopContext == nil {
		m.stopContext, m.stopCancel = context.WithCancel(context.Background())
	}
	if m.shutdownDone == nil {
		m.shutdownDone = make(chan struct{})
	}
}

func (m *Manager) beginOperation(parent context.Context) (context.Context, func(), error) {
	m.mu.Lock()
	m.initLifetimeLocked()
	if m.closing {
		m.mu.Unlock()
		return nil, nil, ErrTransportClosed
	}
	m.operations.Add(1)
	stop := m.stopContext
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	unlink := context.AfterFunc(stop, cancel)
	return ctx, func() { unlink(); cancel(); m.operations.Done() }, nil
}

func (m *Manager) acquireCall(ctx context.Context, server *managedServer) (func(), error) {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return nil, ErrTransportClosed
	}
	if server.callGate == nil {
		server.callGate = make(chan struct{}, 1)
	}
	gate := server.callGate
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
		m.mu.RLock()
		closing := m.closing
		m.mu.RUnlock()
		if closing {
			<-gate
			return nil, ErrTransportClosed
		}
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager) BeginShutdown() {
	m.mu.Lock()
	m.initLifetimeLocked()
	m.closing = true
	cancel := m.stopCancel
	m.mu.Unlock()
	cancel()
}

// Shutdown is joinable after a timed-out attempt. No session callback runs
// under the manager lock, and an uncooperative Close cannot exceed ctx's limit.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.BeginShutdown()
	m.shutdownOnce.Do(func() {
		go func() { m.shutdownErr = m.closeConnections(); close(m.shutdownDone) }()
	})
	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// The caller holds mu and has checked !closing. Retired sessions remain in
// the drain even after replacement removes them from the current registry.
func (m *Manager) detachSessionLocked(server *managedServer) func() {
	session := server.session
	if session == nil {
		return func() {}
	}
	server.session = nil
	m.operations.Add(1)
	return func() { defer m.operations.Done(); _ = session.Close() }
}
