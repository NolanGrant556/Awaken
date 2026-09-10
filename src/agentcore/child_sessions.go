package agentcore

import (
	"context"
	"encoding/json"
	"sync"

	convcontext "awaken/src/context"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

type ChildRestoreState struct {
	RunPlan         *tool.RunPlanSnapshot
	RoleDigest      string
	Worktree        *subagentruntime.WorktreeReceipt
	ContextSeed     json.RawMessage
	Messages        []convcontext.Message
	Bundle          *PostCompactContextBundleV2
	UsageAnchor     *ContextWindowUsageAnchor
	SkillBlocks     []ContextBlock
	ActivatedTools  []string
	ContextRevision uint64
	ToolExecutions  int
	ModelTurns      int
}

// ChildSessions belongs to a thread. Agent identity survives assignments while
// each context has one writer. The map itself never contains a running parent
// Runner; sessions retain their own configuration and conversation.
type ChildSessions struct {
	mu       sync.Mutex
	sessions map[string]*childSession
}

type childSession struct {
	users        int
	worktree     *subagentruntime.WorktreeReceipt
	gate         chan struct{}
	initialized  bool
	config       AgentConfig
	conversation *convcontext.Conversation
}

func NewChildSessions() *ChildSessions {
	return &ChildSessions{sessions: make(map[string]*childSession)}
}

func (s *ChildSessions) Forget(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *ChildSessions) unpin(id string, entry *childSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.users--
	for key, candidate := range s.sessions {
		if len(s.sessions) <= 16 {
			break
		}
		if key != id && candidate.users == 0 && candidate.initialized && candidate.config.LoadChildContext != nil {
			delete(s.sessions, key)
		}
	}
}

func (s *ChildSessions) acquire(ctx context.Context, id string) (*childSession, func(), error) {
	s.mu.Lock()
	entry := s.sessions[id]
	if entry == nil {
		entry = &childSession{gate: make(chan struct{}, 1)}
		s.sessions[id] = entry
	}
	entry.users++
	s.mu.Unlock()
	select {
	case entry.gate <- struct{}{}:
		return entry, func() { <-entry.gate; s.unpin(id, entry) }, nil
	case <-ctx.Done():
		s.unpin(id, entry)
		return nil, nil, ctx.Err()
	}
}
