package envsnapshot

import (
	"sort"
	"strings"
)

type Snapshot struct {
	values map[string]string
}

type Options struct {
	ScrubSensitive      bool
	ProxyEnv            map[string]string
	AdditionalScrubKeys []string
}

type HookEnvFile struct {
	Event   string
	Index   int
	Content string
}

var defaultSensitiveKeys = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"ANTHROPIC_AUTH_TOKEN",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AZURE_CLIENT_SECRET",
	"OTEL_EXPORTER_OTLP_HEADERS",
	"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
	"ACTIONS_RUNTIME_TOKEN",
}

var hookEventPriority = map[string]int{
	"setup":        0,
	"sessionstart": 1,
	"cwdchanged":   2,
	"filechanged":  3,
}

func SnapshotFromMap(values map[string]string) Snapshot {
	return Snapshot{values: copyMap(values)}
}

func BuildSubprocessEnv(base Snapshot, opts Options) map[string]string {
	env := copyMap(base.values)
	for key, value := range opts.ProxyEnv {
		env[key] = value
	}

	if opts.ScrubSensitive {
		for _, key := range defaultSensitiveKeys {
			scrubKey(env, key)
		}
		for _, key := range opts.AdditionalScrubKeys {
			scrubKey(env, key)
		}
	}

	return env
}

func MergeSessionScripts(envFileScript string, hookFiles []HookEnvFile) string {
	var scripts []string
	if script := strings.TrimSpace(envFileScript); script != "" {
		scripts = append(scripts, script)
	}

	hooks := append([]HookEnvFile(nil), hookFiles...)
	sort.SliceStable(hooks, func(i, j int) bool {
		left := hooks[i]
		right := hooks[j]
		leftPriority := hookPriority(left.Event)
		rightPriority := hookPriority(right.Event)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return left.Index < right.Index
	})

	for _, hook := range hooks {
		if script := strings.TrimSpace(hook.Content); script != "" {
			scripts = append(scripts, script)
		}
	}

	return strings.Join(scripts, "\n")
}

func copyMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func scrubKey(env map[string]string, key string) {
	if key == "" {
		return
	}
	delete(env, key)
	delete(env, "INPUT_"+key)
}

func hookPriority(event string) int {
	if priority, ok := hookEventPriority[strings.ToLower(event)]; ok {
		return priority
	}
	return 99
}
