package evalharness

import "time"

type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusBlocked Status = "blocked"
	StatusSkipped Status = "skipped"
)

type RealProviderRequirement struct {
	Required bool     `json:"required"`
	Provider string   `json:"provider,omitempty"`
	Model    string   `json:"model,omitempty"`
	EnvVars  []string `json:"env_vars,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

type Requirements struct {
	RealProvider *RealProviderRequirement `json:"real_provider,omitempty"`
}

func (r Requirements) RequiresRealProvider() bool {
	return r.RealProvider != nil && r.RealProvider.Required
}

type Scenario struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	Requirements Requirements `json:"requirements,omitempty"`
	Steps        []Step       `json:"steps"`
}

type Step struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Command      string        `json:"command"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	Requirements Requirements  `json:"requirements,omitempty"`
}

type Profile struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Scenarios   []Scenario `json:"scenarios"`
}

type ProviderAvailability struct {
	Ready    bool     `json:"ready"`
	Provider string   `json:"provider,omitempty"`
	Model    string   `json:"model,omitempty"`
	Missing  []string `json:"missing,omitempty"`
	Summary  string   `json:"summary,omitempty"`
}

type StepExecution struct {
	Status   Status            `json:"status,omitempty"`
	Summary  string            `json:"summary,omitempty"`
	Text     string            `json:"text,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type StepResult struct {
	StepID      string        `json:"step_id"`
	Name        string        `json:"name"`
	Status      Status        `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at"`
	Duration    time.Duration `json:"duration"`
	Summary     string        `json:"summary,omitempty"`
	Error       string        `json:"error,omitempty"`
	EvidenceIDs []string      `json:"evidence_ids,omitempty"`
}

type Result struct {
	ScenarioID   string        `json:"scenario_id"`
	Name         string        `json:"name"`
	Status       Status        `json:"status"`
	StartedAt    time.Time     `json:"started_at"`
	FinishedAt   time.Time     `json:"finished_at"`
	Duration     time.Duration `json:"duration"`
	Summary      string        `json:"summary,omitempty"`
	Requirements Requirements  `json:"requirements,omitempty"`
	Steps        []StepResult  `json:"steps,omitempty"`
	Evidence     []Evidence    `json:"evidence,omitempty"`
}

type Evidence struct {
	ID         string            `json:"id"`
	ScenarioID string            `json:"scenario_id"`
	StepID     string            `json:"step_id,omitempty"`
	Command    string            `json:"command"`
	Status     Status            `json:"status"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Duration   time.Duration     `json:"duration"`
	Summary    string            `json:"summary,omitempty"`
	Text       string            `json:"text,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
