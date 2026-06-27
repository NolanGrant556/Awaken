package hookrouter

import (
	"errors"
	"reflect"
	"testing"
)

func TestRouterRoutesMatcherModes(t *testing.T) {
	tests := []struct {
		name        string
		matcher     string
		toolName    string
		wantIDs     []string
		wantMode    MatcherMode
		notToolName string
	}{
		{
			name:     "empty matcher matches all",
			toolName: "Bash",
			wantIDs:  []string{"hook"},
			wantMode: MatcherModeAll,
		},
		{
			name:        "regex requires full match",
			matcher:     "Bash|Read",
			toolName:    "Bash",
			wantIDs:     []string{"hook"},
			wantMode:    MatcherModeRegex,
			notToolName: "BashTool",
		},
		{
			name:        "bad regex falls back to literal equality",
			matcher:     "[bad",
			toolName:    "[bad",
			wantIDs:     []string{"hook"},
			wantMode:    MatcherModeLiteral,
			notToolName: "bad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			if err := router.Add(Hook{ID: "hook", Event: "pre_tool_call", Matcher: tt.matcher}); err != nil {
				t.Fatalf("Add returned error: %v", err)
			}

			got := hookIDs(router.Routes(RouteRequest{Event: "pre_tool_call", ToolName: tt.toolName}))
			if !reflect.DeepEqual(got, tt.wantIDs) {
				t.Fatalf("expected routes %v, got %v", tt.wantIDs, got)
			}

			snapshot := router.Snapshot()
			if len(snapshot) != 1 {
				t.Fatalf("expected one hook in snapshot, got %d", len(snapshot))
			}
			if snapshot[0].MatcherMode != tt.wantMode {
				t.Fatalf("expected matcher mode %q, got %q", tt.wantMode, snapshot[0].MatcherMode)
			}

			if tt.notToolName != "" {
				got = hookIDs(router.Routes(RouteRequest{Event: "pre_tool_call", ToolName: tt.notToolName}))
				if len(got) != 0 {
					t.Fatalf("expected no routes for tool %q, got %v", tt.notToolName, got)
				}
			}
		})
	}
}

func TestRouterRoutesPreserveAddOrder(t *testing.T) {
	router := NewRouter()
	for _, hook := range []Hook{
		{ID: "first", Event: "pre_tool_call"},
		{ID: "second", Event: "pre_tool_call"},
		{ID: "third", Event: "pre_tool_call"},
	} {
		if err := router.Add(hook); err != nil {
			t.Fatalf("Add(%q) returned error: %v", hook.ID, err)
		}
	}

	got := hookIDs(router.Routes(RouteRequest{Event: "pre_tool_call", ToolName: "Bash"}))
	want := []string{"first", "second", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected route order %v, got %v", want, got)
	}
}

func TestRouterRoutesFilterPluginAndSkill(t *testing.T) {
	router := NewRouter()
	for _, hook := range []Hook{
		{ID: "global", Event: "pre_tool_call"},
		{ID: "plugin", Event: "pre_tool_call", PluginName: "git"},
		{ID: "skill", Event: "pre_tool_call", SkillName: "review"},
		{ID: "plugin-skill", Event: "pre_tool_call", PluginName: "git", SkillName: "review"},
	} {
		if err := router.Add(hook); err != nil {
			t.Fatalf("Add(%q) returned error: %v", hook.ID, err)
		}
	}

	tests := []struct {
		name string
		req  RouteRequest
		want []string
	}{
		{
			name: "no plugin or skill only gets global hooks",
			req:  RouteRequest{Event: "pre_tool_call", ToolName: "Bash"},
			want: []string{"global"},
		},
		{
			name: "matching plugin includes plugin hooks",
			req:  RouteRequest{Event: "pre_tool_call", ToolName: "Bash", PluginName: "git"},
			want: []string{"global", "plugin"},
		},
		{
			name: "matching skill includes skill hooks",
			req:  RouteRequest{Event: "pre_tool_call", ToolName: "Bash", SkillName: "review"},
			want: []string{"global", "skill"},
		},
		{
			name: "matching plugin and skill includes both scoped hook",
			req:  RouteRequest{Event: "pre_tool_call", ToolName: "Bash", PluginName: "git", SkillName: "review"},
			want: []string{"global", "plugin", "skill", "plugin-skill"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hookIDs(router.Routes(tt.req))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected routes %v, got %v", tt.want, got)
			}
		})
	}
}

func TestRouterRoutesFilterEvent(t *testing.T) {
	router := NewRouter()
	if err := router.Add(Hook{ID: "pre", Event: "pre_tool_call"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if err := router.Add(Hook{ID: "post", Event: "post_tool_call"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	got := hookIDs(router.Routes(RouteRequest{Event: "post_tool_call", ToolName: "Bash"}))
	want := []string{"post"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected routes %v, got %v", want, got)
	}
}

func TestRouterRemove(t *testing.T) {
	router := NewRouter()
	for _, hook := range []Hook{
		{ID: "first", Event: "pre_tool_call"},
		{ID: "second", Event: "pre_tool_call"},
		{ID: "third", Event: "pre_tool_call"},
	} {
		if err := router.Add(hook); err != nil {
			t.Fatalf("Add(%q) returned error: %v", hook.ID, err)
		}
	}

	router.Remove("second")

	got := hookIDs(router.Routes(RouteRequest{Event: "pre_tool_call", ToolName: "Bash"}))
	want := []string{"first", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected routes after remove %v, got %v", want, got)
	}

	if err := router.Add(Hook{ID: "second", Event: "pre_tool_call"}); err != nil {
		t.Fatalf("expected removed ID to be reusable, got %v", err)
	}
}

func TestRouterSnapshotReturnsCopy(t *testing.T) {
	router := NewRouter()
	if err := router.Add(Hook{ID: "hook", Event: "pre_tool_call", Command: "echo ok"}); err != nil {
		t.Fatalf("Add returned error: %v", err)
	}

	snapshot := router.Snapshot()
	snapshot[0].ID = "changed"
	snapshot[0].Command = "changed"

	got := router.Snapshot()
	if got[0].ID != "hook" {
		t.Fatalf("expected stored hook ID to remain unchanged, got %q", got[0].ID)
	}
	if got[0].Command != "echo ok" {
		t.Fatalf("expected stored hook command to remain unchanged, got %q", got[0].Command)
	}
}

func TestRouterAddValidation(t *testing.T) {
	tests := []struct {
		name string
		hook Hook
		want error
	}{
		{
			name: "missing ID",
			hook: Hook{Event: "pre_tool_call"},
			want: ErrInvalidHook,
		},
		{
			name: "missing event",
			hook: Hook{ID: "hook"},
			want: ErrInvalidHook,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter()
			if err := router.Add(tt.hook); !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestRouterAddRejectsDuplicateID(t *testing.T) {
	router := NewRouter()
	if err := router.Add(Hook{ID: "hook", Event: "pre_tool_call"}); err != nil {
		t.Fatalf("first Add returned error: %v", err)
	}

	if err := router.Add(Hook{ID: "hook", Event: "post_tool_call"}); !errors.Is(err, ErrDuplicateHook) {
		t.Fatalf("expected ErrDuplicateHook, got %v", err)
	}
}

func hookIDs(hooks []Hook) []string {
	ids := make([]string, 0, len(hooks))
	for _, hook := range hooks {
		ids = append(ids, hook.ID)
	}
	return ids
}
