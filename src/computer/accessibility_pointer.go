package computer

import (
	"encoding/json"
	"strings"
)

// PointerTargetsFromAccessibilityTree extracts overlay click centers from a
// CUA get_window_state structured tree. This is visual-only and does not
// change how the Host performs the accessibility action.
func PointerTargetsFromAccessibilityTree(tree []byte, space *CoordinateSpace) []PointerTarget {
	if len(tree) == 0 {
		return nil
	}
	var root any
	if err := json.Unmarshal(tree, &root); err != nil {
		return nil
	}
	var targets []PointerTarget
	collectPointerTargets(root, space, &targets)
	return targets
}

func collectPointerTargets(node any, space *CoordinateSpace, out *[]PointerTarget) {
	switch value := node.(type) {
	case map[string]any:
		if token := accessibilityElementToken(value); token != "" {
			if target, ok := pointerTargetFromAccessibilityElement(token, value, space); ok {
				*out = append(*out, target)
			}
		}
		for _, child := range value {
			collectPointerTargets(child, space, out)
		}
	case []any:
		for _, child := range value {
			collectPointerTargets(child, space, out)
		}
	}
}

func accessibilityElementToken(element map[string]any) string {
	for _, key := range []string{"element_token", "token", "locator"} {
		text, ok := element[key].(string)
		text = strings.TrimSpace(text)
		if ok && text != "" {
			return text
		}
	}
	return ""
}

func pointerTargetFromAccessibilityElement(token string, element map[string]any, space *CoordinateSpace) (PointerTarget, bool) {
	if rect, ok := namedRect(element, "frame"); ok {
		x, y := rectCenter(rect)
		x, y = scaleAbsoluteScreenPoint(x, y, space)
		return PointerTarget{Locator: token, X: x, Y: y, Absolute: true}, true
	}
	if rect, ok := namedRect(element, "bounds"); ok {
		x, y := rectCenter(rect)
		return PointerTarget{Locator: token, X: x, Y: y, Absolute: false}, true
	}
	if rect, ok := topLevelRect(element); ok {
		x, y := rectCenter(rect)
		return PointerTarget{Locator: token, X: x, Y: y, Absolute: false}, true
	}
	x, hasX := jsonInt(element["x"])
	y, hasY := jsonInt(element["y"])
	if hasX && hasY {
		return PointerTarget{Locator: token, X: x, Y: y, Absolute: false}, true
	}
	return PointerTarget{}, false
}

type accessibilityRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func namedRect(element map[string]any, key string) (accessibilityRect, bool) {
	nested, ok := element[key].(map[string]any)
	if !ok {
		return accessibilityRect{}, false
	}
	return rectFromMap(nested)
}

func topLevelRect(element map[string]any) (accessibilityRect, bool) {
	width, hasWidth := jsonInt(firstPresent(element, "width", "w"))
	height, hasHeight := jsonInt(firstPresent(element, "height", "h"))
	if !hasWidth || !hasHeight || width <= 0 || height <= 0 {
		return accessibilityRect{}, false
	}
	x, hasX := jsonInt(element["x"])
	y, hasY := jsonInt(element["y"])
	if !hasX || !hasY {
		return accessibilityRect{}, false
	}
	return accessibilityRect{X: x, Y: y, Width: width, Height: height}, true
}

func rectFromMap(values map[string]any) (accessibilityRect, bool) {
	x, hasX := jsonInt(values["x"])
	y, hasY := jsonInt(values["y"])
	if !hasX || !hasY {
		return accessibilityRect{}, false
	}
	width, hasWidth := jsonInt(firstPresent(values, "width", "w"))
	height, hasHeight := jsonInt(firstPresent(values, "height", "h"))
	if hasWidth && hasHeight {
		if width <= 0 || height <= 0 {
			return accessibilityRect{}, false
		}
		return accessibilityRect{X: x, Y: y, Width: width, Height: height}, true
	}
	return accessibilityRect{X: x, Y: y, Width: 1, Height: 1}, true
}

func rectCenter(rect accessibilityRect) (int, int) {
	return rect.X + rect.Width/2, rect.Y + rect.Height/2
}

func scaleAbsoluteScreenPoint(x, y int, space *CoordinateSpace) (int, int) {
	if space == nil || space.Unit != "logical_pixel" || space.ScaleX < 1 || space.ScaleY < 1 {
		return x, y
	}
	return int(float64(x) / space.ScaleX), int(float64(y) / space.ScaleY)
}

func firstPresent(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func jsonInt(value any) (int, bool) {
	number, ok := numericValue(value)
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}
