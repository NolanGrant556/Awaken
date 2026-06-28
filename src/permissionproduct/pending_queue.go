package permissionproduct

import "time"

type PendingQueue struct {
	entriesByID map[string]pendingEntry
	idByDedupe  map[string]string
	order       []string
}

type pendingEntry struct {
	request  PermissionRequest
	decision PermissionDecision
	dedupe   string
}

func NewPendingQueue() PendingQueue {
	return PendingQueue{
		entriesByID: make(map[string]pendingEntry),
		idByDedupe:  make(map[string]string),
	}
}

func (q *PendingQueue) Add(request PermissionRequest, decision PermissionDecision, now time.Time) (PermissionDecision, bool) {
	q.ensure()
	q.PruneExpired(now)

	dedupe := request.dedupeString()
	if existingID, ok := q.idByDedupe[dedupe]; ok {
		if existing, ok := q.entriesByID[existingID]; ok {
			return existing.decision, true
		}
	}

	q.entriesByID[request.ID] = pendingEntry{
		request:  request,
		decision: decision,
		dedupe:   dedupe,
	}
	q.idByDedupe[dedupe] = request.ID
	q.order = append(q.order, request.ID)
	return decision, false
}

func (q *PendingQueue) List(now time.Time) []PermissionRequest {
	q.ensure()
	q.PruneExpired(now)

	requests := make([]PermissionRequest, 0, len(q.entriesByID))
	for _, id := range q.order {
		entry, ok := q.entriesByID[id]
		if !ok {
			continue
		}
		requests = append(requests, entry.request)
	}
	return requests
}

func (q *PendingQueue) Pop(requestID string) (PermissionRequest, bool) {
	q.ensure()
	entry, ok := q.entriesByID[requestID]
	if !ok {
		return PermissionRequest{}, false
	}
	q.remove(requestID, entry.dedupe)
	return entry.request, true
}

func (q *PendingQueue) PruneExpired(now time.Time) []PermissionRequest {
	q.ensure()
	if len(q.entriesByID) == 0 {
		return nil
	}

	expired := make([]PermissionRequest, 0)
	for _, id := range q.order {
		entry, ok := q.entriesByID[id]
		if !ok || !entry.request.IsExpired(now) {
			continue
		}
		expired = append(expired, entry.request)
		q.remove(id, entry.dedupe)
	}
	return expired
}

func (q *PendingQueue) ensure() {
	if q.entriesByID == nil {
		q.entriesByID = make(map[string]pendingEntry)
	}
	if q.idByDedupe == nil {
		q.idByDedupe = make(map[string]string)
	}
}

func (q *PendingQueue) remove(requestID string, dedupe string) {
	delete(q.entriesByID, requestID)
	delete(q.idByDedupe, dedupe)
}

func (r PermissionRequest) dedupeString() string {
	agent := r.Agent.Normalize()
	return r.SessionID + "\x00" + r.ProjectID + "\x00" + agent.ID + "\x00" + agent.Type + "\x00" + r.Key().dedupeString() + "\x00" + string(normalizeRiskLevel(r.RiskLevel))
}
