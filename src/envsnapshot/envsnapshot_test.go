package envsnapshot

import (
	"reflect"
	"testing"
)

func TestSnapshotFromMapCopiesInput(t *testing.T) {
	source := map[string]string{
		"PATH":           "/bin",
		"OPENAI_API_KEY": "secret",
	}
	snapshot := SnapshotFromMap(source)
	source["PATH"] = "/mutated"
	delete(source, "OPENAI_API_KEY")

	env := BuildSubprocessEnv(snapshot, Options{})
	want := map[string]string{
		"PATH":           "/bin",
		"OPENAI_API_KEY": "secret",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("BuildSubprocessEnv() = %#v, want %#v", env, want)
	}
}

func TestBuildSubprocessEnv(t *testing.T) {
	tests := []struct {
		name string
		base map[string]string
		opts Options
		want map[string]string
	}{
		{
			name: "scrubs sensitive keys and github action inputs",
			base: map[string]string{
				"PATH":                        "/bin",
				"ANTHROPIC_API_KEY":           "secret",
				"INPUT_ANTHROPIC_API_KEY":     "input-secret",
				"ACTIONS_RUNTIME_TOKEN":       "runtime-secret",
				"INPUT_ACTIONS_RUNTIME_TOKEN": "input-runtime-secret",
			},
			opts: Options{ScrubSensitive: true},
			want: map[string]string{
				"PATH": "/bin",
			},
		},
		{
			name: "proxy overlay wins over base",
			base: map[string]string{
				"HTTPS_PROXY": "http://base.proxy",
				"PATH":        "/bin",
			},
			opts: Options{
				ProxyEnv: map[string]string{
					"HTTPS_PROXY": "http://overlay.proxy",
					"NO_PROXY":    "127.0.0.1",
				},
			},
			want: map[string]string{
				"HTTPS_PROXY": "http://overlay.proxy",
				"NO_PROXY":    "127.0.0.1",
				"PATH":        "/bin",
			},
		},
		{
			name: "scrubs additional keys and github action inputs",
			base: map[string]string{
				"CUSTOM_SECRET":       "secret",
				"INPUT_CUSTOM_SECRET": "input-secret",
				"PATH":                "/bin",
			},
			opts: Options{
				ScrubSensitive:      true,
				AdditionalScrubKeys: []string{"CUSTOM_SECRET"},
			},
			want: map[string]string{
				"PATH": "/bin",
			},
		},
		{
			name: "keeps sensitive keys when scrub is disabled",
			base: map[string]string{
				"OPENAI_API_KEY":       "secret",
				"INPUT_OPENAI_API_KEY": "input-secret",
				"CUSTOM_SECRET":        "custom-secret",
				"PATH":                 "/bin",
			},
			opts: Options{
				ScrubSensitive:      false,
				AdditionalScrubKeys: []string{"CUSTOM_SECRET"},
			},
			want: map[string]string{
				"OPENAI_API_KEY":       "secret",
				"INPUT_OPENAI_API_KEY": "input-secret",
				"CUSTOM_SECRET":        "custom-secret",
				"PATH":                 "/bin",
			},
		},
		{
			name: "scrubs after applying proxy overlay",
			base: map[string]string{
				"PATH": "/bin",
			},
			opts: Options{
				ScrubSensitive: true,
				ProxyEnv: map[string]string{
					"OPENAI_API_KEY": "proxy-secret",
					"HTTP_PROXY":     "http://proxy",
				},
			},
			want: map[string]string{
				"HTTP_PROXY": "http://proxy",
				"PATH":       "/bin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSubprocessEnv(SnapshotFromMap(tt.base), tt.opts)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BuildSubprocessEnv() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMergeSessionScripts(t *testing.T) {
	tests := []struct {
		name          string
		envFileScript string
		hookFiles     []HookEnvFile
		want          string
	}{
		{
			name:          "orders hooks by event priority then index and skips empty content",
			envFileScript: "  export BASE=1  \n",
			hookFiles: []HookEnvFile{
				{Event: "filechanged", Index: 0, Content: " export FILE=1 "},
				{Event: "setup", Index: 2, Content: "export SETUP_2=1"},
				{Event: "cwdchanged", Index: 0, Content: " \n\t "},
				{Event: "setup", Index: 1, Content: "\nexport SETUP_1=1\n"},
				{Event: "sessionstart", Index: 0, Content: "export SESSION=1"},
				{Event: "cwdchanged", Index: 3, Content: "export CWD=1"},
			},
			want: "export BASE=1\nexport SETUP_1=1\nexport SETUP_2=1\nexport SESSION=1\nexport CWD=1\nexport FILE=1",
		},
		{
			name:          "returns empty string when all scripts are empty",
			envFileScript: " \n",
			hookFiles: []HookEnvFile{
				{Event: "setup", Index: 0, Content: ""},
				{Event: "sessionstart", Index: 0, Content: "\t"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeSessionScripts(tt.envFileScript, tt.hookFiles)
			if got != tt.want {
				t.Fatalf("MergeSessionScripts() = %q, want %q", got, tt.want)
			}
		})
	}
}
