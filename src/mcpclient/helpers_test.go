package mcpclient

import (
	"os"
)

// lookupArgs0 returns os.Args[0] — the compiled test binary under `go test`.
// Used by TestStdioTransportMockServer to exec the test binary in a sub-mode
// that exposes a stdio MCP server.
func lookupArgs0() string {
	if len(os.Args) == 0 {
		return ""
	}
	return os.Args[0]
}

// envValue returns the value of the named environment variable, or empty
// string if unset.
func envValue(key string) string {
	return os.Getenv(key)
}
