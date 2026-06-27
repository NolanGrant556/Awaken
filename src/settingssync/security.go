package settingssync

import "fmt"

type ManagedSettingsSecurityPolicy struct {
	DangerousKeys map[string]string
}

type SecurityCheckResult struct {
	RequiresConfirmation bool
	Dangerous            []DangerousChange
	Safe                 []EntryChange
}

type DangerousChange struct {
	Key       string
	OldValue  string
	NewValue  string
	Source    Source
	Scope     Scope
	Reason    string
	Sensitive bool
}

func (c DangerousChange) String() string {
	sensitive := c.Sensitive || IsSensitiveKey(c.Key) || LooksSensitiveValue(c.OldValue) || LooksSensitiveValue(c.NewValue)
	return fmt.Sprintf(
		"settingssync.DangerousChange{key:%q source:%q scope:%q reason:%q old:%q new:%q}",
		c.Key,
		c.Source,
		c.Scope,
		c.Reason,
		RedactValue(c.Key, c.OldValue, sensitive),
		RedactValue(c.Key, c.NewValue, sensitive),
	)
}

func DefaultManagedSettingsSecurityPolicy() ManagedSettingsSecurityPolicy {
	reasons := map[string]string{
		"allowedcommands":                "changes allowed shell commands",
		"allowedtools":                   "changes allowed tool execution",
		"approvalpolicy":                 "changes approval policy",
		"bypasspermissions":              "bypasses permission checks",
		"dangerouslyskippermissions":     "bypasses permission checks",
		"deniedcommands":                 "changes denied shell commands",
		"deniedtools":                    "changes denied tool execution",
		"disabledmcpjsonservers":         "changes MCP server trust",
		"enableallprojectmcpservers":     "enables project MCP servers",
		"enabledmcpjsonservers":          "changes MCP server trust",
		"hooks":                          "changes lifecycle hooks",
		"mcpservers":                     "changes MCP server configuration",
		"permissions":                    "changes permission policy",
		"shell":                          "changes shell execution behavior",
		"terminal":                       "changes terminal execution behavior",
		"toolpermissions":                "changes tool permission policy",
		"workspacefilesystempermissions": "changes workspace filesystem access",
		"workspacefs":                    "changes workspace filesystem access",
		"workspacefspermissions":         "changes workspace filesystem access",
	}
	return ManagedSettingsSecurityPolicy{DangerousKeys: reasons}
}

func CheckManagedSettingChanges(changes []EntryChange, policy ManagedSettingsSecurityPolicy) SecurityCheckResult {
	if len(policy.DangerousKeys) == 0 {
		policy = DefaultManagedSettingsSecurityPolicy()
	}

	result := SecurityCheckResult{}
	for _, change := range changes {
		if change.OldValue == change.NewValue {
			result.Safe = append(result.Safe, change)
			continue
		}
		if change.Source != SourceRemoteManaged {
			result.Safe = append(result.Safe, change)
			continue
		}
		if dangerous, ok := policy.dangerousChange(change); ok {
			result.Dangerous = append(result.Dangerous, dangerous)
			result.RequiresConfirmation = true
			continue
		}
		result.Safe = append(result.Safe, change)
	}
	return result
}

func (p ManagedSettingsSecurityPolicy) dangerousChange(change EntryChange) (DangerousChange, bool) {
	if len(p.DangerousKeys) == 0 {
		p = DefaultManagedSettingsSecurityPolicy()
	}
	normalized := normalizeKey(change.Key)
	if reason, ok := p.DangerousKeys[normalized]; ok {
		return DangerousChange{
			Key:       change.Key,
			OldValue:  change.OldValue,
			NewValue:  change.NewValue,
			Source:    change.Source,
			Scope:     change.Scope,
			Reason:    reason,
			Sensitive: change.Sensitive,
		}, true
	}
	for key, reason := range p.DangerousKeys {
		if key != "" && containsNormalized(normalized, key) {
			return DangerousChange{
				Key:       change.Key,
				OldValue:  change.OldValue,
				NewValue:  change.NewValue,
				Source:    change.Source,
				Scope:     change.Scope,
				Reason:    reason,
				Sensitive: change.Sensitive,
			}, true
		}
	}
	return DangerousChange{}, false
}

func containsNormalized(value, fragment string) bool {
	return len(fragment) > 0 && len(value) >= len(fragment) && (value == fragment || contains(value, fragment))
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
