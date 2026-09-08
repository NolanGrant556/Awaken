package scheduledtask

import (
	"testing"
	"time"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %s: %v", name, err)
	}
	return loc
}

func cronSchedule(expr, tz string) Schedule {
	return Schedule{Kind: ScheduleKindCron, Cron: expr, Timezone: tz}
}

func TestFakeClock(t *testing.T) {
	base := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	clock := NewFakeClock(base)
	if !clock.Now().Equal(base) {
		t.Fatalf("Now() = %v, want %v", clock.Now(), base)
	}
	clock.Advance(90 * time.Minute)
	if got := clock.Now(); !got.Equal(base.Add(90 * time.Minute)) {
		t.Fatalf("after Advance Now() = %v", got)
	}
	later := base.Add(48 * time.Hour)
	clock.Set(later)
	if got := clock.Now(); !got.Equal(later) {
		t.Fatalf("after Set Now() = %v", got)
	}
}

func TestNextOccurrenceCronPresets(t *testing.T) {
	base := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC) // a Sunday
	cases := []struct {
		name string
		expr string
		tz   string
		want time.Time
	}{
		{"hourly", "0 * * * *", "UTC", time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)},
		{"daily", "30 9 * * *", "UTC", time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)},
		{"weekly monday", "0 9 * * 1", "UTC", time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)},
		{"custom weekdays", "30 9 * * 1-5", "UTC", time.Date(2026, 7, 27, 9, 30, 0, 0, time.UTC)},
		{"custom step list", "*/25 9-17 * * 1-5", "UTC", time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)},
		{"weekend list", "0 10 * * 6,0", "UTC", time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NextOccurrence(cronSchedule(tc.expr, tc.tz), base)
			if !ok {
				t.Fatalf("no occurrence for %q", tc.expr)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("NextOccurrence(%q) = %v, want %v", tc.expr, got, tc.want)
			}
			if !got.After(base) {
				t.Fatalf("occurrence %v not strictly after base %v", got, base)
			}
		})
	}
}

func TestNextOccurrenceStrictlyAfterBase(t *testing.T) {
	// Base exactly on a cron tick must yield the next tick, not the base.
	onTick := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	got, ok := NextOccurrence(cronSchedule("0 * * * *", "UTC"), onTick)
	if !ok || !got.Equal(onTick.Add(time.Hour)) {
		t.Fatalf("cron on-tick next = %v ok=%v, want %v", got, ok, onTick.Add(time.Hour))
	}
	// Calendar: a base exactly on one occurrence yields the following one.
	runAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	sched := Schedule{
		Kind: ScheduleKindCalendar, Timezone: "UTC",
		RunAt: &runAt, RepeatEveryDays: 3,
	}
	got, ok = NextOccurrence(sched, runAt)
	if !ok || !got.Equal(runAt.AddDate(0, 0, 3)) {
		t.Fatalf("calendar on-occurrence next = %v ok=%v, want %v", got, ok, runAt.AddDate(0, 0, 3))
	}
}

func TestNextOccurrenceCronTimezoneShiftsAbsoluteTime(t *testing.T) {
	base := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC)
	shanghai, _ := NextOccurrence(cronSchedule("0 9 * * *", "Asia/Shanghai"), base)
	utc, _ := NextOccurrence(cronSchedule("0 9 * * *", "UTC"), base)
	if shanghai.Equal(utc) {
		t.Fatalf("timezone must change the absolute occurrence: both %v", shanghai)
	}
	if diff := utc.Sub(shanghai); diff != 8*time.Hour {
		t.Fatalf("UTC occurrence should trail Shanghai by 8h, got %v", diff)
	}
	loc := mustLocation(t, "Asia/Shanghai")
	if got := shanghai.In(loc); got.Hour() != 9 || got.Minute() != 0 {
		t.Fatalf("Shanghai occurrence not at 09:00 local: %v", got)
	}
}

func TestNextOccurrenceCronNoFutureOccurrence(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, ok := NextOccurrence(cronSchedule("0 0 30 2 *", "UTC"), base); ok {
		t.Fatalf("February 30 must have no occurrence, got %v", got)
	}
}

func TestNextOccurrenceCronDOWSundayAliases(t *testing.T) {
	base := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC) // Sunday 10:30
	want := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)    // next Sunday 09:00
	for _, expr := range []string{"0 9 * * 0", "0 9 * * 7", "0 9 * * sun"} {
		got, ok := NextOccurrence(cronSchedule(expr, "UTC"), base)
		if !ok || !got.Equal(want) {
			t.Fatalf("NextOccurrence(%q) = %v ok=%v, want %v", expr, got, ok, want)
		}
	}
}

// DST spring forward in America/New_York: 2026-03-08 jumps 02:00 -> 03:00,
// so 02:30 does not exist. The contract sends the task to the next legal
// occurrence computed by the scheduling library.
func TestNextOccurrenceCronDSTSpringForward(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	base := time.Date(2026, 3, 8, 0, 0, 0, 0, ny) // EST
	got, ok := NextOccurrence(cronSchedule("30 2 * * *", "America/New_York"), base)
	if !ok {
		t.Fatal("no occurrence after spring-forward base")
	}
	want := time.Date(2026, 3, 9, 2, 30, 0, 0, ny) // EDT, next legal occurrence
	if !got.Equal(want) {
		t.Fatalf("spring-forward next = %v, want %v", got, want)
	}
	if !got.After(base) {
		t.Fatalf("occurrence %v not after base %v", got, base)
	}
}

// DST fall back in America/New_York: 2026-11-01 repeats 01:30 twice. Each
// expression occurrence executes only once, so the occurrence after the
// first 01:30 is the next day, not the repeated instant.
func TestNextOccurrenceCronDSTFallBackRunsOnce(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, ny) // EDT
	first, ok := NextOccurrence(cronSchedule("30 1 * * *", "America/New_York"), base)
	if !ok {
		t.Fatal("no occurrence on fall-back day")
	}
	if want := time.Date(2026, 11, 1, 1, 30, 0, 0, ny); !first.Equal(want) {
		t.Fatalf("first fall-back occurrence = %v, want %v", first, want)
	}
	second, ok := NextOccurrence(cronSchedule("30 1 * * *", "America/New_York"), first)
	if !ok {
		t.Fatal("no occurrence after first fall-back tick")
	}
	if want := time.Date(2026, 11, 2, 1, 30, 0, 0, ny); !second.Equal(want) {
		t.Fatalf("repeated local time must not fire twice: got %v, want %v", second, want)
	}
}

// Calendar repeats keep the local wall-clock time across DST (the offset is
// normalized by the time package).
func TestNextOccurrenceCalendarDSTKeepsWallClock(t *testing.T) {
	ny := mustLocation(t, "America/New_York")
	runAt := time.Date(2026, 3, 7, 9, 0, 0, 0, ny) // EST (UTC-5)
	sched := Schedule{
		Kind: ScheduleKindCalendar, Timezone: "America/New_York",
		RunAt: &runAt, RepeatEveryDays: 1,
	}
	after := time.Date(2026, 3, 8, 12, 0, 0, 0, ny) // EDT, after spring forward
	got, ok := NextOccurrence(sched, after)
	if !ok {
		t.Fatal("no calendar occurrence across DST")
	}
	local := got.In(ny)
	if local.Hour() != 9 || local.Minute() != 0 {
		t.Fatalf("wall clock not preserved across DST: %v", local)
	}
	if want := time.Date(2026, 3, 9, 9, 0, 0, 0, ny); !got.Equal(want) {
		t.Fatalf("calendar DST occurrence = %v, want %v", got, want)
	}
}

func TestNextOccurrenceCalendarOnce(t *testing.T) {
	future := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	sched := Schedule{Kind: ScheduleKindCalendar, Timezone: "UTC", RunAt: &future}
	base := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	got, ok := NextOccurrence(sched, base)
	if !ok || !got.Equal(future) {
		t.Fatalf("one-time future occurrence = %v ok=%v, want %v", got, ok, future)
	}
	past := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	sched.RunAt = &past
	if got, ok := NextOccurrence(sched, base); ok {
		t.Fatalf("one-time past run_at must yield no occurrence, got %v", got)
	}
}

func TestNextOccurrenceCalendarEveryNDays(t *testing.T) {
	runAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	sched := Schedule{
		Kind: ScheduleKindCalendar, Timezone: "UTC",
		RunAt: &runAt, RepeatEveryDays: 2,
	}
	base := runAt.Add(24 * time.Hour)
	got, ok := NextOccurrence(sched, base)
	if want := runAt.AddDate(0, 0, 2); !ok || !got.Equal(want) {
		t.Fatalf("every-2-days occurrence = %v ok=%v, want %v", got, ok, want)
	}
	// never-ending repeat keeps producing occurrences far ahead.
	got, ok = NextOccurrence(sched, runAt.AddDate(1, 0, 0))
	if !ok || !got.After(runAt.AddDate(1, 0, 0)) {
		t.Fatalf("never repeat stopped early: %v ok=%v", got, ok)
	}
}

func TestNextOccurrenceCalendarUntil(t *testing.T) {
	runAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC) // exactly the 7-day occurrence
	sched := Schedule{
		Kind: ScheduleKindCalendar, Timezone: "UTC",
		RunAt: &runAt, RepeatEveryDays: 7,
		RepeatEnd: &RepeatEnd{Kind: RepeatEndUntil, Until: &until},
	}
	got, ok := NextOccurrence(sched, runAt)
	if !ok || !got.Equal(until) {
		t.Fatalf("until-bound occurrence = %v ok=%v, want %v", got, ok, until)
	}
	if got, ok := NextOccurrence(sched, until); ok {
		t.Fatalf("occurrence beyond until = %v, want none", got)
	}
}

// NextOccurrence only computes time: a count repeat end does not bound it.
// The scheduler bounds execution via consumed_occurrences (contract [misfire、时区和 DST](docs/12_定时任务/定时执行与提醒/实现方案.md#misfire时区和-dst)).
func TestNextOccurrenceCalendarCountDoesNotBoundTime(t *testing.T) {
	runAt := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	sched := Schedule{
		Kind: ScheduleKindCalendar, Timezone: "UTC",
		RunAt: &runAt, RepeatEveryDays: 1,
		RepeatEnd: &RepeatEnd{Kind: RepeatEndCount, Count: 1},
	}
	got, ok := NextOccurrence(sched, runAt)
	if !ok || !got.Equal(runAt.AddDate(0, 0, 1)) {
		t.Fatalf("count end must not bound NextOccurrence: %v ok=%v", got, ok)
	}
}

func decideMisfire(t *testing.T, sched Schedule, last *time.Time, notBefore, now time.Time, grace time.Duration) MisfireDecision {
	t.Helper()
	decision, err := DecideMisfire(sched, last, notBefore, now, grace)
	if err != nil {
		t.Fatalf("DecideMisfire: %v", err)
	}
	return decision
}

func TestDecideMisfireNoDueOccurrence(t *testing.T) {
	last := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, now, 0)
	if decision.Action != MisfireNone {
		t.Fatalf("action = %v, want MisfireNone (occurrence %v)", decision.Action, decision.Occurrence)
	}
}

func TestDecideMisfireWithinGraceRunsOnce(t *testing.T) {
	last := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 11, 5, 0, 0, time.UTC)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, now, 10*time.Minute)
	if decision.Action != MisfireRunOnce {
		t.Fatalf("action = %v, want MisfireRunOnce", decision.Action)
	}
	if want := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC); !decision.Occurrence.Equal(want) {
		t.Fatalf("occurrence = %v, want %v", decision.Occurrence, want)
	}
	if decision.Complete {
		t.Fatal("cron schedule must never complete")
	}
}

// Multiple missed occurrences coalesce into exactly one misfire execution;
// they are never replayed one by one (contract [misfire、时区和 DST](docs/12_定时任务/定时执行与提醒/实现方案.md#misfire时区和-dst)).
func TestDecideMisfireCoalescesMultipleMisses(t *testing.T) {
	last := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC) // six 10-minute ticks missed
	decision := decideMisfire(t, cronSchedule("*/10 * * * *", "UTC"), &last, time.Time{}, now, 15*time.Minute)
	if decision.Action != MisfireRunOnce {
		t.Fatalf("action = %v, want MisfireRunOnce", decision.Action)
	}
	if want := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC); !decision.Occurrence.Equal(want) {
		t.Fatalf("coalesced occurrence = %v, want latest %v", decision.Occurrence, want)
	}
}

func TestDecideMisfireBeyondGraceSkips(t *testing.T) {
	last := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 11, 30, 0, 0, time.UTC)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, now, 10*time.Minute)
	if decision.Action != MisfireSkip {
		t.Fatalf("action = %v, want MisfireSkip", decision.Action)
	}
	if want := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC); !decision.Occurrence.Equal(want) {
		t.Fatalf("skipped occurrence = %v, want %v", decision.Occurrence, want)
	}
	if decision.Complete {
		t.Fatal("repeating schedule must not complete on skip")
	}
}

// A non-repeating calendar schedule skipped beyond grace is exhausted and
// the task becomes completed (contract [misfire、时区和 DST](docs/12_定时任务/定时执行与提醒/实现方案.md#misfire时区和-dst)).
func TestDecideMisfireOneTimeBeyondGraceSkipsAndCompletes(t *testing.T) {
	runAt := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	sched := Schedule{Kind: ScheduleKindCalendar, Timezone: "UTC", RunAt: &runAt}
	notBefore := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	decision := decideMisfire(t, sched, nil, notBefore, now, time.Hour)
	if decision.Action != MisfireSkip || !decision.Complete {
		t.Fatalf("decision = %+v, want MisfireSkip+Complete", decision)
	}
	if !decision.Occurrence.Equal(runAt) {
		t.Fatalf("skipped occurrence = %v, want %v", decision.Occurrence, runAt)
	}
}

func TestDecideMisfireOneTimeWithinGraceRuns(t *testing.T) {
	runAt := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	sched := Schedule{Kind: ScheduleKindCalendar, Timezone: "UTC", RunAt: &runAt}
	notBefore := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC)
	decision := decideMisfire(t, sched, nil, notBefore, now, time.Hour)
	if decision.Action != MisfireRunOnce || decision.Complete {
		t.Fatalf("decision = %+v, want MisfireRunOnce", decision)
	}
	if !decision.Occurrence.Equal(runAt) {
		t.Fatalf("occurrence = %v, want %v", decision.Occurrence, runAt)
	}
}

// Ticks at or before the scheduling base (creation time) are never missed.
func TestDecideMisfireRespectsNotBefore(t *testing.T) {
	notBefore := time.Date(2026, 7, 26, 10, 30, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 10, 45, 0, 0, time.UTC)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), nil, notBefore, now, time.Hour)
	if decision.Action != MisfireNone {
		t.Fatalf("ticks before creation must not misfire: %+v", decision)
	}
	now = time.Date(2026, 7, 26, 11, 5, 0, 0, time.UTC)
	decision = decideMisfire(t, cronSchedule("0 * * * *", "UTC"), nil, notBefore, now, time.Hour)
	if decision.Action != MisfireRunOnce {
		t.Fatalf("action = %v, want MisfireRunOnce", decision.Action)
	}
}

// The occurrence exactly equal to last_processed is already handled.
func TestDecideMisfireLastProcessedBoundary(t *testing.T) {
	last := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 26, 11, 30, 0, 0, time.UTC)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, now, time.Hour)
	if decision.Action != MisfireNone {
		t.Fatalf("processed occurrence must not re-fire: %+v", decision)
	}
}

func TestDecideMisfireAllWithFakeClock(t *testing.T) {
	// The whole misfire suite runs on fake-clock-derived fixed instants;
	// this test pins DecideMisfire to a FakeClock explicitly.
	clock := NewFakeClock(time.Date(2026, 7, 26, 11, 5, 0, 0, time.UTC))
	last := clock.Now().Add(-65 * time.Minute).Truncate(time.Hour)
	decision := decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, clock.Now(), 10*time.Minute)
	if decision.Action != MisfireRunOnce {
		t.Fatalf("fake-clock decision = %+v", decision)
	}
	// After advancing, the newest due tick is recent, so the decision still
	// coalesces; only when even the latest tick ages past grace does it skip.
	clock.Set(clock.Now().Add(50 * time.Minute)) // 11:55, latest tick 11:00
	decision = decideMisfire(t, cronSchedule("0 * * * *", "UTC"), &last, time.Time{}, clock.Now(), 10*time.Minute)
	if decision.Action != MisfireSkip {
		t.Fatalf("advanced fake-clock decision = %+v", decision)
	}
}

func TestCronSundayRangePreservesStartingPointAndStep(t *testing.T) {
	for _, tc := range []struct {
		field string
		days  []time.Weekday
	}{
		{"2-7/2", []time.Weekday{time.Tuesday, time.Thursday, time.Saturday}},
		{"1-7/2", []time.Weekday{time.Monday, time.Wednesday, time.Friday, time.Sunday}},
		{"5-7", []time.Weekday{time.Friday, time.Saturday, time.Sunday}},
		{"7-7", []time.Weekday{time.Sunday}},
		{"MON-7/2", []time.Weekday{time.Monday, time.Wednesday, time.Friday, time.Sunday}},
		{"0,2-7/2", []time.Weekday{time.Tuesday, time.Thursday, time.Saturday, time.Sunday}},
	} {
		t.Run(tc.field, func(t *testing.T) {
			start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
			cursor := start
			for _, want := range tc.days {
				got, ok := NextOccurrence(cronSchedule("0 9 * * "+tc.field, "UTC"), cursor)
				if !ok || got.Weekday() != want || !got.Before(start.AddDate(0, 0, 7)) {
					t.Fatalf("next=%v, want weekday %v in same week", got, want)
				}
				cursor = got
			}
			next, ok := NextOccurrence(cronSchedule("0 9 * * "+tc.field, "UTC"), cursor)
			if !ok || next.Before(start.AddDate(0, 0, 7)) {
				t.Fatalf("invented occurrence: %v", next)
			}
		})
	}
	for _, field := range []string{"2-7/0", "8-7/2", "2-7/999999999999999999999999999", "-1-7"} {
		if _, err := parseCron("0 9 * * " + field); err == nil {
			t.Errorf("invalid field accepted: %s", field)
		}
	}
}
