package subagentruntime

// Policy is a Host-owned execution policy, separate from model window limits.
// Zero fields inherit product defaults for every caller.
type Policy struct {
	Version              int `json:"version"`
	MaxActiveAgents      int `json:"max_active_agents"`
	MaxQueuedAssignments int `json:"max_queued_assignments"`
	MaxDepth             int `json:"max_depth"`
}

func (p Policy) Effective() Policy {
	if p.Version <= 0 {
		p.Version = 1
	}
	if p.MaxActiveAgents <= 0 {
		p.MaxActiveAgents = 8
	}
	if p.MaxQueuedAssignments <= 0 {
		p.MaxQueuedAssignments = 128
	}
	if p.MaxDepth <= 0 {
		p.MaxDepth = 8
	}
	return p
}
