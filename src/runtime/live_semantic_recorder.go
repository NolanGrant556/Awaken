package runtime

import (
	"context"
	"sync"

	"awaken/src/agentcore"
)

// liveSemanticRecorder makes assigned SemanticRecords the only durable live
// event source. Non-terminal records are published after append succeeds;
// terminal records are held until Flush confirms the journal is durable.
type liveSemanticRecorder struct {
	base    agentcore.AssignedSemanticRecorder
	session *Session

	mu               sync.Mutex
	pendingTerminals []agentcore.SemanticRecord
}

func newLiveSemanticRecorder(base agentcore.AssignedSemanticRecorder, session *Session) *liveSemanticRecorder {
	return &liveSemanticRecorder{base: base, session: session}
}

func (r *liveSemanticRecorder) BroadcastsCanonicalSemanticRecords() bool { return true }

func (r *liveSemanticRecorder) Append(ctx context.Context, record agentcore.SemanticRecord) error {
	_, err := r.AppendAssigned(ctx, record)
	return err
}

func (r *liveSemanticRecorder) AppendBatch(ctx context.Context, records []agentcore.SemanticRecord) error {
	_, err := r.AppendBatchAssigned(ctx, records)
	return err
}

func (r *liveSemanticRecorder) AppendAssigned(ctx context.Context, record agentcore.SemanticRecord) (agentcore.SemanticRecord, error) {
	assigned, err := r.base.AppendAssigned(ctx, record)
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	r.publishAppended([]agentcore.SemanticRecord{assigned})
	return assigned, nil
}

func (r *liveSemanticRecorder) AppendBatchAssigned(ctx context.Context, records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	assigned, err := r.base.AppendBatchAssigned(ctx, records)
	if err != nil {
		return nil, err
	}
	r.publishAppended(assigned)
	return assigned, nil
}

func (r *liveSemanticRecorder) publishAppended(records []agentcore.SemanticRecord) {
	if len(records) == 0 {
		return
	}
	nonTerminal := make([]agentcore.SemanticRecord, 0, len(records))
	r.mu.Lock()
	for _, record := range records {
		if record.Kind == agentcore.SemanticKindRunTerminal {
			r.pendingTerminals = append(r.pendingTerminals, record)
			continue
		}
		nonTerminal = append(nonTerminal, record)
	}
	r.mu.Unlock()
	if r.session != nil && len(nonTerminal) > 0 {
		r.session.RecordSemanticRecords(nonTerminal)
	}
}

func (r *liveSemanticRecorder) Flush(ctx context.Context) error {
	if err := r.base.Flush(ctx); err != nil {
		// A terminal whose fsync failed must never become visible later. The
		// runner may append a persistence_error terminal and retry Flush; only
		// that replacement terminal is then eligible for live publication.
		r.mu.Lock()
		r.pendingTerminals = nil
		r.mu.Unlock()
		return err
	}
	r.mu.Lock()
	terminals := append([]agentcore.SemanticRecord(nil), r.pendingTerminals...)
	r.pendingTerminals = nil
	r.mu.Unlock()
	if len(terminals) > 1 {
		terminals = terminals[len(terminals)-1:]
	}
	if r.session != nil && len(terminals) > 0 {
		r.session.RecordSemanticRecords(terminals)
	}
	return nil
}

var _ agentcore.AssignedSemanticRecorder = (*liveSemanticRecorder)(nil)
var _ agentcore.CanonicalLiveSemanticRecorder = (*liveSemanticRecorder)(nil)
