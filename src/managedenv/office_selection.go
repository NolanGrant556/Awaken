package managedenv

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

func (m *Manager) readOfficeSelections() (map[string]officeSelection, error) {
	path := filepath.Join(m.root, "office", "current.json")
	if err := ownedPath(m.root, path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var choices map[string]officeSelection
	if err := json.Unmarshal(data, &choices); err != nil {
		return nil, failure("environment_validation_failed", "办公环境记录无效："+err.Error())
	}
	if choices == nil {
		return nil, failure("environment_validation_failed", "办公环境记录无效")
	}
	for _, choice := range choices {
		if (choice.Current != "" && !safeID.MatchString(choice.Current)) || (choice.Previous != "" && !safeID.MatchString(choice.Previous)) {
			return nil, failure("environment_validation_failed", "办公环境记录无效")
		}
	}
	return choices, nil
}

// Called with both office-current and registry locks. Only completed, healthy,
// compatible manifests can rebuild a missing/corrupt selection. A readable
// selection is never replaced just because a newer directory exists.
func (m *Manager) recoverOfficeSelections(r *registry) (map[string]officeSelection, error) {
	choices, err := m.readOfficeSelections()
	if err == nil {
		return choices, nil
	}
	var invalid *Error
	if !os.IsNotExist(err) && !(errors.As(err, &invalid) && invalid.Code == "environment_validation_failed") {
		return nil, err
	}
	path := filepath.Join(m.root, "office", "current.json")
	if !os.IsNotExist(err) {
		if err := os.Rename(path, path+".invalid-"+newID()); err != nil {
			return nil, err
		}
	}
	choices = map[string]officeSelection{}
	required, err := m.officeRequirements()
	if err != nil {
		return choices, nil
	}
	candidates := []Manifest{}
	for _, f := range r.Instances {
		if officeCompatible(f, required) && m.reusable(f) {
			candidates = append(candidates, f)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].InstanceID < candidates[j].InstanceID
		}
		return candidates[i].CreatedAt.After(candidates[j].CreatedAt)
	})
	if len(candidates) == 0 {
		return choices, nil
	}
	choice := officeSelection{Current: candidates[0].InstanceID}
	if len(candidates) > 1 {
		choice.Previous = candidates[1].InstanceID
	}
	choice = m.newOfficeChoice(choice.Current, choice.Previous)
	choices[m.target] = choice
	return choices, m.persist(path, choices)
}

func (m *Manager) officeChoice() (officeSelection, error) {
	choices, err := m.readOfficeSelections()
	if err == nil {
		return m.activeOfficeChoice(choices[m.target]), nil
	}
	err = m.locked(m.ctx, "office-current", func() error {
		return m.registry(func(r *registry) error {
			var err error
			choices, err = m.recoverOfficeSelections(r)
			return err
		})
	})
	return m.activeOfficeChoice(choices[m.target]), err
}
