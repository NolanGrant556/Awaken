package mcpelicitation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type ManagerOption func(*Manager)

type Resolver interface {
	Resolve(context.Context, Request) (Response, bool, error)
}

type ResolveResult struct {
	Snapshot    ItemSnapshot
	Duplicate   bool
	HookHandled bool
}

type Manager struct {
	mu     sync.Mutex
	policy URLPolicy
	now    func() time.Time
	items  map[Key]*queueItem
	order  []Key
}

type queueItem struct {
	request    Request
	state      State
	lastAction Action
	response   *Response
	createdAt  time.Time
	updatedAt  time.Time
}

func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		policy: DefaultURLPolicy(),
		now:    time.Now,
		items:  make(map[Key]*queueItem),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func WithURLPolicy(policy URLPolicy) ManagerOption {
	return func(m *Manager) {
		m.policy = policy
	}
}

func WithClock(now func() time.Time) ManagerOption {
	return func(m *Manager) {
		if now != nil {
			m.now = now
		}
	}
}

func (m *Manager) Enqueue(req Request) (ItemSnapshot, bool, error) {
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return ItemSnapshot{}, false, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.items[prepared.Key()]; ok {
		return existing.snapshot(), true, nil
	}

	now := m.now()
	if prepared.CreatedAt.IsZero() {
		prepared.CreatedAt = now
	}
	item := &queueItem{
		request:   prepared,
		state:     StateQueued,
		createdAt: prepared.CreatedAt,
		updatedAt: now,
	}
	m.items[prepared.Key()] = item
	m.order = append(m.order, prepared.Key())
	return item.snapshot(), false, nil
}

func (m *Manager) ResolveOrEnqueue(ctx context.Context, req Request, resolver Resolver) (ResolveResult, error) {
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return ResolveResult{}, err
	}

	m.mu.Lock()
	if existing, ok := m.items[prepared.Key()]; ok {
		snapshot := existing.snapshot()
		m.mu.Unlock()
		return ResolveResult{Snapshot: snapshot, Duplicate: true}, nil
	}
	m.mu.Unlock()

	if resolver != nil {
		response, handled, err := resolver.Resolve(ctx, prepared)
		if err != nil {
			return ResolveResult{}, err
		}
		if handled {
			snapshot, duplicate, err := m.record(prepared, normalizeResponse(response, SourceHook), true)
			if err != nil {
				return ResolveResult{}, err
			}
			return ResolveResult{Snapshot: snapshot, Duplicate: duplicate, HookHandled: true}, nil
		}
	}

	snapshot, duplicate, err := m.Enqueue(prepared)
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{Snapshot: snapshot, Duplicate: duplicate}, nil
}

func (m *Manager) Open(key Key) (ItemSnapshot, error) {
	return m.update(key, Response{Action: ActionOpen, Source: SourceUser}, false)
}

func (m *Manager) Complete(key Key, response Response) (ItemSnapshot, error) {
	return m.update(key, response, false)
}

func (m *Manager) Cancel(key Key) (ItemSnapshot, error) {
	return m.update(key, Response{Action: ActionCancel, Source: SourceSystem}, false)
}

func (m *Manager) Timeout(key Key) (ItemSnapshot, error) {
	return m.update(key, Response{Action: ActionTimeout, Source: SourceSystem}, false)
}

func (m *Manager) Get(key Key) (ItemSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[key]
	if !ok {
		return ItemSnapshot{}, false
	}
	return item.snapshot(), true
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	items := make([]ItemSnapshot, 0, len(m.order))
	var pending int
	var updatedAt time.Time
	for _, key := range m.order {
		item, ok := m.items[key]
		if !ok {
			continue
		}
		snapshot := item.snapshot()
		items = append(items, snapshot)
		if statePending(snapshot.State) {
			pending++
		}
		if snapshot.UpdatedAt.After(updatedAt) {
			updatedAt = snapshot.UpdatedAt
		}
	}
	return Snapshot{
		Items:        items,
		PendingCount: pending,
		UpdatedAt:    updatedAt,
	}
}

func (m *Manager) update(key Key, response Response, allowCreate bool) (ItemSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[key]
	if !ok {
		if !allowCreate {
			return ItemSnapshot{}, fmt.Errorf("elicitation %s/%s/%s not found", key.ServerName, key.ToolName, key.ElicitationID)
		}
		return ItemSnapshot{}, fmt.Errorf("cannot create elicitation without request")
	}
	response = normalizeResponse(response, SourceUser)
	item.apply(response, m.now())
	return item.snapshot(), nil
}

func (m *Manager) record(req Request, response Response, allowDuplicate bool) (ItemSnapshot, bool, error) {
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return ItemSnapshot{}, false, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.items[prepared.Key()]; ok {
		if !allowDuplicate {
			return ItemSnapshot{}, true, nil
		}
		return existing.snapshot(), true, nil
	}

	now := m.now()
	if prepared.CreatedAt.IsZero() {
		prepared.CreatedAt = now
	}
	response = normalizeResponse(response, SourceHook)
	item := &queueItem{
		request:   prepared,
		state:     actionState(response.Action),
		createdAt: prepared.CreatedAt,
		updatedAt: now,
	}
	if response.At.IsZero() {
		response.At = now
	}
	item.lastAction = response.Action
	item.response = &response
	m.items[prepared.Key()] = item
	m.order = append(m.order, prepared.Key())
	return item.snapshot(), false, nil
}

func (m *Manager) prepareRequest(req Request) (Request, error) {
	req.ServerName = strings.TrimSpace(req.ServerName)
	req.ToolName = strings.TrimSpace(req.ToolName)
	req.ElicitationID = strings.TrimSpace(req.ElicitationID)
	if req.ServerName == "" {
		return Request{}, fmt.Errorf("server name is required")
	}
	if req.ToolName == "" {
		return Request{}, fmt.Errorf("tool name is required")
	}
	if req.ElicitationID == "" {
		return Request{}, fmt.Errorf("elicitation id is required")
	}

	info, err := m.policy.Validate(req.URL)
	if err != nil {
		return Request{}, err
	}
	req.URL = info.URL
	req.RedactedURL = info.RedactedURL
	return req, nil
}

func (i *queueItem) apply(response Response, now time.Time) {
	if response.At.IsZero() {
		response.At = now
	}
	i.state = actionState(response.Action)
	i.lastAction = response.Action
	i.response = &response
	i.updatedAt = now
}

func (i *queueItem) snapshot() ItemSnapshot {
	var response *Response
	if i.response != nil {
		copy := *i.response
		response = &copy
	}
	return ItemSnapshot{
		ServerName:    i.request.ServerName,
		ToolName:      i.request.ToolName,
		ElicitationID: i.request.ElicitationID,
		Message:       i.request.Message,
		RedactedURL:   i.request.SafeURL(),
		State:         i.state,
		LastAction:    i.lastAction,
		Response:      response,
		CreatedAt:     i.createdAt,
		UpdatedAt:     i.updatedAt,
	}
}

func normalizeResponse(response Response, fallback Source) Response {
	if response.Action == "" {
		response.Action = ActionApprove
	}
	if response.Source == "" {
		response.Source = fallback
	}
	return response
}
