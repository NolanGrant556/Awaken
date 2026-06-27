package featureflags

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const internalUserType = "ant"

var ErrInvalidEnvOverrides = errors.New("invalid feature flag env overrides")

type Source string

const (
	SourceNone           Source = ""
	SourceEnvOverride    Source = "env_override"
	SourceConfigOverride Source = "config_override"
	SourceRemote         Source = "remote"
	SourceDefault        Source = "default"
)

type Inputs struct {
	UserType         string
	EnvOverridesJSON string
	ConfigOverrides  map[string]any
	RemoteValues     map[string]any
	Defaults         map[string]any
}

type Evaluator struct {
	envOverrides    map[string]any
	configOverrides map[string]any
	remoteValues    map[string]any
	defaults        map[string]any
}

type ResolvedValue struct {
	Value  any
	Source Source
	Found  bool
}

type Override struct {
	Feature string
	Value   any
	Source  Source
}

func NewEvaluator(inputs Inputs) (Evaluator, error) {
	envOverrides, err := ParseEnvOverrides(inputs.UserType, inputs.EnvOverridesJSON)
	if err != nil {
		return Evaluator{}, err
	}

	return Evaluator{
		envOverrides:    copyValues(envOverrides),
		configOverrides: copyValues(inputs.ConfigOverrides),
		remoteValues:    copyValues(inputs.RemoteValues),
		defaults:        copyValues(inputs.Defaults),
	}, nil
}

func ParseEnvOverrides(userType, raw string) (map[string]any, error) {
	if userType != internalUserType || raw == "" {
		return nil, nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEnvOverrides, err)
	}

	overrides, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected JSON object", ErrInvalidEnvOverrides)
	}

	return overrides, nil
}

func (e Evaluator) HasOverride(feature string) bool {
	if _, ok := e.envOverrides[feature]; ok {
		return true
	}
	_, ok := e.configOverrides[feature]
	return ok
}

func (e Evaluator) Resolve(feature string) ResolvedValue {
	if value, ok := e.envOverrides[feature]; ok {
		return ResolvedValue{Value: value, Source: SourceEnvOverride, Found: true}
	}
	if value, ok := e.configOverrides[feature]; ok {
		return ResolvedValue{Value: value, Source: SourceConfigOverride, Found: true}
	}
	if value, ok := e.remoteValues[feature]; ok {
		return ResolvedValue{Value: value, Source: SourceRemote, Found: true}
	}
	if value, ok := e.defaults[feature]; ok {
		return ResolvedValue{Value: value, Source: SourceDefault, Found: true}
	}
	return ResolvedValue{Source: SourceNone}
}

func (e Evaluator) ListOverrides() []Override {
	byFeature := make(map[string]Override, len(e.envOverrides)+len(e.configOverrides))
	for feature, value := range e.configOverrides {
		byFeature[feature] = Override{Feature: feature, Value: value, Source: SourceConfigOverride}
	}
	for feature, value := range e.envOverrides {
		byFeature[feature] = Override{Feature: feature, Value: value, Source: SourceEnvOverride}
	}

	features := make([]string, 0, len(byFeature))
	for feature := range byFeature {
		features = append(features, feature)
	}
	sort.Strings(features)

	overrides := make([]Override, 0, len(features))
	for _, feature := range features {
		overrides = append(overrides, byFeature[feature])
	}
	return overrides
}

func SetConfigOverride(current map[string]any, feature string, value any) map[string]any {
	next := copyValues(current)
	if next == nil {
		next = map[string]any{}
	}
	next[feature] = value
	return next
}

func ClearConfigOverride(current map[string]any, feature string) map[string]any {
	next := copyValues(current)
	delete(next, feature)
	return next
}

func ClearConfigOverrides() map[string]any {
	return map[string]any{}
}

func copyValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
