package agentcore

import "time"

type ApprovalPolicy struct {
	TimeoutMs int `json:"timeout_ms,omitempty"`
}

func (p ApprovalPolicy) Normalize() ApprovalPolicy {
	if p.TimeoutMs < 0 {
		p.TimeoutMs = 0
	}
	return p
}

func (p ApprovalPolicy) Timeout() time.Duration {
	return time.Duration(p.Normalize().TimeoutMs) * time.Millisecond
}
