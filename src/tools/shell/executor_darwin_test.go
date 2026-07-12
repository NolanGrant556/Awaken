//go:build darwin

package shelltool

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

func TestDarwinWorkspaceBrokerDeniesInterpreterDeleteOutsideWorkspace(t *testing.T) {
	if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("sandbox-exec cannot establish a Seatbelt boundary in this environment")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 unavailable: %v", err)
	}
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fmt.Sprintf(`python3 -c 'import os; os.remove(%q)'`, outside)
	ctx := tool.ContextWithExecutionBroker(context.Background(), tool.NewWorkspaceExecutionBroker())
	result, err := NewShellExecutor().Execute(ctx, shellParams{Command: command}, tool.NewPermissionContext(workspace, nil), nil)
	if err != nil {
		t.Fatalf("process-level denial should return a process result, result=%+v err=%v", result, err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("outside delete unexpectedly succeeded: %+v", result)
	}
	content, readErr := os.ReadFile(outside)
	if readErr != nil || string(content) != "preserve" {
		t.Fatalf("interpreter escaped workspace sandbox: content=%q err=%v", content, readErr)
	}
}

func TestDarwinWorkspaceBrokerDeniesUnknownChildNetwork(t *testing.T) {
	if sandbox.DefaultAdapter().SupportLevel() != sandbox.SandboxFull {
		t.Skip("sandbox-exec cannot establish a Seatbelt boundary in this environment")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skipf("python3 unavailable: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tcpListener := listener.(*net.TCPListener)
	_ = tcpListener.SetDeadline(time.Now().Add(750 * time.Millisecond))
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	command := fmt.Sprintf(`python3 -c 'import socket; socket.create_connection((%q, %d), 1)'`, host, port)
	ctx := tool.ContextWithExecutionBroker(context.Background(), tool.NewWorkspaceExecutionBroker())
	result, runErr := NewShellExecutor().Execute(ctx, shellParams{Command: command}, tool.NewPermissionContext(t.TempDir(), nil), nil)
	if runErr != nil {
		t.Fatalf("process-level network denial should return a process result, result=%+v err=%v", result, runErr)
	}
	if result.ExitCode == 0 {
		t.Fatalf("unknown child network unexpectedly succeeded: %+v", result)
	}
	connection, acceptErr := tcpListener.Accept()
	if connection != nil {
		_ = connection.Close()
		t.Fatal("unknown child process reached a network recipient")
	}
	if networkError, ok := acceptErr.(net.Error); !ok || !networkError.Timeout() {
		t.Fatalf("expected listener timeout with no connection, got %v", acceptErr)
	}
}
