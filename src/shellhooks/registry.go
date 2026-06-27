package shellhooks

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu    sync.RWMutex
	specs map[HookKey]Spec
}

type PlanError struct {
	Spec  Spec
	Error string
}

type RegistrationPlan struct {
	New        []Spec
	Duplicates []Spec
	Invalid    []PlanError
}

func NewRegistry(specs ...Spec) (*Registry, error) {
	r := &Registry{specs: make(map[HookKey]Spec)}
	for _, spec := range specs {
		if _, err := r.Register(spec); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(spec Spec) (bool, error) {
	if err := ValidateSpec(spec); err != nil {
		return false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.specs == nil {
		r.specs = make(map[HookKey]Spec)
	}
	key := KeyForSpec(spec)
	if _, exists := r.specs[key]; exists {
		return false, nil
	}
	r.specs[key] = spec
	return true, nil
}

func (r *Registry) Specs() []Spec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]Spec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	return specs
}

func (r *Registry) Plan(specs []Spec) RegistrationPlan {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[HookKey]struct{}, len(r.specs)+len(specs))
	for key := range r.specs {
		seen[key] = struct{}{}
	}

	var plan RegistrationPlan
	for _, spec := range specs {
		if err := ValidateSpec(spec); err != nil {
			plan.Invalid = append(plan.Invalid, PlanError{Spec: spec, Error: err.Error()})
			continue
		}
		key := KeyForSpec(spec)
		if _, ok := seen[key]; ok {
			plan.Duplicates = append(plan.Duplicates, spec)
			continue
		}
		seen[key] = struct{}{}
		plan.New = append(plan.New, spec)
	}
	return plan
}

func ValidateSpec(spec Spec) error {
	if !IsValidEvent(spec.Event) {
		return fmt.Errorf("unknown hook event %q", spec.Event)
	}
	if len(spec.Argv) == 0 && spec.Command == "" {
		return fmt.Errorf("hook command is empty")
	}
	if len(spec.Argv) > 0 && spec.Argv[0] == "" {
		return fmt.Errorf("hook argv[0] is empty")
	}
	return nil
}
