package autodream

import "time"

const (
	DefaultMinHours     = 24
	DefaultMinSessions  = 5
	DefaultScanThrottle = 10 * time.Minute

	MemoryIndexName     = "MEMORY.md"
	MaxMemoryIndexLines = 200
)

type Config struct {
	MinHours     float64
	MinSessions  int
	ScanThrottle time.Duration
}

func (c Config) normalized() Config {
	if c.MinHours <= 0 {
		c.MinHours = DefaultMinHours
	}
	if c.MinSessions <= 0 {
		c.MinSessions = DefaultMinSessions
	}
	if c.ScanThrottle <= 0 {
		c.ScanThrottle = DefaultScanThrottle
	}
	return c
}

type EnabledGates struct {
	KairosActive      bool
	RemoteMode        bool
	AutoMemoryEnabled bool
	AutoDreamEnabled  bool
}

func (g EnabledGates) ClosedReasons() []string {
	reasons := make([]string, 0, 4)
	if g.KairosActive {
		reasons = append(reasons, "kairos_active")
	}
	if g.RemoteMode {
		reasons = append(reasons, "remote_mode")
	}
	if !g.AutoMemoryEnabled {
		reasons = append(reasons, "auto_memory_disabled")
	}
	if !g.AutoDreamEnabled {
		reasons = append(reasons, "auto_dream_disabled")
	}
	return reasons
}

type State struct {
	LastSessionScanAt time.Time
}

type Inputs struct {
	Now              time.Time
	Config           Config
	State            State
	Gates            EnabledGates
	Forced           bool
	CurrentSessionID string
	MemoryRoot       string
	TranscriptDir    string
}

type Dependencies struct {
	ReadLast     func() (time.Time, error)
	ListSessions func(since time.Time) ([]string, error)
	AcquireLock  func() (LockAcquireResult, error)
}

type LockAcquireResult struct {
	Acquired   bool
	PriorMtime time.Time
}

type Status string

const (
	StatusFire              Status = "fire"
	StatusDisabled          Status = "disabled"
	StatusTimeGateClosed    Status = "time_gate_closed"
	StatusScanThrottled     Status = "scan_throttled"
	StatusNotEnoughSessions Status = "not_enough_sessions"
	StatusLockBusy          Status = "lock_busy"
	StatusReadLastError     Status = "read_last_error"
	StatusListSessionsError Status = "list_sessions_error"
	StatusLockAcquireError  Status = "lock_acquire_error"
)

type Decision struct {
	Status             Status
	Reason             string
	Forced             bool
	ClosedGates        []string
	HoursSince         float64
	MinHours           float64
	ScanThrottle       time.Duration
	LastConsolidatedAt time.Time
	SessionIDs         []string
	SessionsCount      int
	MinSessions        int
	PriorMtime         time.Time
	Prompt             string
	Plan               *ActionPlan
	NextState          State
}

func (d Decision) ShouldFire() bool {
	return d.Status == StatusFire
}

type ActionPlan struct {
	RegisterTask RegisterDreamTaskPlan
	ForkedAgent  ForkedAgentPlan
	Completion   CompletionPlan
	Steps        []string
}

type RegisterDreamTaskPlan struct {
	SessionsReviewing int
	PriorMtime        time.Time
}

type ForkedAgentPlan struct {
	QuerySource    string
	ForkLabel      string
	SkipTranscript bool
	CanUseTool     ToolAccessPlan
	Prompt         string
}

type ToolAccessPlan struct {
	Mode       string
	MemoryRoot string
}

type CompletionPlan struct {
	OnComplete string
	OnFail     string
	OnRollback string
}
