package mcpclient

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
)

func TestAuthClassificationRejects401InConnectionAddress(t *testing.T) {
	err := fmt.Errorf("initialize: %w", &url.Error{Op: "Post", URL: "http://127.0.0.1:40151", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}})
	if isAuthError(err) {
		t.Fatal("connection refused must not become needs-auth")
	}
	if !isAuthError(errors.New("HTTP 401 Unauthorized")) {
		t.Fatal("HTTP 401 must remain an authentication failure")
	}
}
