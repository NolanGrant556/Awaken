package imagerouting

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		name string
		in   Mode
		want Mode
	}{
		{name: "auto", in: ModeAuto, want: ModeAuto},
		{name: "native", in: ModeNative, want: ModeNative},
		{name: "text", in: ModeText, want: ModeText},
		{name: "case and whitespace", in: Mode(" Native "), want: ModeNative},
		{name: "invalid", in: Mode("pixels"), want: ModeAuto},
		{name: "empty", in: Mode(""), want: ModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeMode(tt.in); got != tt.want {
				t.Fatalf("NormalizeMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDecideRespectsConfiguredModes(t *testing.T) {
	lookup := func(provider, model string) (ModelCapabilities, bool) {
		t.Fatalf("lookup should not be called for explicit mode")
		return ModelCapabilities{}, false
	}

	native := Decide("openai", "gpt-4o", Config{ImageInputMode: ModeNative}, lookup)
	if native.Mode != ModeNative || native.Reason == "" {
		t.Fatalf("expected configured native decision, got %+v", native)
	}

	text := Decide("openai", "gpt-4o", Config{ImageInputMode: ModeText}, lookup)
	if text.Mode != ModeText || text.Reason == "" {
		t.Fatalf("expected configured text decision, got %+v", text)
	}
}

func TestDecideInvalidModeFallsBackToAuto(t *testing.T) {
	decision := Decide("openai", "gpt-4o", Config{ImageInputMode: Mode("bad")}, func(provider, model string) (ModelCapabilities, bool) {
		if provider != "openai" || model != "gpt-4o" {
			t.Fatalf("lookup received provider=%q model=%q", provider, model)
		}
		return ModelCapabilities{SupportsVision: true}, true
	})
	if decision.Mode != ModeNative {
		t.Fatalf("expected invalid mode to fall back to auto native, got %+v", decision)
	}
}

func TestDecideAutoNativeWhenMainModelSupportsVision(t *testing.T) {
	decision := Decide(" openai ", " gpt-4o ", Config{ImageInputMode: ModeAuto}, func(provider, model string) (ModelCapabilities, bool) {
		if provider != "openai" || model != "gpt-4o" {
			t.Fatalf("lookup received provider=%q model=%q", provider, model)
		}
		return ModelCapabilities{SupportsVision: true}, true
	})
	if decision.Mode != ModeNative || !strings.Contains(decision.Reason, "supports vision") {
		t.Fatalf("expected native vision decision, got %+v", decision)
	}
}

func TestDecideAutoTextWithExplicitAuxiliaryVision(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "provider",
			cfg:  Config{Auxiliary: AuxiliaryConfig{Vision: VisionConfig{Provider: "openai"}}},
		},
		{
			name: "model",
			cfg:  Config{Auxiliary: AuxiliaryConfig{Vision: VisionConfig{Provider: "auto", Model: "gpt-4o-mini"}}},
		},
		{
			name: "base url",
			cfg:  Config{Auxiliary: AuxiliaryConfig{Vision: VisionConfig{BaseURL: "https://vision.example.test"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := Decide("openai", "gpt-4o", tt.cfg, func(provider, model string) (ModelCapabilities, bool) {
				t.Fatalf("lookup should not be called with explicit auxiliary vision config")
				return ModelCapabilities{}, false
			})
			if decision.Mode != ModeText || !strings.Contains(decision.Reason, "auxiliary vision") {
				t.Fatalf("expected text aux override decision, got %+v", decision)
			}
		})
	}
}

func TestDecideAutoTextForUnknownOrFalseCapability(t *testing.T) {
	t.Run("nil lookup", func(t *testing.T) {
		decision := Decide("openai", "unknown", Config{}, nil)
		if decision.Mode != ModeText || !strings.Contains(decision.Reason, "unknown") {
			t.Fatalf("expected unknown capability text decision, got %+v", decision)
		}
	})

	t.Run("lookup miss", func(t *testing.T) {
		decision := Decide("openai", "unknown", Config{}, func(provider, model string) (ModelCapabilities, bool) {
			return ModelCapabilities{}, false
		})
		if decision.Mode != ModeText || !strings.Contains(decision.Reason, "unknown") {
			t.Fatalf("expected lookup miss text decision, got %+v", decision)
		}
	})

	t.Run("known false", func(t *testing.T) {
		decision := Decide("openai", "text-only", Config{}, func(provider, model string) (ModelCapabilities, bool) {
			return ModelCapabilities{SupportsVision: false}, true
		})
		if decision.Mode != ModeText || !strings.Contains(decision.Reason, "does not support vision") {
			t.Fatalf("expected no-vision text decision, got %+v", decision)
		}
	})
}

func TestInferMIMEType(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "a.jpg", want: "image/jpeg"},
		{path: "a.jpeg", want: "image/jpeg"},
		{path: "a.png", want: "image/png"},
		{path: "a.gif", want: "image/gif"},
		{path: "a.webp", want: "image/webp"},
		{path: "a.bmp", want: "image/bmp"},
		{path: "a.tiff", want: "image/jpeg"},
		{path: "a", want: "image/jpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := InferMIMEType(tt.path); got != tt.want {
				t.Fatalf("InferMIMEType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildContentPartsDataURLAndMetadata(t *testing.T) {
	path := writeTempImage(t, "sample.png", []byte{0, 1, 2, 3})
	result := BuildContentParts(" look here ", []string{path})

	if len(result.Skipped) != 0 {
		t.Fatalf("expected no skipped images, got %+v", result.Skipped)
	}
	if len(result.Parts) != 2 {
		t.Fatalf("expected text plus image part, got %+v", result.Parts)
	}
	if result.Parts[0].Type != "text" || result.Parts[0].Text != "look here" {
		t.Fatalf("unexpected text part: %+v", result.Parts[0])
	}
	wantURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3})
	if result.Parts[1].Type != "image_url" || result.Parts[1].ImageURL == nil || result.Parts[1].ImageURL.URL != wantURL {
		t.Fatalf("unexpected image part: %+v", result.Parts[1])
	}
	if len(result.Images) != 1 {
		t.Fatalf("expected one image metadata entry, got %+v", result.Images)
	}
	meta := result.Images[0]
	if meta.Path != path || meta.MIMEType != "image/png" || meta.Bytes != 4 || meta.Base64Size != 8 || meta.PartIndex != 1 {
		t.Fatalf("unexpected image metadata: %+v", meta)
	}
}

func TestBuildContentPartsUnknownMIMEDefaultsToJPEG(t *testing.T) {
	path := writeTempImage(t, "sample.image", []byte("raw"))
	result := BuildContentParts("look", []string{path})

	if len(result.Parts) != 2 || result.Parts[1].ImageURL == nil {
		t.Fatalf("expected image URL part, got %+v", result.Parts)
	}
	if !strings.HasPrefix(result.Parts[1].ImageURL.URL, "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg data URL fallback, got %q", result.Parts[1].ImageURL.URL)
	}
	if len(result.Images) != 1 || result.Images[0].MIMEType != "image/jpeg" {
		t.Fatalf("expected jpeg metadata fallback, got %+v", result.Images)
	}
}

func TestBuildContentPartsMissingImageSkipped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	result := BuildContentParts("look", []string{missing})

	if len(result.Parts) != 1 || result.Parts[0].Type != "text" {
		t.Fatalf("expected only text part, got %+v", result.Parts)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Path != missing || !strings.Contains(result.Skipped[0].Reason, "read failed") {
		t.Fatalf("expected missing path to be skipped, got %+v", result.Skipped)
	}
}

func TestBuildContentPartsEmptyTextAddsNeutralPrompt(t *testing.T) {
	path := writeTempImage(t, "sample.gif", []byte("gif"))
	result := BuildContentParts("  ", []string{path})

	if len(result.Parts) != 2 {
		t.Fatalf("expected neutral prompt plus image part, got %+v", result.Parts)
	}
	if result.Parts[0].Type != "text" || result.Parts[0].Text != NeutralImagePrompt {
		t.Fatalf("expected neutral prompt, got %+v", result.Parts[0])
	}
	if result.Parts[1].Type != "image_url" || result.Parts[1].ImageURL == nil {
		t.Fatalf("expected image part after neutral prompt, got %+v", result.Parts[1])
	}
	if len(result.Images) != 1 || result.Images[0].PartIndex != 1 {
		t.Fatalf("expected image metadata part index to account for prompt insertion, got %+v", result.Images)
	}
}

func writeTempImage(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp image: %v", err)
	}
	return path
}
