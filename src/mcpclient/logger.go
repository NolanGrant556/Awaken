package mcpclient

import "log"

// Logger is the minimal logging surface mcpclient consumes. The default
// implementation routes through the standard log package; tests may inject
// their own.
type Logger interface {
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// StdLogger is the default Logger implementation.
type StdLogger struct{}

func (StdLogger) Debugf(format string, args ...any) { log.Printf("[mcp debug] "+format, args...) }
func (StdLogger) Warnf(format string, args ...any)  { log.Printf("[mcp warn] "+format, args...) }
func (StdLogger) Errorf(format string, args ...any) { log.Printf("[mcp error] "+format, args...) }

// noopLogger discards all output; used when the consumer does not inject one.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}
