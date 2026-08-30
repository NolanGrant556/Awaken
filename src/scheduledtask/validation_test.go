package scheduledtask

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func validTask() Task {
	return Task{
		Name:      "工作日科技新闻早报",
		Enabled:   true,
		ProjectID: "project_1",
		Schedule:  cronSchedule("30 9 * * 1-5", "Asia/Shanghai"),
		Action:    Action{Kind: ActionKindAgent, Content: "整理今天最值得关注的科技新闻"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 600},
	}
}

func validatorAt(now time.Time) Validator {
	return Validator{Clock: NewFakeClock(now)}
}

func mustInvalid(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error on %s", field)
	}
	if !errors.Is(err, ErrValidationFailed) {
		t.Fatalf("error %v does not unwrap to ErrValidationFailed", err)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error %v is not a ValidationError", err)
	}
	if verr.Field != field {
		t.Fatalf("field = %q, want %q (issue %q)", verr.Field, field, verr.Issue)
	}
}

func TestValidateTaskValid(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	if err := v.ValidateTask(validTask()); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
}

func TestValidateCronFieldCounts(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	for _, expr := range []string{"", "0 9 *", "0 9 * *", "0 9 * * * 1 2", "   "} {
		task := validTask()
		task.Schedule.Cron = expr
		mustInvalid(t, v.ValidateTask(task), "schedule.cron")
	}
	for _, expr := range []string{"30 9 * * 1-5", "*/25 9-17 * * 1-5", "0 10 * * 6,0"} {
		task := validTask()
		task.Schedule.Cron = expr
		if err := v.ValidateTask(task); err != nil {
			t.Fatalf("five-field cron %q rejected: %v", expr, err)
		}
	}
}

func TestValidateCronDOWNormalization(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	// 0/7/sun are all Sunday; English DOW and month names are supported.
	for _, expr := range []string{
		"0 9 * * 0", "0 9 * * 7", "0 9 * * sun",
		"0 9 * * mon-fri", "0 9 * jan mon", "0 9 15 * *",
		"0 9 * * 5-7", // range ending at Sunday wraps onto the next week
	} {
		task := validTask()
		task.Schedule.Cron = expr
		if err := v.ValidateTask(task); err != nil {
			t.Fatalf("cron %q rejected: %v", expr, err)
		}
	}
	// Out-of-range DOW values are rejected even after normalization.
	for _, expr := range []string{"0 9 * * 8", "0 9 * * 70"} {
		task := validTask()
		task.Schedule.Cron = expr
		mustInvalid(t, v.ValidateTask(task), "schedule.cron")
	}
}

func TestValidateCronRejectsDescriptorShorthand(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	for _, expr := range []string{"@daily", "@every 1h"} {
		task := validTask()
		task.Schedule.Cron = expr
		mustInvalid(t, v.ValidateTask(task), "schedule.cron")
	}
}

func TestValidateTimezone(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	task := validTask()
	task.Schedule.Timezone = "Mars/Olympus"
	mustInvalid(t, v.ValidateTask(task), "schedule.timezone")
	task.Schedule.Timezone = ""
	mustInvalid(t, v.ValidateTask(task), "schedule.timezone")
	task.Schedule.Timezone = "America/New_York"
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("valid IANA timezone rejected: %v", err)
	}
}

// A custom cron with no future occurrence inside the search window is
// rejected (contract §6.3).
func TestValidateCronNoFutureOccurrence(t *testing.T) {
	v := validatorAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	task := validTask()
	task.Schedule.Cron = "0 0 30 2 *" // February 30 never happens
	mustInvalid(t, v.ValidateTask(task), "schedule.cron")
}

func TestValidateName(t *testing.T) {
	if err := ValidateName(""); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := ValidateName(strings.Repeat("a", 121)); err == nil {
		t.Fatal("121-rune name accepted")
	}
	// 120 CJK runes exceed 120 *bytes* but fit the rune limit.
	if err := ValidateName(strings.Repeat("任", 120)); err != nil {
		t.Fatalf("120 CJK runes rejected: %v", err)
	}
	if err := ValidateName(strings.Repeat("任", 121)); err == nil {
		t.Fatal("121 CJK runes accepted")
	}
}

func TestValidateAction(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	task := validTask()
	task.Action.Content = ""
	mustInvalid(t, v.ValidateTask(task), "action.content")
	task.Action.Content = strings.Repeat("x", MaxContentBytes+1)
	mustInvalid(t, v.ValidateTask(task), "action.content")
	task.Action.Content = strings.Repeat("x", MaxContentBytes)
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("32 KiB content rejected: %v", err)
	}
	task.Action.Kind = "webhook"
	mustInvalid(t, v.ValidateTask(task), "action.kind")
}

func TestValidateAgentComposerTokens(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	token := ComposerToken{
		ID:          "skill:daily-report:project",
		Type:        "skill",
		LeadingText: "请使用 ",
		Label:       "daily-report",
		Payload: ComposerTokenPayload{
			SkillID:      "daily-report",
			SourceOrigin: "project",
		},
	}

	task := validTask()
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{token}}
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("token-only Agent action rejected: %v", err)
	}
	if got := task.Action.EffectivePrompt(); got != "请使用" {
		t.Fatalf("effective prompt = %q, want %q", got, "请使用")
	}
	pluginToken := ComposerToken{
		ID: "plugin:acme.tools:digest-v1", Type: "plugin", Label: "acme.tools",
		Payload: ComposerTokenPayload{PluginName: "acme.tools", PluginDigest: "digest-v1"},
	}
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{pluginToken}}
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("Plugin token Agent action rejected: %v", err)
	}
	pluginToken.Payload.PluginDigest = ""
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{pluginToken}}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")
	for _, structured := range []ComposerToken{
		{ID: "rule:acme:strict", Type: "rule", Label: "strict", Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest-v1", RuleLogicalID: "rule/rules/strict.mdc"}},
		{ID: "command:acme:review", Type: "command", Label: "review", Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest-v1", CommandLogicalID: "command/commands/review.md", CommandArgumentsJSON: `["app.ts"]`}},
		{ID: "agent:acme:reviewer", Type: "agent", Label: "reviewer", Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest-v1", AgentLogicalID: "agent/agents/reviewer.md"}},
	} {
		task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{structured}}
		if err := v.ValidateTask(task); err != nil {
			t.Fatalf("%s token Agent action rejected: %v", structured.Type, err)
		}
	}
	invalidCommand := ComposerToken{ID: "command:bad", Type: "command", Label: "bad", Payload: ComposerTokenPayload{PluginName: "acme", PluginDigest: "digest-v1", CommandLogicalID: "command/commands/bad.md", CommandArgumentsJSON: "not-json"}}
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{invalidCommand}}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")
	computerUseToken := ComposerToken{
		ID: "capability:computer-use", Type: "capability", Label: "Computer Use",
		Payload: ComposerTokenPayload{CapabilityID: "computer-use"},
	}
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{computerUseToken}}
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("Computer Use capability token Agent action rejected: %v", err)
	}
	computerUseToken.Payload.CapabilityID = "arbitrary-hidden-prompt"
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: []ComposerToken{computerUseToken}}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")

	task.Action.Kind = ActionKindReminder
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")

	task.Action = Action{
		Kind: ActionKindAgent,
		ComposerTokens: []ComposerToken{{
			ID: "file:brief", Type: "file", Label: "brief",
			Payload: ComposerTokenPayload{SkillID: "daily-report"},
		}},
	}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")

	task.Action = Action{
		Kind:           ActionKindAgent,
		ComposerTokens: []ComposerToken{token, token},
	}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")

	tooMany := make([]ComposerToken, MaxComposerTokens+1)
	for index := range tooMany {
		tooMany[index] = token
		tooMany[index].ID = fmt.Sprintf("skill:daily-report-%d:project", index)
		tooMany[index].Payload.SkillID = fmt.Sprintf("daily-report-%d", index)
	}
	task.Action = Action{Kind: ActionKindAgent, ComposerTokens: tooMany}
	mustInvalid(t, v.ValidateTask(task), "action.composer_tokens")
}

func TestValidateCalendarSchedule(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	runAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	calendar := func(mutate func(*Schedule)) Task {
		task := validTask()
		task.Schedule = Schedule{Kind: ScheduleKindCalendar, Timezone: "Asia/Shanghai", RunAt: &runAt}
		if mutate != nil {
			mutate(&task.Schedule)
		}
		return task
	}
	// Non-repeating carries no repeat fields.
	if err := v.ValidateTask(calendar(nil)); err != nil {
		t.Fatalf("non-repeating calendar rejected: %v", err)
	}
	// Non-repeating must not carry repeat fields.
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEnd = &RepeatEnd{Kind: RepeatEndNever}
	})), "schedule.repeat_end")
	// Every N days with never/until/count ends.
	if err := v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 1
	})); err != nil {
		t.Fatalf("repeat without explicit end (never) rejected: %v", err)
	}
	if err := v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 7
		s.RepeatEnd = &RepeatEnd{Kind: RepeatEndNever}
	})); err != nil {
		t.Fatalf("never repeat end rejected: %v", err)
	}
	until := runAt.Add(8 * 7 * 24 * time.Hour)
	if err := v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 7
		s.RepeatEnd = &RepeatEnd{Kind: RepeatEndUntil, Until: &until}
	})); err != nil {
		t.Fatalf("until repeat end rejected: %v", err)
	}
	// until must be strictly later than run_at.
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 7
		s.RepeatEnd = &RepeatEnd{Kind: RepeatEndUntil, Until: &runAt}
	})), "schedule.repeat_end.until")
	before := runAt.Add(-time.Hour)
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 7
		s.RepeatEnd = &RepeatEnd{Kind: RepeatEndUntil, Until: &before}
	})), "schedule.repeat_end.until")
	// count range 1-10000.
	for _, count := range []int{0, -1, 10001} {
		mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
			s.RepeatEveryDays = 1
			s.RepeatEnd = &RepeatEnd{Kind: RepeatEndCount, Count: count}
		})), "schedule.repeat_end.count")
	}
	for _, count := range []int{1, 14, 10000} {
		if err := v.ValidateTask(calendar(func(s *Schedule) {
			s.RepeatEveryDays = 1
			s.RepeatEnd = &RepeatEnd{Kind: RepeatEndCount, Count: count}
		})); err != nil {
			t.Fatalf("count %d rejected: %v", count, err)
		}
	}
	// repeat_every_days must be >= 1 when repeating.
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = -2
	})), "schedule.repeat_every_days")
	// run_at is required.
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RunAt = nil
	})), "schedule.run_at")
	// unknown kinds.
	mustInvalid(t, v.ValidateTask(calendar(func(s *Schedule) {
		s.RepeatEveryDays = 1
		s.RepeatEnd = &RepeatEnd{Kind: "weekly"}
	})), "schedule.repeat_end.kind")
	task := validTask()
	task.Schedule.Kind = "hourly"
	mustInvalid(t, v.ValidateTask(task), "schedule.kind")
}

func TestValidateExecutionOptions(t *testing.T) {
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	for _, timeout := range []int{0, 29, 3601} {
		task := validTask()
		task.Execution.TimeoutSeconds = timeout
		mustInvalid(t, v.ValidateTask(task), "execution.timeout_seconds")
	}
	for _, timeout := range []int{30, 600, 3600} {
		task := validTask()
		task.Execution.TimeoutSeconds = timeout
		if err := v.ValidateTask(task); err != nil {
			t.Fatalf("timeout %d rejected: %v", timeout, err)
		}
	}
	for _, grace := range []int{-1, 86401} {
		task := validTask()
		task.Execution.MisfireGraceSeconds = grace
		mustInvalid(t, v.ValidateTask(task), "execution.misfire_grace_seconds")
	}
	for _, grace := range []int{0, 600, 86400} {
		task := validTask()
		task.Execution.MisfireGraceSeconds = grace
		if err := v.ValidateTask(task); err != nil {
			t.Fatalf("grace %d rejected: %v", grace, err)
		}
	}
}

func TestValidateProjectAndProfileCheckers(t *testing.T) {
	checkerErr := errors.New("injected failure")
	v := Validator{
		Clock:        NewFakeClock(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)),
		CheckProject: func(string) error { return checkerErr },
	}
	mustInvalid(t, v.ValidateTask(validTask()), "project_id")

	v.CheckProject = func(string) error { return nil }
	v.CheckProfile = func(string) error { return checkerErr }
	task := validTask()
	task.Execution.ProfileID = "profile_explicit"
	mustInvalid(t, v.ValidateTask(task), "execution.profile_id")
	// Empty profile resolves the default at trigger time and is not checked.
	task.Execution.ProfileID = ""
	if err := v.ValidateTask(task); err != nil {
		t.Fatalf("empty profile id must skip the profile checker: %v", err)
	}
}

// Task content is user business data: it must never appear in error
// messages (contract §7).
func TestValidationErrorNeverContainsContent(t *testing.T) {
	secret := "SECRET-CONTENT-9f3k2m-do-not-leak"
	v := validatorAt(time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC))
	invalidTasks := []func() Task{
		func() Task { task := validTask(); task.Name = ""; task.Action.Content = secret; return task },
		func() Task {
			task := validTask()
			task.Action.Content = secret
			task.Schedule.Cron = "bogus"
			return task
		},
		func() Task {
			task := validTask()
			task.Action.Content = secret
			task.Schedule.Timezone = "Nowhere"
			return task
		},
		func() Task {
			task := validTask()
			task.Action.Content = secret
			task.Execution.TimeoutSeconds = 1
			return task
		},
	}
	for i, build := range invalidTasks {
		err := v.ValidateTask(build())
		if err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("case %d: error message leaks task content: %q", i, err.Error())
		}
	}
}
