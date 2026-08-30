// Package variables owns Plugin variable value validation, interpolation, and
// secret persistence. Secret values never enter Plugin state JSON or DTOs.
package variables

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"awaken/src/plugins/ir"
	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keyringService = "awaken-plugin-variables"

var ErrSecretNotFound = errors.New("plugin variable secret not found")

type SecretStore interface {
	Save(pluginName, key, currentRef, value string) (string, error)
	Load(secretRef string) (string, error)
	Delete(secretRef string) error
}

type PersistentSecretStore struct{}

func NewPersistentSecretStore() *PersistentSecretStore { return &PersistentSecretStore{} }

func (*PersistentSecretStore) Save(pluginName, key, currentRef, value string) (string, error) {
	ref := strings.TrimSpace(currentRef)
	if ref == "" {
		ref = pluginName + "/" + key + "/" + uuid.NewString()
	}
	if err := keyring.Set(keyringService, ref, value); err != nil {
		return "", err
	}
	return ref, nil
}

func (*PersistentSecretStore) Load(secretRef string) (string, error) {
	value, err := keyring.Get(keyringService, secretRef)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrSecretNotFound
	}
	return value, err
}

func (*PersistentSecretStore) Delete(secretRef string) error {
	if strings.TrimSpace(secretRef) == "" {
		return nil
	}
	if err := keyring.Delete(keyringService, secretRef); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type MemorySecretStore struct {
	Values map[string]string
}

func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{Values: map[string]string{}}
}

func (s *MemorySecretStore) Save(pluginName, key, currentRef, value string) (string, error) {
	ref := currentRef
	if ref == "" {
		ref = pluginName + "/" + key + "/" + uuid.NewString()
	}
	s.Values[ref] = value
	return ref, nil
}

func (s *MemorySecretStore) Load(ref string) (string, error) {
	value, ok := s.Values[ref]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *MemorySecretStore) Delete(ref string) error {
	delete(s.Values, ref)
	return nil
}

func ValidateValue(spec ir.VariableSpec, value any) (any, error) {
	if value == nil {
		if spec.Required && spec.Default == nil {
			return nil, valueError("VARIABLE_REQUIRED", "%s is required", spec.Key)
		}
		return spec.Default, nil
	}
	switch spec.Type {
	case "string", "directory", "file":
		if spec.Multiple {
			list, ok := value.([]any)
			if !ok {
				return nil, valueError("VARIABLE_TYPE_INVALID", "%s must be an array of strings", spec.Key)
			}
			out := make([]string, 0, len(list))
			for _, item := range list {
				text, ok := item.(string)
				if !ok {
					return nil, valueError("VARIABLE_TYPE_INVALID", "%s must be an array of strings", spec.Key)
				}
				out = append(out, text)
			}
			return out, validateEnum(spec, out)
		}
		text, ok := value.(string)
		if !ok {
			return nil, valueError("VARIABLE_TYPE_INVALID", "%s must be a string", spec.Key)
		}
		return text, validateEnum(spec, text)
	case "number", "integer":
		number, ok := value.(float64)
		if !ok {
			if text, textOK := value.(string); textOK {
				var err error
				number, err = strconv.ParseFloat(text, 64)
				ok = err == nil
			}
		}
		if !ok {
			return nil, valueError("VARIABLE_TYPE_INVALID", "%s must be a number", spec.Key)
		}
		if spec.Minimum != nil && number < *spec.Minimum || spec.Maximum != nil && number > *spec.Maximum {
			return nil, valueError("VARIABLE_RANGE_INVALID", "%s is outside the allowed range", spec.Key)
		}
		return number, validateEnum(spec, number)
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, valueError("VARIABLE_TYPE_INVALID", "%s must be a boolean", spec.Key)
		}
		return boolean, validateEnum(spec, boolean)
	default:
		return nil, valueError("VARIABLE_TYPE_UNSUPPORTED", "%s uses unsupported variable type %q", spec.Key, spec.Type)
	}
}

func validateEnum(spec ir.VariableSpec, value any) error {
	if spec.Const != nil && fmt.Sprint(spec.Const) != fmt.Sprint(value) {
		return valueError("VARIABLE_CONST_MISMATCH", "%s must equal its const value", spec.Key)
	}
	if len(spec.Enum) == 0 {
		return nil
	}
	for _, allowed := range spec.Enum {
		if fmt.Sprint(allowed) == fmt.Sprint(value) {
			return nil
		}
	}
	return valueError("VARIABLE_VALUE_NOT_ALLOWED", "%s is not an allowed value", spec.Key)
}

func valueError(code, format string, args ...any) error {
	return fmt.Errorf(code+": "+format, args...)
}

type InterpolationContext struct {
	PluginRoot string
	PluginData string
	ProjectDir string
	Values     map[string]string
}

func Expand(value string, dialect ir.Dialect, field string, context InterpolationContext) (string, error) {
	allowed := allowedField(dialect, field)
	builtinReplacements := map[string]string{
		"${PLUGIN_ROOT}": context.PluginRoot, "${PLUGIN_DATA}": context.PluginData,
		"${CLAUDE_PLUGIN_ROOT}": context.PluginRoot, "${CLAUDE_PLUGIN_DATA}": context.PluginData,
		"${CURSOR_PLUGIN_ROOT}": context.PluginRoot, "${CURSOR_PLUGIN_DATA}": context.PluginData,
		"${CLAUDE_PROJECT_DIR}": context.ProjectDir, "${CURSOR_PROJECT_DIR}": context.ProjectDir,
	}
	result := value
	for placeholder, replacement := range builtinReplacements {
		result = strings.ReplaceAll(result, placeholder, replacement)
	}
	// Built-in paths are host-controlled and remain valid in command argv.
	// Only user-configurable variables are forbidden in execution positions.
	if !allowed && containsVariableReference(result, dialect) {
		return "", fmt.Errorf("VARIABLE_FORBIDDEN_POSITION: variables are not allowed in %s", field)
	}
	replacements := map[string]string{}
	for key, item := range context.Values {
		replacements["${"+key+"}"] = item
		replacements["${user_config."+key+"}"] = item
	}
	for placeholder, replacement := range replacements {
		result = strings.ReplaceAll(result, placeholder, replacement)
	}
	if (dialect == ir.DialectClaudeCode || dialect == ir.DialectCursor) && containsVariableReference(result, dialect) {
		return "", fmt.Errorf("VARIABLE_MISSING: %s contains an unresolved plugin variable", field)
	}
	return result, nil
}

func allowedField(dialect ir.Dialect, field string) bool {
	switch dialect {
	case ir.DialectClaudeCode:
		switch field {
		case "mcp.args", "mcp.env", "mcp.cwd", "mcp.url", "hook.env", "skill.content", "agent.content":
			return true
		}
	case ir.DialectCursor:
		switch field {
		case "mcp.args", "mcp.env", "mcp.cwd", "mcp.url", "mcp.headers", "hook.prompt":
			return true
		}
	}
	return field == "mcp.args" || field == "mcp.env" || field == "mcp.cwd"
}

func containsVariableReference(value string, dialect ir.Dialect) bool {
	if strings.Contains(value, "${PLUGIN_ROOT}") || strings.Contains(value, "${PLUGIN_DATA}") {
		return false
	}
	if dialect == ir.DialectClaudeCode {
		return strings.Contains(value, "${user_config.") || strings.Contains(value, "${CLAUDE_")
	}
	return strings.Contains(value, "${")
}
