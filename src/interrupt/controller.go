package interrupt

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StateIdle        State = "idle"
	StateRunning     State = "running"
	StateInterrupted State = "interrupted"
	StateCompleted   State = "completed"
)

type EventKind string

const (
	EventRunStarted          EventKind = "run_started"
	EventInterrupted         EventKind = "interrupted"
	EventInterruptIgnored    EventKind = "interrupt_ignored"
	EventRunCompleted        EventKind = "run_completed"
	EventPermissionCancelled EventKind = "permission_cancelled"
)

var (
	ErrInvalidRunID       = errors.New("run id is required")
	ErrRunAlreadyActive   = errors.New("run already active")
	ErrNoActiveRun        = errors.New("no active run")
	ErrRunCompleted       = errors.New("run already completed")
	ErrAlreadyInterrupted = errors.New("run already interrupted")
	ErrViewerOnly         = errors.New("viewer only controller cannot interrupt")
	ErrReadOnly           = errors.New("read only controller cannot interrupt")
	ErrInvalidPermission  = errors.New("permission request id and tool use id are required")
	ErrPermissionExists   = errors.New("permission request already pending")
	ErrPermissionNotFound = errors.New("permission request not found")
)

type Options struct {
	ViewerOnly bool
	ReadOnly   bool
}

type Event struct {
	Kind      EventKind
	RunID     string
	State     State
	Reason    string
	Time      time.Time
	Err       error
	RequestID string
	ToolUseID string
}

type Snapshot struct {
	RunID              string
	State              State
	Reason             string
	UpdatedAt          time.Time
	ViewerOnly         bool
	ReadOnly           bool
	PendingPermissions map[string]string
}

type Listener func(Event)

type Controller struct {
	mu sync.Mutex

	runID     string
	state     State
	reason    string
	updatedAt time.Time

	viewerOnly bool
	readOnly   bool

	pendingPermissions map[string]string
	listeners          map[int]Listener
	nextListenerID     int
}

func NewController() *Controller {
	return NewControllerWithOptions(Options{})
}

func NewControllerWithOptions(options Options) *Controller {
	return &Controller{
		state:              StateIdle,
		viewerOnly:         options.ViewerOnly,
		readOnly:           options.ReadOnly,
		pendingPermissions: make(map[string]string),
		listeners:          make(map[int]Listener),
	}
}

func (c *Controller) StartRun(runID string) (Snapshot, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return c.Snapshot(), ErrInvalidRunID
	}

	c.mu.Lock()
	c.initLocked()
	if c.state == StateRunning || c.state == StateInterrupted {
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrRunAlreadyActive
	}

	now := time.Now().UTC()
	c.runID = runID
	c.state = StateRunning
	c.reason = ""
	c.updatedAt = now
	c.pendingPermissions = make(map[string]string)

	event := c.eventLocked(EventRunStarted, "", now, nil, "", "")
	snapshot := c.snapshotLocked()
	listeners := c.listenersLocked()
	c.mu.Unlock()

	notify(listeners, event)
	return snapshot, nil
}

func (c *Controller) Interrupt(reason string) (Snapshot, error) {
	c.mu.Lock()
	c.initLocked()

	switch c.state {
	case StateIdle:
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrNoActiveRun
	case StateCompleted:
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrRunCompleted
	case StateInterrupted:
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrAlreadyInterrupted
	}

	if c.viewerOnly || c.readOnly {
		err := ErrReadOnly
		if c.viewerOnly {
			err = ErrViewerOnly
		}
		now := time.Now().UTC()
		event := c.eventLocked(EventInterruptIgnored, reason, now, err, "", "")
		snapshot := c.snapshotLocked()
		listeners := c.listenersLocked()
		c.mu.Unlock()

		notify(listeners, event)
		return snapshot, err
	}

	now := time.Now().UTC()
	c.state = StateInterrupted
	c.reason = reason
	c.updatedAt = now

	event := c.eventLocked(EventInterrupted, reason, now, nil, "", "")
	snapshot := c.snapshotLocked()
	listeners := c.listenersLocked()
	c.mu.Unlock()

	notify(listeners, event)
	return snapshot, nil
}

func (c *Controller) CompleteRun() (Snapshot, error) {
	c.mu.Lock()
	c.initLocked()

	switch c.state {
	case StateIdle:
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrNoActiveRun
	case StateCompleted:
		snapshot := c.snapshotLocked()
		c.mu.Unlock()
		return snapshot, ErrRunCompleted
	}

	now := time.Now().UTC()
	c.state = StateCompleted
	c.reason = ""
	c.updatedAt = now
	c.pendingPermissions = make(map[string]string)

	event := c.eventLocked(EventRunCompleted, "", now, nil, "", "")
	snapshot := c.snapshotLocked()
	listeners := c.listenersLocked()
	c.mu.Unlock()

	notify(listeners, event)
	return snapshot, nil
}

func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	return c.state
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	return c.snapshotLocked()
}

func (c *Controller) AddListener(listener Listener) func() {
	if listener == nil {
		return func() {}
	}

	c.mu.Lock()
	c.initLocked()
	id := c.nextListenerID
	c.nextListenerID++
	c.listeners[id] = listener
	c.mu.Unlock()

	return func() {
		c.mu.Lock()
		delete(c.listeners, id)
		c.mu.Unlock()
	}
}

func (c *Controller) AddPermission(requestID, toolUseID string) error {
	requestID = strings.TrimSpace(requestID)
	toolUseID = strings.TrimSpace(toolUseID)
	if requestID == "" || toolUseID == "" {
		return ErrInvalidPermission
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.initLocked()
	if _, ok := c.pendingPermissions[requestID]; ok {
		return ErrPermissionExists
	}
	c.pendingPermissions[requestID] = toolUseID
	return nil
}

func (c *Controller) CancelPermission(requestID string) (string, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", ErrPermissionNotFound
	}

	c.mu.Lock()
	c.initLocked()
	toolUseID, ok := c.pendingPermissions[requestID]
	if !ok {
		c.mu.Unlock()
		return "", ErrPermissionNotFound
	}
	delete(c.pendingPermissions, requestID)

	now := time.Now().UTC()
	event := c.eventLocked(EventPermissionCancelled, "permission cancelled", now, nil, requestID, toolUseID)
	listeners := c.listenersLocked()
	c.mu.Unlock()

	notify(listeners, event)
	return toolUseID, nil
}

func (c *Controller) initLocked() {
	if c.state == "" {
		c.state = StateIdle
	}
	if c.pendingPermissions == nil {
		c.pendingPermissions = make(map[string]string)
	}
	if c.listeners == nil {
		c.listeners = make(map[int]Listener)
	}
}

func (c *Controller) snapshotLocked() Snapshot {
	pending := make(map[string]string, len(c.pendingPermissions))
	for requestID, toolUseID := range c.pendingPermissions {
		pending[requestID] = toolUseID
	}

	return Snapshot{
		RunID:              c.runID,
		State:              c.state,
		Reason:             c.reason,
		UpdatedAt:          c.updatedAt,
		ViewerOnly:         c.viewerOnly,
		ReadOnly:           c.readOnly,
		PendingPermissions: pending,
	}
}

func (c *Controller) eventLocked(kind EventKind, reason string, eventTime time.Time, err error, requestID, toolUseID string) Event {
	return Event{
		Kind:      kind,
		RunID:     c.runID,
		State:     c.state,
		Reason:    reason,
		Time:      eventTime,
		Err:       err,
		RequestID: requestID,
		ToolUseID: toolUseID,
	}
}

func (c *Controller) listenersLocked() []Listener {
	listeners := make([]Listener, 0, len(c.listeners))
	for _, listener := range c.listeners {
		listeners = append(listeners, listener)
	}
	return listeners
}

func notify(listeners []Listener, event Event) {
	for _, listener := range listeners {
		listener(event)
	}
}
