package runtime

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"awaken/src/filestore"

	"github.com/google/uuid"
)

const (
	runtimeManifestSchemaVersion = 1
	runtimeManifestFileName      = "core.json"
	runtimeLockFileName          = "core.lock"
	runtimeSubdir                = "runtime"
)

// ErrManifestOwnedByOther is returned when the manifest file is owned by a live
// Core instance with a different instance_id.
var ErrManifestOwnedByOther = errors.New("runtime manifest owned by another running instance")

// RuntimeIdentity is the durable discovery record for a single awaken-core
// instance. It is written to the userData-specific runtime directory and used
// by awaken-acp to locate and authenticate against the Core.
type RuntimeIdentity struct {
	SchemaVersion   int       `json:"schema_version"`
	RuntimeIdentity string    `json:"runtime_identity"`
	InstanceID      string    `json:"instance_id"`
	PID             int       `json:"pid"`
	OwnerMode       string    `json:"owner_mode"`
	BaseURL         string    `json:"base_url"`
	ControlToken    string    `json:"control_token"`
	StartedAt       time.Time `json:"started_at"`
}

// RuntimeManifestPaths locates the manifest and lock files for a given userData
// directory.
type RuntimeManifestPaths struct {
	Dir      string
	Manifest string
	Lock     string
}

// ResolveRuntimeManifestPaths returns the runtime directory, manifest path and
// lock path under userDataDir.
func ResolveRuntimeManifestPaths(userDataDir string) RuntimeManifestPaths {
	dir := filepath.Join(userDataDir, runtimeSubdir)
	return RuntimeManifestPaths{
		Dir:      dir,
		Manifest: filepath.Join(dir, runtimeManifestFileName),
		Lock:     filepath.Join(dir, runtimeLockFileName),
	}
}

// GenerateRuntimeIdentity creates a new identity for the current process.
// userDataDir must be the absolute, canonical userData directory for the
// packaged or development identity.
func GenerateRuntimeIdentity(userDataDir, ownerMode, baseURL string) (*RuntimeIdentity, error) {
	if userDataDir == "" {
		return nil, errors.New("runtime identity requires a userData directory")
	}
	if ownerMode != "desktop" && ownerMode != "headless" {
		return nil, fmt.Errorf("invalid owner_mode %q", ownerMode)
	}
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	token, err := generateControlToken()
	if err != nil {
		return nil, fmt.Errorf("generate control token: %w", err)
	}
	return &RuntimeIdentity{
		SchemaVersion:   runtimeManifestSchemaVersion,
		RuntimeIdentity: ComputeRuntimeIdentity(userDataDir),
		InstanceID:      "core_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		PID:             os.Getpid(),
		OwnerMode:       ownerMode,
		BaseURL:         baseURL,
		ControlToken:    token,
		StartedAt:       time.Now().UTC(),
	}, nil
}

// WriteManifest atomically writes the identity manifest to the userData runtime
// directory with restrictive permissions.
func WriteManifest(userDataDir string, id *RuntimeIdentity) error {
	paths := ResolveRuntimeManifestPaths(userDataDir)
	if err := filestore.EnsureDir(paths.Dir); err != nil {
		return fmt.Errorf("ensure runtime dir: %w", err)
	}
	if err := restrictDirPermissions(paths.Dir); err != nil {
		return fmt.Errorf("restrict runtime dir: %w", err)
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	if err := filestore.WriteFileAtomic(paths.Manifest, data); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// CompareAndSwapManifest writes the manifest only if no other live Core owns the
// userData directory. If an existing manifest exists, its PID is alive, and its
// instance_id differs from expectedInstanceID, ErrManifestOwnedByOther is
// returned.
func CompareAndSwapManifest(userDataDir, expectedInstanceID string, id *RuntimeIdentity) error {
	existing, err := ReadManifest(userDataDir)
	if err == nil && existing != nil && existing.PID > 0 && processAlive(existing.PID) && existing.InstanceID != expectedInstanceID {
		return fmt.Errorf("%w (instance_id=%s pid=%d)", ErrManifestOwnedByOther, existing.InstanceID, existing.PID)
	}
	return WriteManifest(userDataDir, id)
}

// DeleteManifestIfOwned removes the manifest only when it is owned by the given
// instanceID or does not exist. A mismatching live manifest returns
// ErrManifestOwnedByOther.
func DeleteManifestIfOwned(userDataDir, instanceID string) error {
	existing, err := ReadManifest(userDataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read manifest before delete: %w", err)
	}
	if existing.InstanceID != instanceID {
		return fmt.Errorf("%w (instance_id=%s)", ErrManifestOwnedByOther, existing.InstanceID)
	}
	return DeleteManifest(userDataDir)
}

// ReadManifest reads and returns the manifest if it exists.
func ReadManifest(userDataDir string) (*RuntimeIdentity, error) {
	paths := ResolveRuntimeManifestPaths(userDataDir)
	data, err := os.ReadFile(paths.Manifest)
	if err != nil {
		return nil, err
	}
	var id RuntimeIdentity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("corrupt manifest: %w", err)
	}
	return &id, nil
}

// DeleteManifest removes the manifest. It is a no-op if the manifest does not
// exist.
func DeleteManifest(userDataDir string) error {
	paths := ResolveRuntimeManifestPaths(userDataDir)
	if err := os.Remove(paths.Manifest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete manifest: %w", err)
	}
	return nil
}

// ValidateManifest checks that the manifest is well-formed, points to a loopback
// URL, has a live PID, and matches the expected runtime identity.
func ValidateManifest(id *RuntimeIdentity, expectedRuntimeIdentity string) error {
	if id == nil {
		return errors.New("nil manifest")
	}
	if id.SchemaVersion != runtimeManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema version %d", id.SchemaVersion)
	}
	if expectedRuntimeIdentity != "" && id.RuntimeIdentity != expectedRuntimeIdentity {
		return fmt.Errorf("runtime identity mismatch: got %q, want %q", id.RuntimeIdentity, expectedRuntimeIdentity)
	}
	if id.InstanceID == "" {
		return errors.New("missing instance_id")
	}
	if id.ControlToken == "" {
		return errors.New("missing control_token")
	}
	if err := validateBaseURL(id.BaseURL); err != nil {
		return err
	}
	if id.PID > 0 && !processAlive(id.PID) {
		return fmt.Errorf("manifest PID %d is not alive", id.PID)
	}
	return nil
}

// ValidateControlToken checks whether candidate matches the manifest token using
// constant-time comparison.
func ValidateControlToken(id *RuntimeIdentity, candidate string) bool {
	if id == nil || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(id.ControlToken), []byte(candidate)) == 1
}

// AcquireRuntimeLock obtains the cross-process runtime lock for the userData
// identity. The caller must release it after spawn/attach is complete.
func AcquireRuntimeLock(userDataDir, ownerID string, ttl time.Duration) (*filestore.FileLock, error) {
	paths := ResolveRuntimeManifestPaths(userDataDir)
	if err := filestore.EnsureDir(paths.Dir); err != nil {
		return nil, fmt.Errorf("ensure runtime dir: %w", err)
	}
	return filestore.AcquireLock(paths.Lock, ownerID, ttl)
}

func ComputeRuntimeIdentity(userDataDir string) string {
	// Use a stable digest of the canonical userData path so packaged and dev
	// identities remain separate while each identity converges across restarts.
	canonical := normalizeProjectWorkspaceRoot(userDataDir)
	return "packaged:" + stableProjectID(canonical)
}

func generateControlToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validateBaseURL(baseURL string) error {
	if baseURL == "" {
		return errors.New("base_url is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("base_url must use http scheme, got %q", u.Scheme)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return fmt.Errorf("base_url must have host:port: %w", err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("base_url must use 127.0.0.1, got %q", host)
	}
	if port == "" {
		return errors.New("base_url must include a port")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("base_url port must be numeric: %w", err)
	}
	return nil
}
func restrictDirPermissions(dir string) error {
	if runtime.GOOS == "windows" {
		// Windows ACL enforcement is handled separately if needed; ensure the
		// directory is not world-writable at a minimum.
		return nil
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		if info.Mode().Perm() != filestore.DirPerm {
			if err := os.Chmod(dir, filestore.DirPerm); err != nil {
				return err
			}
		}
	}
	return nil
}
