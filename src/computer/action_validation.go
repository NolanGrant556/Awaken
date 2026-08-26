package computer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// ValidateActionSpec enforces the parameters required by each action before an
// action is prepared, recorded, or handed to a driver. Schema validation alone
// cannot express these action-dependent requirements without exposing a large
// provider-sensitive oneOf schema.
func ValidateActionSpec(spec ActionSpec) error {
	parameters := CloneActionParameters(spec.Parameters)
	if value, ok := parameters["delivery_mode"]; ok {
		mode, valid := value.(string)
		if !valid || (mode != "background" && mode != "foreground") {
			return fmt.Errorf("parameters.delivery_mode must be background or foreground for %s", spec.Kind)
		}
	}
	if point := spec.Target.PixelFallback; point != nil {
		if _, ok := parameters["x"]; !ok {
			parameters["x"] = point.X
		}
		if _, ok := parameters["y"]; !ok {
			parameters["y"] = point.Y
		}
	}

	switch spec.Kind {
	case ActionClick, ActionDoubleClick:
		if strings.TrimSpace(spec.Target.Locator) != "" {
			return nil
		}
		if err := requireScreenCoordinate(parameters, "x", spec.Kind); err != nil {
			return err
		}
		return requireScreenCoordinate(parameters, "y", spec.Kind)
	case ActionScroll, ActionMoveCursor:
		if err := requireScreenCoordinate(parameters, "x", spec.Kind); err != nil {
			return err
		}
		return requireScreenCoordinate(parameters, "y", spec.Kind)
	case ActionDrag:
		for _, name := range []string{"from_x", "from_y", "to_x", "to_y"} {
			if err := requireScreenCoordinate(parameters, name, spec.Kind); err != nil {
				return err
			}
		}
		return nil
	case ActionKey:
		return requireNonEmptyString(parameters, "key", spec.Kind)
	case ActionHotkey:
		return requireNonEmptyStringList(parameters, "keys", spec.Kind)
	case ActionTypeText:
		return requireNonEmptyString(parameters, "text", spec.Kind)
	case ActionWait:
		if value, ok := parameters["ms"]; ok {
			return validateNonNegativeInteger(value, "parameters.ms", spec.Kind)
		}
		return nil
	case "":
		return fmt.Errorf("action is required")
	default:
		return fmt.Errorf("unsupported action %q", spec.Kind)
	}
}

func requireScreenCoordinate(parameters map[string]any, name string, action ActionKind) error {
	value, ok := parameters[name]
	if !ok {
		return fmt.Errorf("parameters.%s is required for %s; pointer actions require a complete screen-point coordinate set", name, action)
	}
	return validateNonNegativeInteger(value, "parameters."+name, action)
}

func validateNonNegativeInteger(value any, field string, action ActionKind) error {
	number, ok := numericValue(value)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
		return fmt.Errorf("%s must be an integer for %s", field, action)
	}
	if number < 0 {
		return fmt.Errorf("%s must be non-negative for %s", field, action)
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case float32:
		return float64(number), true
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func requireNonEmptyString(parameters map[string]any, name string, action ActionKind) error {
	value, ok := parameters[name]
	if !ok {
		return fmt.Errorf("parameters.%s is required for %s", name, action)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("parameters.%s must be a non-empty string for %s", name, action)
	}
	return nil
}

func requireNonEmptyStringList(parameters map[string]any, name string, action ActionKind) error {
	value, ok := parameters[name]
	if !ok {
		return fmt.Errorf("parameters.%s is required for %s", name, action)
	}
	var values []string
	switch raw := value.(type) {
	case []string:
		values = raw
	case []any:
		values = make([]string, 0, len(raw))
		for _, item := range raw {
			text, ok := item.(string)
			if !ok {
				return fmt.Errorf("parameters.%s must be a non-empty array of strings for %s", name, action)
			}
			values = append(values, text)
		}
	default:
		return fmt.Errorf("parameters.%s must be a non-empty array of strings for %s", name, action)
	}
	if len(values) == 0 {
		return fmt.Errorf("parameters.%s must be a non-empty array of strings for %s", name, action)
	}
	for _, item := range values {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("parameters.%s must contain only non-empty strings for %s", name, action)
		}
	}
	return nil
}
