package agentbridge

import (
	"context"
	"log"
	"os"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

// Server runs the ACP agent-side connection over stdio.
type Server struct {
	Agent      *Agent
	LeaseID    string
	CoreClient *CoreClient
}

// Run starts the stdio ACP server and blocks until the peer disconnects or the
// context is cancelled. It also keeps the bridge lease alive.
func (s *Server) Run(ctx context.Context) error {
	conn := acpsdk.NewAgentSideConnection(s.Agent, os.Stdout, os.Stdin)
	s.Agent.SetConnection(conn)

	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	if s.LeaseID != "" && s.CoreClient != nil {
		go s.leaseHeartbeatLoop(leaseCtx, s.LeaseID)
	}

	select {
	case <-conn.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) leaseHeartbeatLoop(ctx context.Context, leaseID string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.CoreClient.HeartbeatLease(ctx, leaseID); err != nil {
				log.Printf("awaken-acp: lease heartbeat failed: %v", err)
			}
		}
	}
}
