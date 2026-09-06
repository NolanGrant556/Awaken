package computer

import "strings"

// PointerSpace identifies the coordinate system of a virtual-cursor hint.
// Desktop Main converts these values into Electron screen DIP before placing
// the click-through overlay cursor.
type PointerSpace string

const (
	// PointerSpacePrimaryDisplayDIP is relative to the primary display origin
	// in Electron DIP / screen points / Windows logical pixels.
	PointerSpacePrimaryDisplayDIP PointerSpace = "primary_display_dip"
	// PointerSpaceScreenDIP is already an absolute virtual-screen DIP point.
	PointerSpaceScreenDIP PointerSpace = "screen_dip"
)

// PointerHint is a non-authoritative visual cue for the Computer Use overlay
// cursor. It never grants input permission and is not a substitute for
// action digest, fence, or checked perform.
type PointerHint struct {
	Kind  ActionKind   `json:"kind"`
	X     *int         `json:"x,omitempty"`
	Y     *int         `json:"y,omitempty"`
	ToX   *int         `json:"to_x,omitempty"`
	ToY   *int         `json:"to_y,omitempty"`
	Space PointerSpace `json:"space,omitempty"`
}

// PointerTarget is an overlay-only click center parsed from accessibility
// geometry. Absolute targets are already screen DIP; other targets are
// window-local pixels and still require a trusted window origin.
type PointerTarget struct {
	Locator  string
	X        int
	Y        int
	Absolute bool
}

// PointerHintForAction derives the overlay cursor target from a prepared
// action and its bound observation. Keyboard, hotkey, type, and wait actions
// have no new pointer location.
func PointerHintForAction(action PreparedAction, obs *Observation) PointerHint {
	hint := PointerHint{Kind: action.Kind}
	switch action.Kind {
	case ActionClick, ActionDoubleClick, ActionScroll, ActionMoveCursor:
		x, y, ok := pointerPoint(action)
		if ok {
			return locatePointer(hint, obs, x, y, nil, nil)
		}
		return pointerHintFromAccessibility(hint, action, obs)
	case ActionDrag:
		fromX, fromOK := integerParameter(action.Parameters, "from_x")
		fromY, fromYOK := integerParameter(action.Parameters, "from_y")
		toX, toOK := integerParameter(action.Parameters, "to_x")
		toY, toYOK := integerParameter(action.Parameters, "to_y")
		if !fromOK || !fromYOK || !toOK || !toYOK {
			return hint
		}
		return locatePointer(hint, obs, fromX, fromY, &toX, &toY)
	default:
		return hint
	}
}

func locatePointer(hint PointerHint, obs *Observation, x, y int, toX, toY *int) PointerHint {
	space := PointerSpacePrimaryDisplayDIP
	if obs != nil && obs.Scope == CaptureWindow {
		if origin, originSpace, ok := windowOrigin(obs); ok {
			x += origin.X
			y += origin.Y
			if toX != nil {
				shifted := *toX + origin.X
				toX = &shifted
			}
			if toY != nil {
				shifted := *toY + origin.Y
				toY = &shifted
			}
			space = originSpace
		} else {
			// Window-local pixels without a trusted screen origin must not be
			// treated as desktop coordinates; the overlay keeps its last point.
			return PointerHint{Kind: hint.Kind}
		}
	}
	hint.X = intPointer(x)
	hint.Y = intPointer(y)
	hint.ToX = toX
	hint.ToY = toY
	hint.Space = space
	return hint
}

func pointerHintFromAccessibility(hint PointerHint, action PreparedAction, obs *Observation) PointerHint {
	token := strings.TrimSpace(action.Target.Locator)
	if token == "" {
		token, _ = stringParameter(action.Parameters, "element_token")
	}
	if token == "" || obs == nil {
		return hint
	}
	for _, target := range obs.PointerTargets {
		if target.Locator != token {
			continue
		}
		if target.Absolute {
			hint.X = intPointer(target.X)
			hint.Y = intPointer(target.Y)
			hint.Space = PointerSpaceScreenDIP
			return hint
		}
		return locatePointer(hint, obs, target.X, target.Y, nil, nil)
	}
	return hint
}

func stringParameter(parameters map[string]any, name string) (string, bool) {
	if parameters == nil {
		return "", false
	}
	text, ok := parameters[name].(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func pointerPoint(action PreparedAction) (int, int, bool) {
	if x, ok := integerParameter(action.Parameters, "x"); ok {
		if y, yok := integerParameter(action.Parameters, "y"); yok {
			return x, y, true
		}
	}
	if action.Target.PixelFallback != nil {
		return action.Target.PixelFallback.X, action.Target.PixelFallback.Y, true
	}
	return 0, 0, false
}

func windowOrigin(obs *Observation) (Point, PointerSpace, bool) {
	if obs == nil || obs.CoordinateSpace == nil || !obs.CoordinateSpace.HasOrigin {
		return Point{}, "", false
	}
	space := PointerSpace(obs.CoordinateSpace.OriginSpace)
	if space == "" {
		space = PointerSpaceScreenDIP
	}
	if space != PointerSpaceScreenDIP && space != PointerSpacePrimaryDisplayDIP {
		return Point{}, "", false
	}
	return Point{X: obs.CoordinateSpace.OriginX, Y: obs.CoordinateSpace.OriginY}, space, true
}

func integerParameter(parameters map[string]any, name string) (int, bool) {
	if parameters == nil {
		return 0, false
	}
	number, ok := numericValue(parameters[name])
	if !ok || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func intPointer(value int) *int {
	return &value
}

func (h PointerHint) Payload() map[string]any {
	payload := map[string]any{
		"pointer_kind": string(h.Kind),
	}
	if h.Space != "" {
		payload["pointer_space"] = string(h.Space)
	}
	if h.X != nil {
		payload["pointer_x"] = *h.X
	}
	if h.Y != nil {
		payload["pointer_y"] = *h.Y
	}
	if h.ToX != nil {
		payload["pointer_to_x"] = *h.ToX
	}
	if h.ToY != nil {
		payload["pointer_to_y"] = *h.ToY
	}
	return payload
}
