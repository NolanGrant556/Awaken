package artifactruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

const (
	runtimeSchemaVersion = 2
	runtimeName          = "primary-runtime"
)

// RuntimeManifestTemplate is the schemaVersion=2 template shipped with the app.
type RuntimeManifestTemplate struct {
	SchemaVersion int                       `json:"schemaVersion"`
	RuntimeName   string                    `json:"runtimeName"`
	Name          string                    `json:"name,omitempty"`
	BundleVersion string                    `json:"bundleVersion"`
	Version       string                    `json:"version,omitempty"`
	Mode          string                    `json:"mode,omitempty"`
	Components    []RuntimeComponentTemplate `json:"components"`
}

type RuntimeComponentTemplate struct {
	Name                     string            `json:"name"`
	Kind                     string            `json:"kind"`
	Required                 bool              `json:"required"`
	Version                  string            `json:"version,omitempty"`
	Source                   string            `json:"source,omitempty"`
	RelativePath             string            `json:"relativePath,omitempty"`
	RelativePathByPlatform   map[string]string `json:"relativePathByPlatform,omitempty"`
	Path                     string            `json:"path,omitempty"`
	PathByPlatform           map[string]string `json:"pathByPlatform,omitempty"`
	SHA256                   string            `json:"sha256,omitempty"`
	Probe                    *RuntimeProbeSpec `json:"probe,omitempty"`
}

type RuntimeProbeSpec struct {
	Args                              []string          `json:"args,omitempty"`
	AcceptedExitCodes                 []int             `json:"acceptedExitCodes,omitempty"`
	Tools                             []string          `json:"tools,omitempty"`
	RequireFile                       string            `json:"requireFile,omitempty"`
	ExecutableRelativePath            string            `json:"executableRelativePath,omitempty"`
	ExecutableRelativePathByPlatform  map[string]string `json:"executableRelativePathByPlatform,omitempty"`
}

// RuntimeCurrentPointer is the atomic activation pointer (no directory symlink).
type RuntimeCurrentPointer struct {
	SchemaVersion int    `json:"schemaVersion"`
	RuntimeName   string `json:"runtimeName"`
	Active        string `json:"active"`
	Previous      string `json:"previous,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

// RuntimeResolvedState is written into versions/<key>/state.json after probe.
type RuntimeResolvedState struct {
	SchemaVersion int                      `json:"schemaVersion"`
	RuntimeName   string                   `json:"runtimeName"`
	BundleVersion string                   `json:"bundleVersion"`
	Platform      string                   `json:"platform"`
	Arch          string                   `json:"arch"`
	Root          string                   `json:"root,omitempty"`
	Components    []RuntimeResolvedComponent `json:"components"`
	GeneratedAt   string                   `json:"generatedAt,omitempty"`
}

type RuntimeResolvedComponent struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	Required     bool              `json:"required"`
	Version      string            `json:"version,omitempty"`
	Source       string            `json:"source,omitempty"`
	RelativePath string            `json:"relativePath"`
	SHA256       string            `json:"sha256,omitempty"`
	Probe        *RuntimeProbeSpec `json:"probe,omitempty"`
}

func loadRuntimeManifestTemplate(path string) (RuntimeManifestTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeManifestTemplate{}, err
	}
	var manifest RuntimeManifestTemplate
	if err := json.Unmarshal(data, &manifest); err != nil {
		return RuntimeManifestTemplate{}, err
	}
	if strings.TrimSpace(manifest.RuntimeName) == "" {
		manifest.RuntimeName = firstNonEmpty(manifest.Name, runtimeName)
	}
	if strings.TrimSpace(manifest.BundleVersion) == "" {
		manifest.BundleVersion = firstNonEmpty(manifest.Version, "0.2.0")
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = runtimeSchemaVersion
	}
	return manifest, nil
}

func loadRuntimeCurrent(path string) (RuntimeCurrentPointer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeCurrentPointer{}, err
	}
	var current RuntimeCurrentPointer
	if err := json.Unmarshal(data, &current); err != nil {
		return RuntimeCurrentPointer{}, err
	}
	return current, nil
}

func loadRuntimeResolvedState(path string) (RuntimeResolvedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeResolvedState{}, err
	}
	var state RuntimeResolvedState
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeResolvedState{}, err
	}
	return state, nil
}

func componentRelativePath(component RuntimeComponentTemplate) string {
	for _, key := range runtimePlatformPathKeys() {
		if path := strings.TrimSpace(component.RelativePathByPlatform[key]); path != "" {
			return stripLegacyRuntimePrefix(path)
		}
		if path := strings.TrimSpace(component.PathByPlatform[key]); path != "" {
			return stripLegacyRuntimePrefix(path)
		}
	}
	if path := strings.TrimSpace(component.RelativePath); path != "" {
		return stripLegacyRuntimePrefix(path)
	}
	return stripLegacyRuntimePrefix(strings.TrimSpace(component.Path))
}

func stripLegacyRuntimePrefix(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, `\`, `/`)
	path = strings.TrimPrefix(path, "../runtime/")
	return filepath.FromSlash(path)
}

func runtimePlatformPathKeys() []string {
	switch goruntime.GOOS {
	case "windows":
		return []string{"windows", "win32", "default"}
	default:
		return []string{goruntime.GOOS, "default"}
	}
}

func runtimePlatformName() string {
	if goruntime.GOOS == "windows" {
		return "windows"
	}
	return goruntime.GOOS
}

func runtimeVersionKey(bundleVersion string) string {
	return bundleVersion + "/" + runtimePlatformName() + "-" + goruntime.GOARCH
}
