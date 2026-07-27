package scheduledtask

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Clock abstracts time so scheduler and repository tests never wait on the
// wall clock.
type Clock interface {
	Now() time.Time
}

// RealClock is the production clock.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a manually advanced clock for tests in this package and in
// later scheduler slices.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock pinned at now.
func NewFakeClock(now time.Time) *FakeClock {
	return &FakeClock{now: now}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set pins the clock at t.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// cronParser accepts exactly the five standard fields (minute, hour,
// day-of-month, month, day-of-week) with English names for month/DOW. The
// Descriptor flag stays off on purpose: @every/@daily shorthands are not
// part of the contract (§6.3).
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// dowRangeEndingAtSeven matches range tokens such as "5-7" or "1-7/2".
var dowRangeEndingAtSeven = regexp.MustCompile(`^(.+)-7(/[0-9]+)?$`)

// normalizeDOWField maps the numeric Sunday 7 onto 0, which is what the
// underlying parser accepts. A range ending at 7 ("5-7") wraps onto the next
// week, so it becomes "<low>-6,0".
func normalizeDOWField(field string) string {
	parts := strings.Split(field, ",")
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		switch {
		case part == "7":
			out = append(out, "0")
		case dowRangeEndingAtSeven.MatchString(part):
			match := dowRangeEndingAtSeven.FindStringSubmatch(part)
			step := match[2]
			out = append(out, match[1]+"-6"+step, "0")
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}

// normalizeCronExpression returns the expression with the DOW Sunday
// normalization applied. The result still has exactly five fields.
func normalizeCronExpression(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return expr
	}
	fields[4] = normalizeDOWField(fields[4])
	return strings.Join(fields, " ")
}

// cronFieldCount validates the field count; the parser itself would accept
// nothing else once Descriptor is off, but the explicit check gives a
// field-level error and rejects empty expressions.
func cronFieldCount(expr string) int {
	return len(strings.Fields(expr))
}

// parseCron parses a five-field expression after DOW normalization.
func parseCron(expr string) (cron.Schedule, error) {
	if cronFieldCount(expr) != 5 {
		return nil, fmt.Errorf("cron expression must have exactly 5 fields, got %d", cronFieldCount(expr))
	}
	schedule, err := cronParser.Parse(normalizeCronExpression(expr))
	if err != nil {
		return nil, err
	}
	return schedule, nil
}

// cronSearchWindow caps how far NextOccurrence looks ahead. It matches the
// underlying library's own five-year activation bound.
const cronSearchWindow = 5 * 365 * 24 * time.Hour

// sameLocalWallTime compares the civil time (date, hour, minute) of two
// instants inside the same location.
func sameLocalWallTime(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd &&
		a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

// NextOccurrence returns the first occurrence strictly after `after`, or
// false when the schedule has none within the search window.
//
// Cron occurrences are computed in the task timezone and returned as
// absolute instants. Calendar occurrence k (k from 0) is
// run_at.AddDate(0, 0, k*repeat_every_days) evaluated in the task timezone,
// so the local wall-clock time survives DST transitions (the time package
// normalizes the offset). A `count` repeat end does not bound this function:
// count semantics combine with consumed_occurrences in the scheduler.
func NextOccurrence(sched Schedule, after time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil {
		return time.Time{}, false
	}
	switch sched.Kind {
	case ScheduleKindCron:
		parsed, err := parseCron(sched.Cron)
		if err != nil {
			return time.Time{}, false
		}
		next := parsed.Next(after.In(loc))
		// Fall-back repeats a local wall-clock time twice; the contract
		// executes each expression occurrence only once (§8.3), so skip the
		// second instant of an already-seen local time.
		for !next.IsZero() && sameLocalWallTime(next, after.In(loc)) {
			next = parsed.Next(next)
		}
		if next.IsZero() || next.After(after.Add(cronSearchWindow)) {
			return time.Time{}, false
		}
		return next, true
	case ScheduleKindCalendar:
		if sched.RunAt == nil {
			return time.Time{}, false
		}
		base := sched.RunAt.In(loc)
		if sched.RepeatEveryDays <= 0 {
			if base.After(after) {
				return base, true
			}
			return time.Time{}, false
		}
		for k := 0; ; k++ {
			occurrence := base.AddDate(0, 0, k*sched.RepeatEveryDays)
			if sched.RepeatEnd != nil && sched.RepeatEnd.Kind == RepeatEndUntil &&
				sched.RepeatEnd.Until != nil && occurrence.After(*sched.RepeatEnd.Until) {
				return time.Time{}, false
			}
			if occurrence.After(after) {
				return occurrence, true
			}
		}
	default:
		return time.Time{}, false
	}
}

// MisfireAction is the startup/recovery decision for due occurrences.
type MisfireAction int

const (
	// MisfireNone means no unprocessed occurrence is due; wait for the next
	// scheduled occurrence.
	MisfireNone MisfireAction = iota
	// MisfireRunOnce means the newest missed occurrence is inside the grace
	// window: coalesce all misses into a single trigger=misfire execution.
	MisfireRunOnce
	// MisfireSkip means the newest missed occurrence is outside the grace
	// window: record one skipped execution and resume from the next
	// occurrence. Missed occurrences are never replayed one by one.
	MisfireSkip
)

// MisfireDecision is the outcome of DecideMisfire.
type MisfireDecision struct {
	Action MisfireAction
	// Occurrence is the due occurrence to execute (MisfireRunOnce) or to
	// record as skipped (MisfireSkip). Zero for MisfireNone.
	Occurrence time.Time
	// Complete reports that the schedule is exhausted by this decision: a
	// non-repeating calendar schedule whose single occurrence is skipped is
	// done and the task transitions to completed.
	Complete bool
}

// maxMisfireScanSteps bounds the occurrence scan so a pathological schedule
// cannot loop DecideMisfire forever.
const maxMisfireScanSteps = 1000000

// DecideMisfire applies the contract §8.3 misfire rules.
//
// lastProcessed is the newest occurrence already processed (nil when none);
// notBefore lower-bounds the scan (normally the task creation time, so a
// cron task never "misses" ticks that predate it). now and grace come from
// the injectable clock and misfire_grace_seconds. Occurrences at or before
// both bounds are considered already handled.
func DecideMisfire(sched Schedule, lastProcessed *time.Time, notBefore, now time.Time, grace time.Duration) (MisfireDecision, error) {
	lower := notBefore
	if lastProcessed != nil && lastProcessed.After(lower) {
		lower = *lastProcessed
	}
	if lower.IsZero() {
		// Defensive default; callers pass the task creation time.
		lower = now.Add(-30 * 24 * time.Hour)
	}
	// Coarse fast-forward: occurrences at or before the cursor can never be
	// the latest due one, so skip ahead in 40-day strides to bound the fine
	// scan even when the lower bound lies far in the past.
	cursor := lower
	for {
		probe := cursor.Add(40 * 24 * time.Hour)
		occurrence, ok := NextOccurrence(sched, probe)
		if !ok || occurrence.After(now) {
			break
		}
		cursor = probe
	}
	var latest time.Time
	found := false
	for steps := 0; steps < maxMisfireScanSteps; steps++ {
		occurrence, ok := NextOccurrence(sched, cursor)
		if !ok || occurrence.After(now) {
			break
		}
		latest = occurrence
		found = true
		cursor = occurrence
	}
	if !found {
		return MisfireDecision{Action: MisfireNone}, nil
	}
	if now.Sub(latest) <= grace {
		return MisfireDecision{Action: MisfireRunOnce, Occurrence: latest}, nil
	}
	decision := MisfireDecision{Action: MisfireSkip, Occurrence: latest}
	if sched.Kind == ScheduleKindCalendar && sched.RepeatEveryDays <= 0 {
		decision.Complete = true
	}
	return decision, nil
}
