package connectors

import "log"

// StdLogger is the default Logger implementation using the standard log package.
type StdLogger struct{}

func (StdLogger) Debugf(format string, args ...any) { log.Printf("[connectors debug] "+format, args...) }
func (StdLogger) Warnf(format string, args ...any)  { log.Printf("[connectors warn] "+format, args...) }
func (StdLogger) Errorf(format string, args ...any) { log.Printf("[connectors error] "+format, args...) }