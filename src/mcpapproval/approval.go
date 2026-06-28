package mcpapproval

import (
	"regexp"
	"sort"
	"strings"
)

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
	ScopeLocal   Scope = "local"
	ScopeDynamic Scope = "dynamic"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusUnknown  Status = "unknown"
)

type DecisionType string

const (
	DecisionApprove DecisionType = "approve"
	DecisionReject  DecisionType = "reject"
	DecisionDefer   DecisionType = "defer"
)

type PlanKind string

const (
	PlanNone        PlanKind = "none"
	PlanSingle      PlanKind = "single"
	PlanMultiselect PlanKind = "multiselect"
)

type ActionKind string

const (
	ActionUpdateStatus ActionKind = "update_status"
	ActionNoChange     ActionKind = "no_change"
)

type ActionReason string

const (
	ReasonApproved         ActionReason = "approved"
	ReasonRejected         ActionReason = "rejected"
	ReasonDeferred         ActionReason = "deferred"
	ReasonUnknownServer    ActionReason = "unknown_server"
	ReasonNonProject       ActionReason = "non_project"
	ReasonAlreadyDecided   ActionReason = "already_decided"
	ReasonStatusNotPending ActionReason = "status_not_pending"
	ReasonInvalidDecision  ActionReason = "invalid_decision"
)

type RiskCode string

const (
	RiskDangerousCommand RiskCode = "dangerous_command"
	RiskSensitiveEnv     RiskCode = "sensitive_env"
	RiskRuntimeEnv       RiskCode = "runtime_env"
)

type ServerConfig struct {
	Name    string
	Scope   Scope
	Status  Status
	Command string
	Args    []string
	Env     map[string]string
}

type ServerApproval struct {
	Name      string
	Scope     Scope
	Status    Status
	Command   []string
	RiskHints []RiskHint
}

type ApprovalPlan struct {
	Kind             PlanKind
	Servers          []ServerApproval
	AllowedDecisions []DecisionType
}

type Decision struct {
	ServerName string
	Decision   DecisionType
}

type StatusUpdateAction struct {
	Kind       ActionKind
	ServerName string
	Known      bool
	Scope      Scope
	Decision   DecisionType
	From       Status
	To         Status
	Reason     ActionReason
}

type DecisionResult struct {
	Actions            []StatusUpdateAction
	DuplicateDecisions []Decision
}

type RiskHint struct {
	Code    RiskCode
	Field   string
	Message string
}

func BuildApprovalPlan(configs []ServerConfig) ApprovalPlan {
	servers := pendingProjectApprovals(configs)
	kind := PlanNone
	if len(servers) == 1 {
		kind = PlanSingle
	} else if len(servers) > 1 {
		kind = PlanMultiselect
	}

	plan := ApprovalPlan{
		Kind:    kind,
		Servers: servers,
	}
	if kind != PlanNone {
		plan.AllowedDecisions = []DecisionType{
			DecisionApprove,
			DecisionReject,
			DecisionDefer,
		}
	}
	return plan
}

func ResolveDecisions(configs []ServerConfig, decisions []Decision) DecisionResult {
	byName := make(map[string]ServerConfig, len(configs))
	for _, config := range configs {
		if _, exists := byName[config.Name]; !exists {
			byName[config.Name] = config
		}
	}

	result := DecisionResult{
		Actions: make([]StatusUpdateAction, 0, len(decisions)),
	}
	seen := map[string]struct{}{}
	for _, decision := range decisions {
		name := strings.TrimSpace(decision.ServerName)
		if _, exists := seen[name]; exists {
			result.DuplicateDecisions = append(result.DuplicateDecisions, decision)
			continue
		}
		seen[name] = struct{}{}

		config, ok := byName[name]
		if !ok {
			result.Actions = append(result.Actions, StatusUpdateAction{
				Kind:       ActionNoChange,
				ServerName: name,
				Known:      false,
				Decision:   decision.Decision,
				From:       StatusUnknown,
				To:         StatusUnknown,
				Reason:     ReasonUnknownServer,
			})
			continue
		}

		action := StatusUpdateAction{
			Kind:       ActionNoChange,
			ServerName: config.Name,
			Known:      true,
			Scope:      config.Scope,
			Decision:   decision.Decision,
			From:       config.Status,
			To:         config.Status,
		}

		switch {
		case config.Scope != ScopeProject:
			action.Reason = ReasonNonProject
		case config.Status == StatusApproved || config.Status == StatusRejected:
			action.Reason = ReasonAlreadyDecided
		case config.Status != StatusPending:
			action.Reason = ReasonStatusNotPending
		default:
			switch decision.Decision {
			case DecisionApprove:
				action.Kind = ActionUpdateStatus
				action.To = StatusApproved
				action.Reason = ReasonApproved
			case DecisionReject:
				action.Kind = ActionUpdateStatus
				action.To = StatusRejected
				action.Reason = ReasonRejected
			case DecisionDefer:
				action.Reason = ReasonDeferred
			default:
				action.Reason = ReasonInvalidDecision
			}
		}

		result.Actions = append(result.Actions, action)
	}

	return result
}

func RiskHints(config ServerConfig) []RiskHint {
	hints := commandRiskHints(config)
	hints = append(hints, envRiskHints(config.Env)...)
	return hints
}

func pendingProjectApprovals(configs []ServerConfig) []ServerApproval {
	servers := make([]ServerApproval, 0, len(configs))
	for _, config := range configs {
		if config.Scope != ScopeProject || config.Status != StatusPending {
			continue
		}
		servers = append(servers, ServerApproval{
			Name:      config.Name,
			Scope:     config.Scope,
			Status:    config.Status,
			Command:   commandArray(config),
			RiskHints: RiskHints(config),
		})
	}
	sort.SliceStable(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})
	return servers
}

func commandArray(config ServerConfig) []string {
	if config.Command == "" && len(config.Args) == 0 {
		return nil
	}
	command := make([]string, 0, len(config.Args)+1)
	if config.Command != "" {
		command = append(command, config.Command)
	}
	command = append(command, config.Args...)
	return command
}

var commandRiskPatterns = []struct {
	pattern *regexp.Regexp
	message string
}{
	{regexp.MustCompile(`\brm\s+(-[^\s]*r[^\s]*|--recursive)\b`), "recursive delete command"},
	{regexp.MustCompile(`\b(curl|wget)\b.*\|\s*(ba)?sh\b`), "remote download piped to shell"},
	{regexp.MustCompile(`\b(bash|sh|zsh|ksh)\s+-[^\s]*c\b`), "shell command via -c flag"},
	{regexp.MustCompile(`\b(python[23]?|perl|ruby|node)\s+-[ec]\b`), "inline script execution"},
	{regexp.MustCompile(`\bmkfs(\.[a-z0-9]+)?\b`), "filesystem format command"},
	{regexp.MustCompile(`\bdd\b.*\bof=/dev/`), "raw device write command"},
	{regexp.MustCompile(`\bchmod\b.*\b(777|666)\b`), "world-writable permission change"},
	{regexp.MustCompile(`\bchown\b.*\b-R\b.*\broot\b`), "recursive ownership change to root"},
	{regexp.MustCompile(`\bsystemctl\s+(-[^\s]+\s+)*(stop|restart|disable|mask)\b`), "system service mutation"},
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), "destructive git reset"},
	{regexp.MustCompile(`\bgit\s+push\b.*(--force|\s-f\b)`), "force push command"},
}

func commandRiskHints(config ServerConfig) []RiskHint {
	command := commandArray(config)
	if len(command) == 0 {
		return nil
	}
	line := strings.ToLower(strings.Join(command, " "))
	hints := []RiskHint{}

	if commandName(command[0]) == "sudo" {
		hints = append(hints, RiskHint{
			Code:    RiskDangerousCommand,
			Field:   "command",
			Message: "privileged command execution",
		})
	}
	for _, candidate := range commandRiskPatterns {
		if candidate.pattern.MatchString(line) {
			hints = append(hints, RiskHint{
				Code:    RiskDangerousCommand,
				Field:   "command",
				Message: candidate.message,
			})
		}
	}
	return hints
}

func commandName(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	command = strings.TrimRight(command, `/\`)
	if idx := strings.LastIndexAny(command, `/\`); idx >= 0 {
		command = command[idx+1:]
	}
	return strings.ToLower(command)
}

var runtimeEnvKeys = map[string]string{
	"BASH_ENV":              "shell startup file injection",
	"DYLD_INSERT_LIBRARIES": "dynamic library injection",
	"ENV":                   "shell startup file injection",
	"GIT_SSH_COMMAND":       "git ssh command override",
	"LD_PRELOAD":            "dynamic library injection",
	"NODE_OPTIONS":          "node runtime option injection",
	"PERL5OPT":              "perl runtime option injection",
	"PYTHONPATH":            "python import path override",
	"RUBYOPT":               "ruby runtime option injection",
}

var sensitiveEnvFragments = []string{
	"ACCESS_KEY",
	"API_KEY",
	"AUTHORIZATION",
	"CREDENTIAL",
	"PASSWORD",
	"PASSWD",
	"PRIVATE_KEY",
	"SECRET",
	"TOKEN",
}

func envRiskHints(env map[string]string) []RiskHint {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	hints := []RiskHint{}
	for _, key := range keys {
		upperKey := strings.ToUpper(key)
		if message, ok := runtimeEnvKeys[upperKey]; ok {
			hints = append(hints, RiskHint{
				Code:    RiskRuntimeEnv,
				Field:   "env." + key,
				Message: message,
			})
		}
		for _, fragment := range sensitiveEnvFragments {
			if strings.Contains(upperKey, fragment) {
				hints = append(hints, RiskHint{
					Code:    RiskSensitiveEnv,
					Field:   "env." + key,
					Message: "environment variable may contain credentials",
				})
				break
			}
		}
	}
	return hints
}
