package computer

import (
	"strings"
	"testing"
)

func TestValidateActionSpecRequiresCompleteActionParameters(t *testing.T) {
	tests := []struct {
		name string
		spec ActionSpec
		want string
	}{
		{name: "click missing both", spec: ActionSpec{Kind: ActionClick}, want: "parameters.x is required"},
		{name: "click missing y", spec: ActionSpec{Kind: ActionClick, Parameters: map[string]any{"x": 0}}, want: "parameters.y is required"},
		{name: "click missing x", spec: ActionSpec{Kind: ActionClick, Parameters: map[string]any{"y": 10}}, want: "parameters.x is required"},
		{name: "negative coordinate", spec: ActionSpec{Kind: ActionMoveCursor, Parameters: map[string]any{"x": -1, "y": 10}}, want: "must be non-negative"},
		{name: "fractional coordinate", spec: ActionSpec{Kind: ActionScroll, Parameters: map[string]any{"x": 1.5, "y": 10}}, want: "must be an integer"},
		{name: "drag incomplete", spec: ActionSpec{Kind: ActionDrag, Parameters: map[string]any{"from_x": 1, "from_y": 2, "to_x": 3}}, want: "parameters.to_y is required"},
		{name: "key missing", spec: ActionSpec{Kind: ActionKey}, want: "parameters.key is required"},
		{name: "hotkey empty", spec: ActionSpec{Kind: ActionHotkey, Parameters: map[string]any{"keys": []string{}}}, want: "non-empty array"},
		{name: "type text empty", spec: ActionSpec{Kind: ActionTypeText, Parameters: map[string]any{"text": ""}}, want: "non-empty string"},
		{name: "invalid delivery mode", spec: ActionSpec{Kind: ActionClick, Parameters: map[string]any{"x": 1, "y": 2, "delivery_mode": "auto"}}, want: "delivery_mode must be background or foreground"},
		{name: "wait negative", spec: ActionSpec{Kind: ActionWait, Parameters: map[string]any{"ms": -1}}, want: "must be non-negative"},
		{name: "unknown action", spec: ActionSpec{Kind: "teleport"}, want: "unsupported action"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateActionSpec(test.spec)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateActionSpec() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateActionSpecAcceptsZeroCoordinateAndValidKeyboardActions(t *testing.T) {
	tests := []ActionSpec{
		{Kind: ActionClick, Target: ElementRef{PixelFallback: &Point{X: 0, Y: 0}}},
		{Kind: ActionClick, Target: ElementRef{Locator: "s00000001:4"}, Parameters: map[string]any{"delivery_mode": "background"}},
		{Kind: ActionMoveCursor, Parameters: map[string]any{"x": float64(0), "y": float64(10)}},
		{Kind: ActionDrag, Parameters: map[string]any{"from_x": 0, "from_y": 1, "to_x": 2, "to_y": 3}},
		{Kind: ActionKey, Parameters: map[string]any{"key": "Enter"}},
		{Kind: ActionHotkey, Parameters: map[string]any{"keys": []any{"command", "space"}}},
		{Kind: ActionTypeText, Parameters: map[string]any{"text": "hello"}},
		{Kind: ActionWait},
	}
	for _, spec := range tests {
		if err := ValidateActionSpec(spec); err != nil {
			t.Fatalf("ValidateActionSpec(%s) unexpected error: %v", spec.Kind, err)
		}
	}
}
