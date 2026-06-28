package structuredio

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

const defaultResolvedIDLimit = 1000

var ErrPendingRequestsClosed = errors.New("structuredio: pending requests closed")

type PendingRequestsOption func(*PendingRequests)

func WithRequestIDGenerator(generator func() string) PendingRequestsOption {
	return func(manager *PendingRequests) {
		if generator != nil {
			manager.nextID = generator
		}
	}
}

func WithResolvedIDLimit(limit int) PendingRequestsOption {
	return func(manager *PendingRequests) {
		manager.resolved.limit = limit
	}
}

type PendingRequests struct {
	mu       sync.Mutex
	pending  map[string]*PendingRequest
	resolved boundedIDSet
	nextID   func() string
	closed   bool
}

type PendingRequest struct {
	RequestID   string
	Message     ControlRequest
	duplicateID string

	once sync.Once
	done chan PendingResult
}

type PendingResult struct {
	Response ControlResponse
	Err      error
}

type ResponseStatus string

const (
	ResponseResolved          ResponseStatus = "resolved"
	ResponseUnexpected        ResponseStatus = "unexpected"
	ResponseResolvedDuplicate ResponseStatus = "resolved_duplicate"
)

type ResponseResolution struct {
	Status    ResponseStatus
	RequestID string
	Request   *PendingRequest
	Err       error
}

func NewPendingRequests(options ...PendingRequestsOption) *PendingRequests {
	manager := &PendingRequests{
		pending: make(map[string]*PendingRequest),
		resolved: boundedIDSet{
			limit: defaultResolvedIDLimit,
			seen:  make(map[string]struct{}),
		},
		nextID: newRequestID,
	}
	for _, option := range options {
		option(manager)
	}
	if manager.resolved.limit < 0 {
		manager.resolved.limit = 0
	}
	return manager
}

func (m *PendingRequests) SendRequest(request any) (*PendingRequest, error) {
	raw, err := marshalRawMessage(request)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrPendingRequestsClosed
	}

	requestID := m.nextID()
	if requestID == "" {
		requestID = newRequestID()
	}
	if _, exists := m.pending[requestID]; exists {
		return nil, fmt.Errorf("structuredio: duplicate request id %q", requestID)
	}

	message := ControlRequest{
		Type:      EnvelopeControlRequest,
		RequestID: requestID,
		Request:   raw,
	}
	pending := &PendingRequest{
		RequestID:   requestID,
		Message:     message,
		duplicateID: duplicateIDFromPayload(raw),
		done:        make(chan PendingResult, 1),
	}
	m.pending[requestID] = pending
	return pending, nil
}

func (m *PendingRequests) Pending() []ControlRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	requests := make([]ControlRequest, 0, len(m.pending))
	for _, pending := range m.pending {
		requests = append(requests, pending.Message)
	}
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].RequestID < requests[j].RequestID
	})
	return requests
}

func (m *PendingRequests) ResolveResponse(response ControlResponse) ResponseResolution {
	requestID := response.Response.RequestID
	responseDuplicateID := duplicateIDFromPayload(response.Response.Response)

	m.mu.Lock()
	pending := m.pending[requestID]
	if pending == nil {
		status := ResponseUnexpected
		if m.resolved.Contains(requestID) || m.resolved.Contains(responseDuplicateID) {
			status = ResponseResolvedDuplicate
		}
		m.mu.Unlock()
		return ResponseResolution{Status: status, RequestID: requestID}
	}

	delete(m.pending, requestID)
	m.resolved.Add(requestID)
	m.resolved.Add(pending.duplicateID)
	m.resolved.Add(responseDuplicateID)
	m.mu.Unlock()

	resultErr := errorFromControlResponse(response)
	pending.complete(PendingResult{
		Response: response,
		Err:      resultErr,
	})

	return ResponseResolution{
		Status:    ResponseResolved,
		RequestID: requestID,
		Request:   pending,
		Err:       resultErr,
	}
}

func (m *PendingRequests) RejectAllOnClose(err error) []ControlRequest {
	if err == nil {
		err = errors.New("structuredio: input closed before response received")
	}

	m.mu.Lock()
	pending := make([]*PendingRequest, 0, len(m.pending))
	for _, request := range m.pending {
		pending = append(pending, request)
	}
	m.pending = make(map[string]*PendingRequest)
	m.closed = true
	m.mu.Unlock()

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].RequestID < pending[j].RequestID
	})

	rejected := make([]ControlRequest, 0, len(pending))
	for _, request := range pending {
		request.complete(PendingResult{Err: err})
		rejected = append(rejected, request.Message)
	}
	return rejected
}

func (p *PendingRequest) Done() <-chan PendingResult {
	return p.done
}

func (p *PendingRequest) complete(result PendingResult) {
	p.once.Do(func() {
		p.done <- result
		close(p.done)
	})
}

func errorFromControlResponse(response ControlResponse) error {
	if response.Response.Subtype != "error" {
		return nil
	}
	if response.Response.Error == "" {
		return errors.New("structuredio: control response error")
	}
	return errors.New(response.Response.Error)
}

func marshalRawMessage(value any) (json.RawMessage, error) {
	switch typed := value.(type) {
	case nil:
		return json.RawMessage(`{}`), nil
	case json.RawMessage:
		return copyRawJSON(typed)
	case []byte:
		return copyRawJSON(typed)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(raw), nil
	}
}

func copyRawJSON(raw []byte) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("structuredio: raw JSON is invalid")
	}
	return append(json.RawMessage(nil), raw...), nil
}

func duplicateIDFromPayload(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}

	for _, key := range []string{"tool_use_id", "toolUseID"} {
		valueRaw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(valueRaw, &value); err == nil && value != "" {
			return value
		}
	}
	return ""
}

type boundedIDSet struct {
	limit int
	order []string
	seen  map[string]struct{}
}

func (s *boundedIDSet) Add(id string) {
	if id == "" || s.limit == 0 {
		return
	}
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	if _, ok := s.seen[id]; ok {
		return
	}
	if len(s.order) >= s.limit {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.seen, oldest)
	}
	s.order = append(s.order, id)
	s.seen[id] = struct{}{}
}

func (s *boundedIDSet) Contains(id string) bool {
	if id == "" {
		return false
	}
	_, ok := s.seen[id]
	return ok
}

var fallbackRequestID atomic.Uint64

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", fallbackRequestID.Add(1))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
