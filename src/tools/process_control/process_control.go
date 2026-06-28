package processcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/processes"
	"awaken/src/tool"
)

const (
	defaultWaitTimeoutMS = 10000
	maxWaitTimeoutMS     = 120000
)

type ProcessControlTool struct {
	registry *processes.Registry
}

type processControlParams struct {
	Action       string   `json:"action"`
	SessionID    string   `json:"session_id"`
	Command      string   `json:"command"`
	Workdir      string   `json:"workdir"`
	Stdin        string   `json:"stdin"`
	Background   *bool    `json:"background"`
	TimeoutMS    int      `json:"timeout_ms"`
	MaxLogChars  int      `json:"max_log_chars"`
	EnvAllowlist []string `json:"env_allowlist"`
}

type processControlResponse struct {
	Action       string                      `json:"action"`
	Session      *processes.SessionSnapshot  `json:"session,omitempty"`
	Sessions     []processes.SessionSnapshot `json:"sessions,omitempty"`
	Log          *processes.ProcessLog       `json:"log,omitempty"`
	WrittenBytes int                         `json:"written_bytes,omitempty"`
	TimedOut     bool                        `json:"timed_out,omitempty"`
	Mode         string                      `json:"mode,omitempty"`
}

func NewProcessControlTool(registry *processes.Registry) *ProcessControlTool {
	if registry == nil {
		registry = processes.DefaultRegistry
	}
	return &ProcessControlTool{registry: registry}
}

func (t *ProcessControlTool) Name() string {
	return "process_control"
}

func (t *ProcessControlTool) Description() string {
	return "管理 Awaken 启动的 pipe 模式后台进程：start、list、log、write、kill、wait；不是完整 PTY 终端模拟器"
}

func (t *ProcessControlTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "action", Description: "操作类型：start、list、log、write、kill、wait", Type: "string", Required: true},
		{Name: "session_id", Description: "进程 session id，log/write/kill/wait 需要", Type: "string", Required: false},
		{Name: "command", Description: "start 要执行的 shell command", Type: "string", Required: false},
		{Name: "workdir", Description: "start 工作目录，默认 workspace root，必须在 workspace 内", Type: "string", Required: false},
		{Name: "stdin", Description: "write 写入进程 stdin 的文本", Type: "string", Required: false},
		{Name: "background", Description: "start 是否后台运行，默认 true；false 会按 timeout_ms 等待", Type: "boolean", Required: false},
		{Name: "timeout_ms", Description: "wait 或 foreground start 的等待上限，默认 10000，最大 120000", Type: "integer", Required: false},
		{Name: "max_log_chars", Description: "start 日志缓冲或 log 返回的最大字符数", Type: "integer", Required: false},
		{Name: "env_allowlist", Description: "允许继承的非敏感环境变量名列表；敏感名称会被过滤", Type: "array", Required: false},
	}
}

func (t *ProcessControlTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	action := normalizeAction(params.Action)
	if action == "" {
		return "", fmt.Errorf("缺少必需参数: action")
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	var response processControlResponse
	response.Action = action
	response.Mode = processes.ModePipe

	switch action {
	case "start":
		session, timedOut, err := t.start(ctx, params, permissionContext)
		if err != nil {
			return "", err
		}
		response.Session = &session
		response.TimedOut = timedOut
	case "list":
		response.Sessions = t.registry.List()
	case "log":
		log, err := t.registry.GetLog(params.SessionID, params.MaxLogChars)
		if err != nil {
			return "", err
		}
		response.Log = &log
	case "write":
		if strings.TrimSpace(params.SessionID) == "" {
			return "", fmt.Errorf("session_id is required for write")
		}
		n, err := t.registry.WriteStdin(ctx, params.SessionID, params.Stdin)
		if err != nil {
			return "", err
		}
		response.WrittenBytes = n
	case "kill":
		if strings.TrimSpace(params.SessionID) == "" {
			return "", fmt.Errorf("session_id is required for kill")
		}
		session, err := t.registry.Kill(ctx, params.SessionID)
		if err != nil {
			return "", err
		}
		response.Session = &session
	case "wait":
		if strings.TrimSpace(params.SessionID) == "" {
			return "", fmt.Errorf("session_id is required for wait")
		}
		waited, err := t.registry.Wait(ctx, params.SessionID, waitTimeout(params.TimeoutMS))
		if err != nil {
			return "", err
		}
		response.Session = &waited.Session
		response.TimedOut = waited.TimedOut
	default:
		return "", fmt.Errorf("unsupported process_control action %q", params.Action)
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *ProcessControlTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}
	params, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}

	switch normalizeAction(params.Action) {
	case "start":
		if strings.TrimSpace(params.Command) == "" {
			return tool.DenyPermission("command is required for process_control start")
		}
		if strings.TrimSpace(params.Workdir) != "" && !permissionContext.IsWithinWorkspaceResolved(params.Workdir) {
			return tool.DenyPermission(fmt.Sprintf("workdir %s is outside workspace or escapes via symlink", params.Workdir))
		}
		reason, riskTone := processStartPermissionReason(params.Command)
		if reason == "" {
			return tool.AllowPermission()
		}
		return tool.AskPermission(reason, tool.ApprovalRequest{
			Title:       "Approve process start",
			Description: processApprovalDescription(params.Command, params.Workdir, reason),
			RiskTone:    riskTone,
		})
	case "kill":
		if strings.TrimSpace(params.SessionID) == "" {
			return tool.DenyPermission("session_id is required for process_control kill")
		}
		return tool.AskPermission("process_control will terminate a managed background process", tool.ApprovalRequest{
			Title:       "Approve process kill",
			Description: "Terminate process session " + strings.TrimSpace(params.SessionID),
			RiskTone:    "warning",
		})
	case "write":
		if strings.TrimSpace(params.SessionID) == "" {
			return tool.DenyPermission("session_id is required for process_control write")
		}
		return tool.AskPermission("process_control will send input to a managed process", tool.ApprovalRequest{
			Title:       "Approve process input",
			Description: "Write stdin to process session " + strings.TrimSpace(params.SessionID),
			RiskTone:    "warning",
		})
	default:
		return tool.AllowPermission()
	}
}

func (t *ProcessControlTool) start(
	ctx context.Context,
	params processControlParams,
	permissionContext tool.PermissionContext,
) (processes.SessionSnapshot, bool, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		return processes.SessionSnapshot{}, false, fmt.Errorf("command is required for start")
	}

	background := true
	if params.Background != nil {
		background = *params.Background
	}

	session, err := t.registry.Start(ctx, processes.StartOptions{
		Command:           command,
		Workdir:           params.Workdir,
		EnvAllowlist:      params.EnvAllowlist,
		Background:        true,
		MaxLogChars:       params.MaxLogChars,
		PermissionContext: permissionContext,
	})
	if err != nil {
		return processes.SessionSnapshot{}, false, err
	}
	if background {
		return session, false, nil
	}

	waited, err := t.registry.Wait(ctx, session.ID, waitTimeout(params.TimeoutMS))
	if err != nil {
		return waited.Session, waited.TimedOut, err
	}
	return waited.Session, waited.TimedOut, nil
}

func parseParams(input string) (processControlParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return processControlParams{}, nil
	}

	var params processControlParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return processControlParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func normalizeAction(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}

func waitTimeout(value int) time.Duration {
	if value <= 0 {
		value = defaultWaitTimeoutMS
	}
	if value > maxWaitTimeoutMS {
		value = maxWaitTimeoutMS
	}
	return time.Duration(value) * time.Millisecond
}

func processStartPermissionReason(command string) (string, string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", ""
	}
	if isDangerousProcessCommand(command) {
		return "process_control start may mutate files, change repository state, or access external systems", "danger"
	}
	if hasProcessControlOperator(command) {
		return "process_control start contains shell operators, redirections, substitutions, or multiple operations", "warning"
	}
	if isSimpleReadOnlyProcessCommand(command) {
		return "", ""
	}
	return "process_control start is not classified as a simple read-only process", "warning"
}

func processApprovalDescription(command string, workdir string, reason string) string {
	var builder strings.Builder
	builder.WriteString(reason)
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(command))
	if strings.TrimSpace(workdir) != "" {
		builder.WriteString("\n\nworkdir: ")
		builder.WriteString(strings.TrimSpace(workdir))
	}
	return builder.String()
}

func hasProcessControlOperator(command string) bool {
	for _, marker := range []string{"\n", "&&", "||", ";", "|", ">", "<", "`", "$("} {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func isSimpleReadOnlyProcessCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return true
	}
	base := filepath.Base(fields[0])
	switch base {
	case "pwd", "printf", "echo", "date", "whoami":
		return true
	case "cat":
		return len(fields) == 1
	case "sleep":
		return len(fields) == 2
	case "ls":
		return onlyFlags(fields[1:])
	default:
		return false
	}
}

func onlyFlags(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return false
		}
	}
	return true
}

func isDangerousProcessCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	switch base {
	case "rm", "mv", "cp", "mkdir", "rmdir", "chmod", "chown", "chgrp", "touch", "ln", "sudo", "doas", "pkexec", "curl", "wget", "ssh", "scp", "rsync":
		return true
	case "git":
		return containsAnyToken(fields[1:], "push", "reset", "clean", "checkout", "switch", "commit", "merge", "rebase", "pull", "fetch", "restore")
	default:
		return false
	}
}

func containsAnyToken(values []string, candidates ...string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func init() {
	tool.MustRegisterTool(NewProcessControlTool(processes.DefaultRegistry))
}
