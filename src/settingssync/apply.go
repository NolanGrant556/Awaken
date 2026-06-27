package settingssync

import (
	"fmt"
	"sort"
)

type ApplyAction string

const (
	ApplyActionWrite                     ApplyAction = "write"
	ApplyActionSkipUnchanged             ApplyAction = "skip_unchanged"
	ApplyActionSkipTooLarge              ApplyAction = "skip_too_large"
	ApplyActionBlockRequiresConfirmation ApplyAction = "block_requires_confirmation"
)

type RemoteApplyOptions struct {
	MaxEntryBytes         int
	SecurityPolicy        ManagedSettingsSecurityPolicy
	ConfirmedDangerousKey map[string]bool
}

type ApplyDecision struct {
	Key                  string
	Action               ApplyAction
	Entry                Entry
	CurrentValue         string
	CurrentExists        bool
	Reason               string
	SizeBytes            int
	MaxBytes             int
	RequiresConfirmation bool
	DangerousChange      DangerousChange
}

func (d ApplyDecision) Change() EntryChange {
	return EntryChange{
		Key:       d.Key,
		OldValue:  d.CurrentValue,
		NewValue:  d.Entry.Value,
		Source:    d.Entry.Source,
		Scope:     d.Entry.Scope,
		Sensitive: d.Entry.Sensitive || IsSensitiveKey(d.Key),
	}
}

type RemoteApplyPlan struct {
	Decisions []ApplyDecision
}

func PlanRemoteApply(remote []Entry, local LocalSnapshot, opts RemoteApplyOptions) RemoteApplyPlan {
	maxBytes := opts.MaxEntryBytes
	if maxBytes <= 0 {
		maxBytes = local.MaxEntryBytes
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxEntryBytes
	}

	remoteEntries := append([]Entry(nil), remote...)
	sort.Slice(remoteEntries, func(i, j int) bool {
		return remoteEntries[i].Key < remoteEntries[j].Key
	})

	plan := RemoteApplyPlan{}
	for _, entry := range remoteEntries {
		decision := ApplyDecision{
			Key:       entry.Key,
			Entry:     entry,
			SizeBytes: entry.SizeBytes(),
			MaxBytes:  maxBytes,
		}
		if current, ok := local.Entry(entry.Key); ok {
			decision.CurrentValue = current.Value
			decision.CurrentExists = true
			if current.Value == entry.Value {
				decision.Action = ApplyActionSkipUnchanged
				decision.Reason = "unchanged"
				plan.Decisions = append(plan.Decisions, decision)
				continue
			}
		}

		if decision.SizeBytes > maxBytes {
			decision.Action = ApplyActionSkipTooLarge
			decision.Reason = "entry exceeds size limit"
			plan.Decisions = append(plan.Decisions, decision)
			continue
		}

		check := CheckManagedSettingChanges([]EntryChange{decision.Change()}, opts.SecurityPolicy)
		if len(check.Dangerous) > 0 && !opts.ConfirmedDangerousKey[entry.Key] {
			decision.Action = ApplyActionBlockRequiresConfirmation
			decision.Reason = "dangerous managed setting requires confirmation"
			decision.RequiresConfirmation = true
			decision.DangerousChange = check.Dangerous[0]
			plan.Decisions = append(plan.Decisions, decision)
			continue
		}

		decision.Action = ApplyActionWrite
		decision.Reason = "changed"
		plan.Decisions = append(plan.Decisions, decision)
	}
	return plan
}

func (p RemoteApplyPlan) EntriesToApply() []Entry {
	entries := make([]Entry, 0, len(p.Decisions))
	for _, decision := range p.Decisions {
		if decision.Action == ApplyActionWrite {
			entries = append(entries, decision.Entry)
		}
	}
	return entries
}

func (p RemoteApplyPlan) BlockedDangerousChanges() []DangerousChange {
	changes := make([]DangerousChange, 0)
	for _, decision := range p.Decisions {
		if decision.Action == ApplyActionBlockRequiresConfirmation {
			changes = append(changes, decision.DangerousChange)
		}
	}
	return changes
}

func (p RemoteApplyPlan) Summary() string {
	counts := map[ApplyAction]int{}
	for _, decision := range p.Decisions {
		counts[decision.Action]++
	}
	return fmt.Sprintf(
		"settingssync.RemoteApplyPlan{write:%d unchanged:%d tooLarge:%d requiresConfirmation:%d}",
		counts[ApplyActionWrite],
		counts[ApplyActionSkipUnchanged],
		counts[ApplyActionSkipTooLarge],
		counts[ApplyActionBlockRequiresConfirmation],
	)
}

type ApplyError struct {
	Key string
	Err error
}

func (e ApplyError) Error() string {
	if e.Err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %v", e.Key, e.Err)
}

type RemoteApplyResult struct {
	Applied []Entry
	Skipped []ApplyDecision
	Blocked []DangerousChange
	Errors  []ApplyError
}

func (r RemoteApplyResult) Success() bool {
	return len(r.Errors) == 0 && len(r.Blocked) == 0
}

func (r RemoteApplyResult) Summary() string {
	return fmt.Sprintf("settingssync.RemoteApplyResult{applied:%d skipped:%d blocked:%d errors:%d}", len(r.Applied), len(r.Skipped), len(r.Blocked), len(r.Errors))
}

func ApplyRemotePlan(plan RemoteApplyPlan, apply func(Entry) error) (result RemoteApplyResult) {
	if apply == nil {
		apply = func(Entry) error { return nil }
	}

	for _, decision := range plan.Decisions {
		switch decision.Action {
		case ApplyActionWrite:
			err := safeApplyEntry(decision.Entry, apply)
			if err != nil {
				result.Errors = append(result.Errors, ApplyError{Key: decision.Key, Err: err})
				continue
			}
			result.Applied = append(result.Applied, decision.Entry)
		case ApplyActionBlockRequiresConfirmation:
			result.Blocked = append(result.Blocked, decision.DangerousChange)
			result.Skipped = append(result.Skipped, decision)
		default:
			result.Skipped = append(result.Skipped, decision)
		}
	}
	return result
}

func safeApplyEntry(entry Entry, apply func(Entry) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("apply panic: %v", recovered)
		}
	}()
	return apply(entry)
}
