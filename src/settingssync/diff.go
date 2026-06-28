package settingssync

import "fmt"

type UploadPlan struct {
	Changed   []Entry
	Unchanged []Entry
	Skipped   []SnapshotSkip
}

func PlanUploadDiff(local LocalSnapshot, remote map[string]string) UploadPlan {
	plan := UploadPlan{
		Skipped: append([]SnapshotSkip(nil), local.Skipped...),
	}

	for _, key := range sortedEntryKeys(local.Entries) {
		entry := local.Entries[key]
		if remoteValue, ok := remote[key]; ok && remoteValue == entry.Value {
			plan.Unchanged = append(plan.Unchanged, entry)
			continue
		}
		plan.Changed = append(plan.Changed, entry)
	}

	return plan
}

func (p UploadPlan) HasChanges() bool {
	return len(p.Changed) > 0
}

func (p UploadPlan) ChangedEntryValues() map[string]string {
	values := make(map[string]string, len(p.Changed))
	for _, entry := range p.Changed {
		values[entry.Key] = entry.Value
	}
	return values
}

func (p UploadPlan) Summary() string {
	return fmt.Sprintf("settingssync.UploadPlan{changed:%d unchanged:%d skipped:%d}", len(p.Changed), len(p.Unchanged), len(p.Skipped))
}
