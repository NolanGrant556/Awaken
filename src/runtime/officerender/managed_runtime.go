package officerender

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const managedMacOSBundleIdentifier = "com.awaken.runtime.libreoffice-renderer"

type managedCurrent struct {
	Active string `json:"active"`
}

type managedState struct {
	BundleVersion string `json:"bundleVersion"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	Components    []struct {
		Name         string `json:"name"`
		RelativePath string `json:"relativePath"`
	} `json:"components"`
	OfficeRender struct {
		Component                 string `json:"component"`
		MacOSActivationPolicy     string `json:"macOSActivationPolicy"`
		MacOSBundleIdentifier     string `json:"macOSBundleIdentifier"`
		MacOSLaunchServicesClaims string `json:"macOSLaunchServicesClaims"`
	} `json:"officeRender"`
}

type resolvedManagedRuntime struct {
	runtime Runtime
	state   managedState
}

// ManagedRuntimeResolver derives executable authority from current.json and
// state.json owned by the Awaken primary runtime. Callers never supply a binary.
type ManagedRuntimeResolver struct {
	root string
}

func NewManagedRuntimeResolver(root string) (*ManagedRuntimeResolver, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		var err error
		root, err = DefaultManagedRuntimeRoot()
		if err != nil {
			return nil, err
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if canonical, canonicalErr := filepath.EvalSymlinks(absolute); canonicalErr == nil {
		absolute = canonical
	}
	return &ManagedRuntimeResolver{root: filepath.Clean(absolute)}, nil
}

func DefaultManagedRuntimeRoot() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("AWAKEN_PRIMARY_RUNTIME_ROOT")); explicit != "" {
		return filepath.Abs(explicit)
	}
	userData := strings.TrimSpace(os.Getenv("AWAKEN_USER_DATA_DIR"))
	if userData == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		switch runtime.GOOS {
		case "darwin":
			userData = filepath.Join(home, "Library", "Application Support", "@awaken", "desktop")
		case "windows":
			if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
				userData = filepath.Join(appData, "@awaken", "desktop")
			} else {
				userData = filepath.Join(home, "AppData", "Roaming", "@awaken", "desktop")
			}
		default:
			if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
				userData = filepath.Join(xdg, "@awaken", "desktop")
			} else {
				userData = filepath.Join(home, ".config", "@awaken", "desktop")
			}
		}
	}
	return filepath.Join(userData, "runtimes", "primary-runtime"), nil
}

func (r *ManagedRuntimeResolver) Resolve(ctx context.Context) (Runtime, error) {
	resolved, err := r.resolve(ctx)
	return resolved.runtime, err
}

func (r *ManagedRuntimeResolver) Attest(ctx context.Context, expected Runtime) error {
	resolved, err := r.resolve(ctx)
	if err != nil {
		return err
	}
	actual := resolved.runtime
	if expected.Revision != actual.Revision || filepath.Clean(expected.Root) != actual.Root ||
		filepath.Clean(expected.SOfficePath) != actual.SOfficePath || expected.Attestation != actual.Attestation {
		return fmt.Errorf("%w: active managed runtime changed before execution", ErrRuntimeAttestation)
	}
	if err := attestInternalSymlinks(actual.Root); err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeAttestation, err)
	}
	if runtime.GOOS == "darwin" {
		if err := attestManagedMacOSApplication(ctx, actual, resolved.state); err != nil {
			return fmt.Errorf("%w: %v", ErrRuntimeAttestation, err)
		}
	}
	after, err := r.resolve(ctx)
	if err != nil || after.runtime.Attestation != actual.Attestation {
		return fmt.Errorf("%w: managed runtime changed during attestation", ErrRuntimeAttestation)
	}
	return nil
}

func (r *ManagedRuntimeResolver) NewWorker(config Config) *OfficeRenderWorker {
	extraAttestor := config.Attest
	config.ResolveRuntime = r.Resolve
	config.Attest = func(ctx context.Context, runtimeSpec Runtime) error {
		if err := r.Attest(ctx, runtimeSpec); err != nil {
			return err
		}
		if extraAttestor != nil {
			return extraAttestor(ctx, runtimeSpec)
		}
		return nil
	}
	return NewOfficeRenderWorker(config)
}

func (r *ManagedRuntimeResolver) resolve(ctx context.Context) (resolvedManagedRuntime, error) {
	if err := ctx.Err(); err != nil {
		return resolvedManagedRuntime{}, err
	}
	currentBytes, err := os.ReadFile(filepath.Join(r.root, "current.json"))
	if err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("read primary runtime current.json: %w", err)
	}
	var current managedCurrent
	if err := json.Unmarshal(currentBytes, &current); err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("decode primary runtime current.json: %w", err)
	}
	parts, err := safeRevisionParts(current.Active)
	if err != nil {
		return resolvedManagedRuntime{}, err
	}
	versionRoot := filepath.Join(append([]string{r.root, "versions"}, parts...)...)
	versionRoot, err = filepath.Abs(versionRoot)
	if err != nil {
		return resolvedManagedRuntime{}, err
	}
	if inside, relErr := pathInside(versionRoot, r.root); relErr != nil || !inside {
		return resolvedManagedRuntime{}, fmt.Errorf("active runtime revision escapes managed root")
	}
	stateBytes, err := os.ReadFile(filepath.Join(versionRoot, "state.json"))
	if err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("read active runtime state.json: %w", err)
	}
	var state managedState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("decode active runtime state.json: %w", err)
	}
	if canonical, canonicalErr := filepath.EvalSymlinks(versionRoot); canonicalErr == nil {
		versionRoot = canonical
	}
	if state.Platform != "" && state.Platform != managedPlatformName() {
		return resolvedManagedRuntime{}, fmt.Errorf("active runtime platform mismatch: %s", state.Platform)
	}
	if state.Arch != "" && state.Arch != runtime.GOARCH {
		return resolvedManagedRuntime{}, fmt.Errorf("active runtime architecture mismatch: %s", state.Arch)
	}
	componentName := strings.TrimSpace(state.OfficeRender.Component)
	if componentName == "" {
		componentName = "libreoffice"
	}
	componentPath := ""
	for _, component := range state.Components {
		if component.Name == componentName {
			componentPath = strings.TrimSpace(component.RelativePath)
			break
		}
	}
	if componentPath == "" || filepath.IsAbs(componentPath) {
		return resolvedManagedRuntime{}, fmt.Errorf("active runtime has no bounded LibreOffice component")
	}
	componentRoot := filepath.Join(versionRoot, filepath.FromSlash(componentPath))
	componentRoot, err = filepath.EvalSymlinks(componentRoot)
	if err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("resolve LibreOffice component: %w", err)
	}
	if inside, relErr := pathInside(componentRoot, versionRoot); relErr != nil || !inside {
		return resolvedManagedRuntime{}, fmt.Errorf("LibreOffice component escapes active revision")
	}
	soffice := findManagedSoffice(componentRoot)
	if soffice == "" {
		return resolvedManagedRuntime{}, fmt.Errorf("soffice is missing from active runtime")
	}
	soffice, err = filepath.EvalSymlinks(soffice)
	if err != nil {
		return resolvedManagedRuntime{}, fmt.Errorf("resolve soffice: %w", err)
	}
	if inside, relErr := pathInside(soffice, versionRoot); relErr != nil || !inside {
		return resolvedManagedRuntime{}, fmt.Errorf("soffice escapes active revision")
	}
	digest := sha256.Sum256(append([]byte(current.Active+"\x00"), stateBytes...))
	return resolvedManagedRuntime{
		runtime: Runtime{
			Revision:    current.Active,
			Root:        versionRoot,
			SOfficePath: soffice,
			Attestation: hex.EncodeToString(digest[:]),
		},
		state: state,
	}, nil
}

func safeRevisionParts(active string) ([]string, error) {
	active = strings.TrimSpace(strings.ReplaceAll(active, `\`, "/"))
	parts := strings.Split(active, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid active runtime revision %q", active)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\`) {
			return nil, fmt.Errorf("invalid active runtime revision %q", active)
		}
	}
	return parts, nil
}

func findManagedSoffice(componentRoot string) string {
	candidates := []string{componentRoot}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates, filepath.Join(componentRoot, "Contents", "MacOS", "soffice"))
	case "windows":
		candidates = append(candidates,
			filepath.Join(componentRoot, "program", "soffice.com"),
			filepath.Join(componentRoot, "program", "soffice.exe"),
			filepath.Join(componentRoot, "program", "soffice.bin.exe"),
		)
	default:
		candidates = append(candidates,
			filepath.Join(componentRoot, "program", "soffice"),
			filepath.Join(componentRoot, "soffice"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

func managedPlatformName() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return runtime.GOOS
}

func attestInternalSymlinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("broken managed runtime symlink %s", path)
		}
		inside, err := pathInside(resolved, root)
		if err != nil || !inside {
			return fmt.Errorf("managed runtime symlink escapes revision: %s", path)
		}
		return nil
	})
}

func attestManagedMacOSApplication(ctx context.Context, runtimeSpec Runtime, state managedState) error {
	appRoot := ""
	for cursor := filepath.Dir(runtimeSpec.SOfficePath); ; cursor = filepath.Dir(cursor) {
		if strings.HasSuffix(strings.ToLower(cursor), ".app") {
			appRoot = cursor
			break
		}
		if cursor == runtimeSpec.Root || filepath.Dir(cursor) == cursor {
			break
		}
	}
	if appRoot == "" {
		return fmt.Errorf("managed macOS soffice is not inside an application bundle")
	}
	if output, err := exec.CommandContext(ctx, "codesign", "--verify", "--deep", "--strict", "--verbose=2", appRoot).CombinedOutput(); err != nil {
		return fmt.Errorf("strict codesign verification failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	expectedIdentifier := strings.TrimSpace(state.OfficeRender.MacOSBundleIdentifier)
	if expectedIdentifier == "" {
		expectedIdentifier = managedMacOSBundleIdentifier
	}
	infoPlist := filepath.Join(appRoot, "Contents", "Info.plist")
	if value, err := plutilValue(ctx, infoPlist, "LSBackgroundOnly"); err != nil || value != "true" {
		return fmt.Errorf("managed macOS LibreOffice is not background-only")
	}
	if value, err := plutilValue(ctx, infoPlist, "CFBundleIdentifier"); err != nil || value != expectedIdentifier {
		return fmt.Errorf("managed macOS LibreOffice bundle identifier mismatch")
	}
	for _, key := range []string{"CFBundleDocumentTypes", "CFBundleURLTypes", "NSServices", "UTExportedTypeDeclarations", "UTImportedTypeDeclarations"} {
		if _, err := plutilValue(ctx, infoPlist, key); err == nil {
			return fmt.Errorf("managed macOS LibreOffice still declares %s", key)
		}
	}
	for _, path := range []string{
		filepath.Join(appRoot, "Contents", "Library", "Spotlight", "OOoSpotlightImporter.mdimporter"),
		filepath.Join(appRoot, "Contents", "PlugIns", "QuickLookPreview.appex"),
		filepath.Join(appRoot, "Contents", "PlugIns", "QuickLookThumbnail.appex"),
	} {
		if _, err := os.Stat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("managed macOS LibreOffice still contains registration bundle %s", path)
		}
	}
	return nil
}

func plutilValue(ctx context.Context, plist, key string) (string, error) {
	output, err := exec.CommandContext(ctx, "plutil", "-extract", key, "raw", "-o", "-", plist).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
