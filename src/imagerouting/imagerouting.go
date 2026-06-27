package imagerouting

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeNative Mode = "native"
	ModeText   Mode = "text"
)

const NeutralImagePrompt = "What do you see in this image?"

type Config struct {
	ImageInputMode Mode            `json:"image_input_mode"`
	Auxiliary      AuxiliaryConfig `json:"auxiliary"`
}

type AuxiliaryConfig struct {
	Vision VisionConfig `json:"vision"`
}

type VisionConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
}

type ModelCapabilities struct {
	SupportsVision bool
}

type CapabilityLookup func(provider, model string) (ModelCapabilities, bool)

type Decision struct {
	Mode   Mode   `json:"mode"`
	Reason string `json:"reason"`
}

func NormalizeMode(mode Mode) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case ModeNative:
		return ModeNative
	case ModeText:
		return ModeText
	default:
		return ModeAuto
	}
}

func Decide(provider, model string, cfg Config, lookup CapabilityLookup) Decision {
	switch NormalizeMode(cfg.ImageInputMode) {
	case ModeNative:
		return Decision{Mode: ModeNative, Reason: "configured image_input_mode=native"}
	case ModeText:
		return Decision{Mode: ModeText, Reason: "configured image_input_mode=text"}
	}

	if cfg.hasExplicitAuxiliaryVision() {
		return Decision{Mode: ModeText, Reason: "auto: explicit auxiliary vision configuration uses text image pipeline"}
	}

	if lookup == nil {
		return Decision{Mode: ModeText, Reason: "auto: main model vision capability unknown"}
	}

	caps, ok := lookup(strings.TrimSpace(provider), strings.TrimSpace(model))
	if !ok {
		return Decision{Mode: ModeText, Reason: "auto: main model vision capability unknown"}
	}
	if caps.SupportsVision {
		return Decision{Mode: ModeNative, Reason: "auto: main model supports vision"}
	}
	return Decision{Mode: ModeText, Reason: "auto: main model does not support vision"}
}

func (cfg Config) hasExplicitAuxiliaryVision() bool {
	vision := cfg.Auxiliary.Vision
	provider := strings.ToLower(strings.TrimSpace(vision.Provider))
	model := strings.TrimSpace(vision.Model)
	baseURL := strings.TrimSpace(vision.BaseURL)
	return (provider != "" && provider != "auto") || model != "" || baseURL != ""
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type ImageMetadata struct {
	Path       string `json:"path"`
	MIMEType   string `json:"mime_type"`
	Bytes      int64  `json:"bytes"`
	Base64Size int    `json:"base64_size"`
	PartIndex  int    `json:"part_index"`
}

type SkippedImage struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ContentPartsResult struct {
	Parts   []ContentPart   `json:"parts"`
	Images  []ImageMetadata `json:"images"`
	Skipped []SkippedImage  `json:"skipped"`
}

func BuildContentParts(text string, imagePaths []string) ContentPartsResult {
	result := ContentPartsResult{
		Parts:   []ContentPart{},
		Images:  []ImageMetadata{},
		Skipped: []SkippedImage{},
	}

	trimmedText := strings.TrimSpace(text)
	if trimmedText != "" {
		result.Parts = append(result.Parts, ContentPart{Type: "text", Text: trimmedText})
	}

	for _, rawPath := range imagePaths {
		if strings.TrimSpace(rawPath) == "" {
			result.Skipped = append(result.Skipped, SkippedImage{Path: rawPath, Reason: "empty path"})
			continue
		}

		data, err := os.ReadFile(rawPath)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedImage{
				Path:   rawPath,
				Reason: fmt.Sprintf("read failed: %v", err),
			})
			continue
		}

		mimeType := InferMIMEType(rawPath)
		encoded := base64.StdEncoding.EncodeToString(data)
		dataURL := "data:" + mimeType + ";base64," + encoded

		partIndex := len(result.Parts)
		result.Parts = append(result.Parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: dataURL},
		})
		result.Images = append(result.Images, ImageMetadata{
			Path:       rawPath,
			MIMEType:   mimeType,
			Bytes:      int64(len(data)),
			Base64Size: len(encoded),
			PartIndex:  partIndex,
		})
	}

	if trimmedText == "" && len(result.Images) > 0 {
		result.Parts = append([]ContentPart{{Type: "text", Text: NeutralImagePrompt}}, result.Parts...)
		for i := range result.Images {
			result.Images[i].PartIndex++
		}
	}

	return result
}

func InferMIMEType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/jpeg"
	}
}
