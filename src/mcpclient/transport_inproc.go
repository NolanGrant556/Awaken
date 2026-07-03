package mcpclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InProcServerFactory builds a fresh *mcp.Server each time the manager
// connects. The server is connected to the in-memory transport pair and
// runs on its own goroutine for the lifetime of the connection. Factories
// must be safe to call multiple times.
type InProcServerFactory func() *mcp.Server

// InProcRegistry holds local Go-implemented MCP server factories by name.
// Servers register themselves at construction; the manager looks them up
// when building an in-process transport.
type InProcRegistry struct {
	mu        sync.RWMutex
	factories map[string]InProcServerFactory
}

// NewInProcRegistry returns an empty registry.
func NewInProcRegistry() *InProcRegistry {
	return &InProcRegistry{factories: make(map[string]InProcServerFactory)}
}

// Register adds a factory under the given name. Duplicate names return an
// error.
func (r *InProcRegistry) Register(name string, factory InProcServerFactory) error {
	if factory == nil {
		return fmt.Errorf("in-proc factory %q is nil", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("in-proc server %q already registered", name)
	}
	r.factories[name] = factory
	return nil
}

// Lookup returns the factory registered under name, or an error.
func (r *InProcRegistry) Lookup(name string) (InProcServerFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("in-proc server %q not found", name)
	}
	return factory, nil
}

// preconnectedTransport wraps an already-acquired mcp.Connection so that
// Server.Run / Server.Connect reuse it instead of calling
// InMemoryTransport.Connect a second time. This gives runInProcServer a
// handle it can Close() to unblock the client reader when the factory
// panics before the server takes ownership of the transport.
type preconnectedTransport struct {
	conn mcp.Connection
	once sync.Once
}

func (t *preconnectedTransport) Connect(context.Context) (mcp.Connection, error) {
	return t.conn, nil
}

// runInProcServer bridges an InProcServerFactory to the server end of an
// in-memory transport pair. It runs until the client side disconnects or
// the server returns. Panics from the server factory are recovered so a
// buggy server cannot crash the host process; the recovery also closes
// the server-side connection so the client observes EOF rather than
// hanging indefinitely. When onPanic is set, it is called with the panic
// value so callers can record it as the server's error reason.
func runInProcServer(factory InProcServerFactory, serverTransport *mcp.InMemoryTransport, onPanic func(panicMsg string)) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverConn, err := serverTransport.Connect(ctx)
	if err != nil {
		return
	}
	wrapped := &preconnectedTransport{conn: serverConn}
	defer serverConn.Close()

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("in-proc server panic: %v", r)
			if onPanic != nil {
				onPanic(msg)
			}
		}
	}()

	server := factory()
	if server == nil {
		return
	}
	_ = server.Run(ctx, wrapped)
}

// ErrInProcNotRegistered is returned when a transport is requested for a
// server name that no longer exists in the registry.
var ErrInProcNotRegistered = fmt.Errorf("in-proc server not registered")
