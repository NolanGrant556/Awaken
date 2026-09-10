package researchruntime

import (
	"os"
	"strings"

	"awaken/src/subagentruntime"
)

const searchProvidersEnv = "AWAKEN_WEB_SEARCH_PROVIDERS"

var ChildToolAllowlist = []string{"Read", "Glob", "Grep", "WebSearch", "WebFetch", "tool_result_read"}

var ChildToolDenylist = []string{
	"Bash", "Edit", "Write", "trash_file", "artifact_publish", "subagent", "ask_user",
	"update_plan", "memory", "delegate_external_agent", "configure_hook", "configure_acp_runner",
	"git_remote", "dependency_install", "archived_history", "plugin", "skill", "skill_install",
	"skill_package_export", "skill_resource_read", "computer_observe", "computer_act", "computer_verify",
}

func SearchChannelsEnabled() bool {
	configured, ok := os.LookupEnv(searchProvidersEnv)
	if !ok {
		return true
	}
	return strings.TrimSpace(configured) != ""
}

func ChildToolAllowlistFor(hint SourceHint, searchEnabled bool) []string {
	allow := []string{"Read", "Glob", "Grep", "tool_result_read"}
	if hint != SourceWorkspace && searchEnabled {
		allow = append(allow, "WebSearch", "WebFetch")
	}
	return allow
}

func ChildHostSpawnOptions(hint SourceHint) *subagentruntime.HostSpawnOptions {
	return &subagentruntime.HostSpawnOptions{
		ToolAllowlist:     ChildToolAllowlistFor(hint, SearchChannelsEnabled()),
		ExtraHidden:       append([]string(nil), ChildToolDenylist...),
		MaxThoughts:       ChildMaxThoughts,
		MaxToolExecutions: ChildMaxToolCalls,
		SkipMemory:        true,
	}
}
