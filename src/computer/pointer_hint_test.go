package computer

import "testing"

func TestPointerHintForDesktopClickUsesPrimaryDisplaySpace(t *testing.T) {
	hint := PointerHintForAction(PreparedAction{
		Kind:       ActionClick,
		Parameters: map[string]any{"x": 100, "y": 40},
	}, &Observation{Scope: CaptureDesktop})
	if hint.Kind != ActionClick || hint.Space != PointerSpacePrimaryDisplayDIP {
		t.Fatalf("unexpected hint: %+v", hint)
	}
	if hint.X == nil || hint.Y == nil || *hint.X != 100 || *hint.Y != 40 {
		t.Fatalf("expected desktop (100,40), got %+v %+v", hint.X, hint.Y)
	}
}

func TestPointerHintAllowsOriginZero(t *testing.T) {
	hint := PointerHintForAction(PreparedAction{
		Kind:       ActionMoveCursor,
		Parameters: map[string]any{"x": 0, "y": 0},
	}, &Observation{Scope: CaptureDesktop})
	if hint.X == nil || hint.Y == nil || *hint.X != 0 || *hint.Y != 0 {
		t.Fatalf("origin (0,0) must remain a valid pointer, got %+v", hint)
	}
}

func TestPointerHintShiftsWindowPixelsByTrustedOrigin(t *testing.T) {
	hint := PointerHintForAction(PreparedAction{
		Kind:       ActionClick,
		Parameters: map[string]any{"x": 12, "y": 8},
	}, &Observation{
		Scope: CaptureWindow,
		CoordinateSpace: &CoordinateSpace{
			HasOrigin:   true,
			OriginX:     200,
			OriginY:     80,
			OriginSpace: string(PointerSpaceScreenDIP),
		},
	})
	if hint.Space != PointerSpaceScreenDIP || hint.X == nil || hint.Y == nil || *hint.X != 212 || *hint.Y != 88 {
		t.Fatalf("expected screen (212,88), got %+v", hint)
	}
}

func TestPointerHintIgnoresWindowPixelsWithoutOrigin(t *testing.T) {
	hint := PointerHintForAction(PreparedAction{
		Kind:       ActionClick,
		Parameters: map[string]any{"x": 12, "y": 8},
	}, &Observation{Scope: CaptureWindow})
	if hint.X != nil || hint.Y != nil || hint.Space != "" {
		t.Fatalf("window-local pixels without origin must not become screen coords: %+v", hint)
	}
}

func TestPointerHintDragAndFallback(t *testing.T) {
	drag := PointerHintForAction(PreparedAction{
		Kind: ActionDrag,
		Parameters: map[string]any{
			"from_x": 1, "from_y": 2, "to_x": 5, "to_y": 9,
		},
	}, &Observation{Scope: CaptureDesktop})
	if drag.X == nil || drag.ToX == nil || *drag.X != 1 || *drag.ToX != 5 || *drag.ToY != 9 {
		t.Fatalf("unexpected drag hint: %+v", drag)
	}

	fallback := PointerHintForAction(PreparedAction{
		Kind:   ActionClick,
		Target: ElementRef{Locator: "s1:2", PixelFallback: &Point{X: 7, Y: 11}},
	}, &Observation{Scope: CaptureDesktop})
	if fallback.X == nil || *fallback.X != 7 || *fallback.Y != 11 {
		t.Fatalf("expected pixel fallback, got %+v", fallback)
	}

	wait := PointerHintForAction(PreparedAction{Kind: ActionWait}, &Observation{Scope: CaptureDesktop})
	if wait.X != nil || wait.Space != "" {
		t.Fatalf("wait must not move the pointer: %+v", wait)
	}
}
