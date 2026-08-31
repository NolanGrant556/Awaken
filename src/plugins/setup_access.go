package plugins

import (
	"fmt"
	"strings"

	"awaken/src/plugins/ir"
)

// ResolveSetupRequirement performs the mutation-time compare-and-swap checks
// for a setup action. Callers must send both the installed package digest and
// the immutable requirement fingerprint so stale detail pages cannot authorize
// a replaced plugin or a changed account contract.
func (s *Store) ResolveSetupRequirement(name, setupID, expectedDigest, expectedFingerprint string) (*InstalledPlugin, ir.SetupRequirement, error) {
	plugin, err := s.Get(strings.TrimSpace(name))
	if err != nil {
		return nil, ir.SetupRequirement{}, err
	}
	if !plugin.IntegrityValid || strings.TrimSpace(expectedDigest) == "" || expectedDigest != plugin.Digest {
		return nil, ir.SetupRequirement{}, fmt.Errorf("%w: setup page digest is stale", ErrDigestChanged)
	}
	for _, requirement := range plugin.Setup {
		if requirement.ID != strings.TrimSpace(setupID) {
			continue
		}
		if strings.TrimSpace(expectedFingerprint) == "" || expectedFingerprint != requirement.Fingerprint {
			return nil, ir.SetupRequirement{}, fmt.Errorf("PLUGIN_SETUP_FINGERPRINT_CHANGED: setup requirement %s changed", requirement.ID)
		}
		return plugin, requirement, nil
	}
	return nil, ir.SetupRequirement{}, fmt.Errorf("%w: setup requirement %s", ErrNotFound, setupID)
}

// ResolveVariableValue is a Core-only credential resolver. The HTTP and
// Renderer layers must never expose its return value.
func (s *Store) ResolveVariableValue(name, key string) (string, error) {
	plugin, err := s.Get(strings.TrimSpace(name))
	if err != nil {
		return "", err
	}
	var spec *ir.VariableSpec
	for _, component := range plugin.Components {
		if component.Variable != nil && component.Variable.Key == key {
			copy := *component.Variable
			spec = &copy
			break
		}
	}
	if spec == nil {
		return "", fmt.Errorf("VARIABLE_UNKNOWN: unknown plugin variable %s", key)
	}
	var record Record
	if err := s.withLock(func() error {
		state, readErr := s.readStateUnlocked()
		if readErr != nil {
			return readErr
		}
		var ok bool
		record, ok = state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil
	}); err != nil {
		return "", err
	}
	stored, ok := record.Variables[key]
	if !ok {
		if value, ok := spec.Default.(string); ok {
			return value, nil
		}
		return "", fmt.Errorf("VARIABLE_REQUIRED: plugin variable %s is not configured", key)
	}
	if stored.SecretRef != "" {
		if s.secrets == nil {
			return "", fmt.Errorf("VARIABLE_SECRET_STORE_UNAVAILABLE: secret store is unavailable")
		}
		return s.secrets.Load(stored.SecretRef)
	}
	value, ok := stored.Value.(string)
	if !ok {
		return "", fmt.Errorf("VARIABLE_TYPE_INVALID: plugin variable %s is not a string", key)
	}
	return value, nil
}
