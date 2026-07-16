package skills

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"awaken/src/tool"
	bashtool "awaken/src/tools/bash"
)

const marketSkillsManifestPath = "testdata/market_skills_manifest.json"

const marketDesktopWorkspaceEnv = "AWAKEN_MARKET_SKILLS_DESKTOP_WORKSPACE"

var (
	marketCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	marketSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	marketSlugPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
)

type marketSkillsManifest struct {
	SchemaVersion    int                 `json:"schema_version"`
	VerificationMode string              `json:"verification_mode"`
	CloneRootEnv     string              `json:"clone_root_env"`
	Samples          []marketSkillSample `json:"samples"`
}

type marketSkillSample struct {
	SourceID         string                 `json:"source_id"`
	Owner            string                 `json:"owner"`
	RepositoryName   string                 `json:"repository_name"`
	RepositoryURL    string                 `json:"repository_url"`
	Commit           string                 `json:"commit"`
	SkillPath        string                 `json:"skill_path"`
	SkillSHA256      string                 `json:"skill_sha256"`
	License          marketSkillLicense     `json:"license"`
	Category         string                 `json:"category"`
	PackageResources marketPackageResources `json:"package_resources"`
	Golden           marketLoadGolden       `json:"golden"`
}

type marketSkillLicense struct {
	SPDX string `json:"spdx"`
	Path string `json:"path"`
}

type marketPackageResources struct {
	Scripts    marketResourcePresence `json:"scripts"`
	Assets     marketResourcePresence `json:"assets"`
	References marketResourcePresence `json:"references"`
}

type marketResourcePresence struct {
	Present bool     `json:"present"`
	Paths   []string `json:"paths"`
}

type marketLoadGolden struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	State               string `json:"state"`
	Context             string `json:"context"`
	UserInvocable       bool   `json:"user_invocable"`
	BodyCached          bool   `json:"body_cached"`
	DescriptionNonempty bool   `json:"description_nonempty"`
}

func TestMarketSkillsManifestContract(t *testing.T) {
	manifest := readMarketSkillsManifest(t)
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", manifest.SchemaVersion)
	}
	if manifest.VerificationMode != "load_only_no_script_execution" {
		t.Fatalf("verification_mode = %q; market acceptance must never execute Skill scripts", manifest.VerificationMode)
	}
	if manifest.CloneRootEnv != "AWAKEN_MARKET_SKILLS_CLONE_ROOT" {
		t.Fatalf("clone_root_env = %q, want AWAKEN_MARKET_SKILLS_CLONE_ROOT", manifest.CloneRootEnv)
	}
	if len(manifest.Samples) != 20 {
		t.Fatalf("market sample count = %d, want exactly 20", len(manifest.Samples))
	}

	sourceIDs := make(map[string]struct{}, len(manifest.Samples))
	owners := make(map[string]struct{}, len(manifest.Samples))
	repositoryURLs := make(map[string]struct{}, len(manifest.Samples))
	coverage := map[string]int{"scripts": 0, "assets": 0, "references": 0}
	for i, sample := range manifest.Samples {
		label := fmt.Sprintf("samples[%d]", i)
		if !marketSlugPattern.MatchString(sample.Owner) {
			t.Errorf("%s owner %q is not a canonical lower-case GitHub slug", label, sample.Owner)
		}
		if !marketSlugPattern.MatchString(sample.RepositoryName) {
			t.Errorf("%s repository_name %q is not a canonical lower-case GitHub slug", label, sample.RepositoryName)
		}
		wantSourceID := sample.Owner + "/" + sample.RepositoryName
		if sample.SourceID != wantSourceID {
			t.Errorf("%s source_id = %q, want %q", label, sample.SourceID, wantSourceID)
		}
		if _, duplicate := sourceIDs[sample.SourceID]; duplicate {
			t.Errorf("%s duplicates source_id %q", label, sample.SourceID)
		}
		sourceIDs[sample.SourceID] = struct{}{}
		if _, duplicate := owners[sample.Owner]; duplicate {
			t.Errorf("%s duplicates owner %q; acceptance sources must have 20 distinct owners", label, sample.Owner)
		}
		owners[sample.Owner] = struct{}{}

		validateMarketRepositoryURL(t, label, sample)
		normalizedURL := normalizeMarketRepositoryURL(sample.RepositoryURL)
		if _, duplicate := repositoryURLs[normalizedURL]; duplicate {
			t.Errorf("%s duplicates repository_url %q", label, sample.RepositoryURL)
		}
		repositoryURLs[normalizedURL] = struct{}{}
		if !marketCommitPattern.MatchString(sample.Commit) {
			t.Errorf("%s commit %q is not a pinned 40-hex object id", label, sample.Commit)
		}
		if !marketSHA256Pattern.MatchString(sample.SkillSHA256) {
			t.Errorf("%s skill_sha256 %q is not 64 lower-case hex characters", label, sample.SkillSHA256)
		}
		if !safeRepositoryPath(sample.SkillPath) || path.Base(sample.SkillPath) != "SKILL.md" {
			t.Errorf("%s skill_path %q is not a safe repository-relative SKILL.md path", label, sample.SkillPath)
		}
		if !marketSlugPattern.MatchString(sample.Category) {
			t.Errorf("%s category %q is not a canonical lower-case slug", label, sample.Category)
		}
		validateMarketLicense(t, label, sample.License)

		resources := []struct {
			kind     string
			presence marketResourcePresence
		}{
			{kind: "scripts", presence: sample.PackageResources.Scripts},
			{kind: "assets", presence: sample.PackageResources.Assets},
			{kind: "references", presence: sample.PackageResources.References},
		}
		for _, resource := range resources {
			if resource.presence.Present {
				coverage[resource.kind]++
			}
			if resource.presence.Present != (len(resource.presence.Paths) > 0) {
				t.Errorf("%s %s presence/path mismatch: present=%v paths=%v", label, resource.kind, resource.presence.Present, resource.presence.Paths)
			}
			for _, resourcePath := range resource.presence.Paths {
				if !safeRepositoryPath(resourcePath) {
					t.Errorf("%s %s path %q is not package-relative", label, resource.kind, resourcePath)
				}
				base := path.Base(resourcePath)
				if resource.kind == "references" {
					if base != "references" && base != "reference" {
						t.Errorf("%s references path %q must name a references or reference directory", label, resourcePath)
					}
				} else if base != resource.kind {
					t.Errorf("%s %s path %q must name a %s directory", label, resource.kind, resourcePath, resource.kind)
				}
			}
		}

		if strings.TrimSpace(sample.Golden.ID) == "" || strings.TrimSpace(sample.Golden.Name) == "" {
			t.Errorf("%s golden id/name must be non-empty", label)
		}
		if sample.Golden.State != string(StateLoaded) {
			t.Errorf("%s golden state = %q, want %q", label, sample.Golden.State, StateLoaded)
		}
		if sample.Golden.Context != string(ContextInline) {
			t.Errorf("%s golden context = %q, want %q", label, sample.Golden.Context, ContextInline)
		}
		if sample.Golden.BodyCached {
			t.Errorf("%s golden body_cached must be false: discovery is metadata-only", label)
		}
		if !sample.Golden.DescriptionNonempty {
			t.Errorf("%s golden description_nonempty must be true", label)
		}
	}
	if len(sourceIDs) != 20 || len(owners) != 20 || len(repositoryURLs) != 20 {
		t.Fatalf("expected 20 distinct source IDs, owners, and repository URLs; got %d/%d/%d", len(sourceIDs), len(owners), len(repositoryURLs))
	}
	if coverage["scripts"] == 0 || coverage["assets"] == 0 {
		t.Fatalf("market samples must cover packages with scripts and assets; coverage=%v", coverage)
	}
	if coverage["references"] == 0 {
		t.Fatalf("market samples must cover reference-heavy packages; coverage=%v", coverage)
	}
}

// TestMarketSkillsPinnedClonesLoadOriginalPackages is intentionally opt-in.
// It executes only Git plumbing and the Awaken metadata loader. It never runs
// any script, hook, command, or executable contained in a sampled Skill.
func TestMarketSkillsPinnedClonesLoadOriginalPackages(t *testing.T) {
	manifest := readMarketSkillsManifest(t)
	cloneRoot := strings.TrimSpace(os.Getenv(manifest.CloneRootEnv))
	if cloneRoot == "" {
		t.Skipf("set %s to a directory of pinned Git clones to run reproducible market package acceptance", manifest.CloneRootEnv)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required for pinned market package acceptance: %v", err)
	}
	repositories := discoverMarketRepositories(t, cloneRoot)
	if len(repositories) < len(manifest.Samples) {
		t.Fatalf("found %d Git repositories in %s, need at least %d", len(repositories), cloneRoot, len(manifest.Samples))
	}

	for _, sample := range manifest.Samples {
		sample := sample
		t.Run(sample.SourceID, func(t *testing.T) {
			repositoryDir := repositories[normalizeMarketRepositoryURL(sample.RepositoryURL)]
			if repositoryDir == "" {
				t.Fatalf("no clone with origin %s under %s", sample.RepositoryURL, cloneRoot)
			}
			head := strings.TrimSpace(runMarketGit(t, repositoryDir, "rev-parse", "HEAD"))
			if head != sample.Commit {
				t.Fatalf("clone HEAD = %s, manifest pins %s", head, sample.Commit)
			}
			if sample.License.Path != "" {
				licenseBlob := runMarketGitBytes(t, repositoryDir, "cat-file", "blob", sample.Commit+":"+sample.License.Path)
				validateMarketLicenseBlob(t, sample.License, licenseBlob)
			}
			skillBlob := runMarketGitBytes(t, repositoryDir, "cat-file", "blob", sample.Commit+":"+sample.SkillPath)
			digest := sha256.Sum256(skillBlob)
			if got := hex.EncodeToString(digest[:]); got != sample.SkillSHA256 {
				t.Fatalf("SKILL.md SHA-256 = %s, manifest pins %s", got, sample.SkillSHA256)
			}

			exportRoot := t.TempDir()
			packageRoot := path.Dir(sample.SkillPath)
			archive := runMarketGitBytes(t, repositoryDir, "archive", "--format=tar", "--prefix="+sample.Golden.ID+"/", sample.Commit+":"+packageRoot)
			extractMarketArchive(t, exportRoot, archive)
			validateExportedMarketResources(t, exportRoot, sample)

			loaded := LoadDir(exportRoot, SourceUser)
			if len(loaded) != 1 {
				t.Fatalf("loaded %d definitions from exported package, want exactly 1: %#v", len(loaded), loaded)
			}
			definition := loaded[sample.Golden.ID]
			if definition == nil {
				t.Fatalf("loaded definitions do not contain golden id %q: %#v", sample.Golden.ID, loaded)
			}
			assertMarketGoldenDefinition(t, definition, sample.Golden)

			packageSource := filepath.Join(exportRoot, sample.Golden.ID)
			pkg, err := PackageFromPath(packageSource)
			if err != nil {
				t.Fatalf("create formal install package from exported upstream Skill: %v", err)
			}
			workspace := t.TempDir()
			fakeHome := filepath.Join(workspace, "home-must-remain-clean")
			if err := os.MkdirAll(fakeHome, 0o755); err != nil {
				t.Fatalf("create isolated home sentinel: %v", err)
			}
			t.Setenv("HOME", fakeHome)
			t.Setenv("USERPROFILE", fakeHome)
			installResult, err := InstallSkillPackage(InstallRequest{
				Package:         pkg,
				Scope:           string(SourceProject),
				ProjectRoot:     workspace,
				OverwritePolicy: "abort",
			})
			if err != nil {
				t.Fatalf("install exported upstream Skill through formal project installer: %v", err)
			}
			projectSkillsRoot := filepath.Join(workspace, ".awaken", "skills")
			wantInstallPath := filepath.Join(projectSkillsRoot, sample.Golden.ID)
			if installResult.Scope != string(SourceProject) || installResult.Path != wantInstallPath {
				t.Errorf("formal install result scope/path = %q/%q, want %q/%q", installResult.Scope, installResult.Path, SourceProject, wantInstallPath)
			}
			if installResult.Skill == nil {
				t.Fatal("formal installer returned nil Skill definition")
			} else {
				if installResult.Skill.ID != sample.Golden.ID || installResult.Skill.Name != sample.Golden.Name || string(installResult.Skill.State) != sample.Golden.State {
					t.Errorf("formal installer returned id/name/state = %q/%q/%q, want %q/%q/%q", installResult.Skill.ID, installResult.Skill.Name, installResult.Skill.State, sample.Golden.ID, sample.Golden.Name, sample.Golden.State)
				}
				if installResult.Skill.Source != SourceProject || installResult.Skill.ProjectPath != workspace {
					t.Errorf("installed source/project path = %q/%q, want %q/%q", installResult.Skill.Source, installResult.Skill.ProjectPath, SourceProject, workspace)
				}
			}
			if _, err := os.Stat(filepath.Join(fakeHome, ".awaken")); !os.IsNotExist(err) {
				t.Errorf("project install touched isolated user home: %v", err)
			}
			installedSkillBlob, err := os.ReadFile(filepath.Join(wantInstallPath, "SKILL.md"))
			if err != nil {
				t.Fatalf("read formally installed SKILL.md: %v", err)
			}
			installedDigest := sha256.Sum256(installedSkillBlob)
			if got := hex.EncodeToString(installedDigest[:]); got != sample.SkillSHA256 {
				t.Errorf("installed SKILL.md SHA-256 = %s, want pinned upstream %s", got, sample.SkillSHA256)
			}
			validateExportedMarketResources(t, projectSkillsRoot, sample)

			installed := LoadDir(projectSkillsRoot, SourceProject)
			if len(installed) != 1 {
				t.Fatalf("project discovery loaded %d definitions after formal install, want exactly 1: %#v", len(installed), installed)
			}
			installedDefinition := installed[sample.Golden.ID]
			if installedDefinition == nil {
				t.Fatalf("project discovery does not contain installed golden id %q: %#v", sample.Golden.ID, installed)
			}
			assertMarketGoldenDefinition(t, installedDefinition, sample.Golden)
			if installedDefinition.Source != SourceProject || installedDefinition.ProjectPath != workspace {
				t.Errorf("discovered installed source/project path = %q/%q, want %q/%q", installedDefinition.Source, installedDefinition.ProjectPath, SourceProject, workspace)
			}
		})
	}
}

// TestMarketSkillsInstallDesktopWorkspace is an opt-in bridge between the
// reproducible market fixture and real Desktop acceptance. Unlike the
// per-sample test above, it intentionally leaves one caller-selected workspace
// intact so Awaken Desktop can open it and inspect all 20 project Skills in a
// single live catalog. The target must be an empty absolute workspace and the
// test never writes the user's global Skill directory.
func TestMarketSkillsInstallDesktopWorkspace(t *testing.T) {
	manifest := readMarketSkillsManifest(t)
	cloneRoot := strings.TrimSpace(os.Getenv(manifest.CloneRootEnv))
	if cloneRoot == "" {
		t.Skipf("set %s to the pinned Git clone root", manifest.CloneRootEnv)
	}
	workspace := strings.TrimSpace(os.Getenv(marketDesktopWorkspaceEnv))
	if workspace == "" {
		t.Skipf("set %s to an empty absolute workspace reserved for real Desktop acceptance", marketDesktopWorkspaceEnv)
	}
	if !filepath.IsAbs(workspace) {
		t.Fatalf("%s must be absolute: %q", marketDesktopWorkspaceEnv, workspace)
	}
	workspace = filepath.Clean(workspace)
	if workspace == string(filepath.Separator) {
		t.Fatalf("refusing to use the filesystem root as a Desktop acceptance workspace")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create Desktop acceptance workspace: %v", err)
	}
	projectSkillsRoot := filepath.Join(workspace, ".awaken", "skills")
	if entries, err := os.ReadDir(projectSkillsRoot); err == nil && len(entries) > 0 {
		t.Fatalf("Desktop acceptance Skill root must start empty: %s contains %d entries", projectSkillsRoot, len(entries))
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("inspect Desktop acceptance Skill root: %v", err)
	}

	repositories := discoverMarketRepositories(t, cloneRoot)
	for _, sample := range manifest.Samples {
		sample := sample
		t.Run(sample.SourceID, func(t *testing.T) {
			repositoryDir := repositories[normalizeMarketRepositoryURL(sample.RepositoryURL)]
			if repositoryDir == "" {
				t.Fatalf("no clone with origin %s under %s", sample.RepositoryURL, cloneRoot)
			}
			if head := strings.TrimSpace(runMarketGit(t, repositoryDir, "rev-parse", "HEAD")); head != sample.Commit {
				t.Fatalf("clone HEAD = %s, manifest pins %s", head, sample.Commit)
			}
			exportRoot := t.TempDir()
			packageRoot := path.Dir(sample.SkillPath)
			archive := runMarketGitBytes(t, repositoryDir, "archive", "--format=tar", "--prefix="+sample.Golden.ID+"/", sample.Commit+":"+packageRoot)
			extractMarketArchive(t, exportRoot, archive)
			pkg, err := PackageFromPath(filepath.Join(exportRoot, sample.Golden.ID))
			if err != nil {
				t.Fatalf("package pinned upstream Skill: %v", err)
			}
			installed, err := InstallSkillPackage(InstallRequest{
				Package: pkg, Scope: string(SourceProject), ProjectRoot: workspace, OverwritePolicy: "abort",
			})
			if err != nil {
				t.Fatalf("formal project install: %v", err)
			}
			if installed.Skill == nil || installed.Skill.ID != sample.Golden.ID || installed.Skill.State != StateLoaded {
				t.Fatalf("unexpected installed Skill: %+v", installed)
			}
		})
	}

	loaded := LoadDir(projectSkillsRoot, SourceProject)
	if len(loaded) != len(manifest.Samples) {
		t.Fatalf("persistent Desktop workspace loaded %d Skills, want %d", len(loaded), len(manifest.Samples))
	}
	for _, sample := range manifest.Samples {
		definition := loaded[sample.Golden.ID]
		if definition == nil {
			t.Errorf("persistent Desktop workspace is missing %q", sample.Golden.ID)
			continue
		}
		assertMarketGoldenDefinition(t, definition, sample.Golden)
	}
	t.Logf("installed %d pinned market Skills into Desktop acceptance workspace %s", len(loaded), workspace)
}

func TestPinnedMarketSkillScriptExecutesThroughSupervisedShell(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the production workspace mutation broker currently has a real process boundary only on macOS")
	}
	manifest := readMarketSkillsManifest(t)
	cloneRoot := strings.TrimSpace(os.Getenv(manifest.CloneRootEnv))
	if cloneRoot == "" {
		t.Skipf("set %s to run the pinned market Skill script acceptance", manifest.CloneRootEnv)
	}
	const sourceID = "nextlevelbuilder/ui-ux-pro-max-skill"
	var sample *marketSkillSample
	for index := range manifest.Samples {
		if manifest.Samples[index].SourceID == sourceID {
			sample = &manifest.Samples[index]
			break
		}
	}
	if sample == nil {
		t.Fatalf("market manifest does not contain %s", sourceID)
	}
	if !sample.PackageResources.Scripts.Present {
		t.Fatalf("selected market fixture no longer declares scripts: %+v", sample.PackageResources)
	}

	repositories := discoverMarketRepositories(t, cloneRoot)
	repositoryDir := repositories[normalizeMarketRepositoryURL(sample.RepositoryURL)]
	if repositoryDir == "" {
		t.Fatalf("no clone with origin %s under %s", sample.RepositoryURL, cloneRoot)
	}
	if head := strings.TrimSpace(runMarketGit(t, repositoryDir, "rev-parse", "HEAD")); head != sample.Commit {
		t.Fatalf("clone HEAD = %s, manifest pins %s", head, sample.Commit)
	}
	exportRoot := t.TempDir()
	packageRoot := path.Dir(sample.SkillPath)
	archive := runMarketGitBytes(t, repositoryDir, "archive", "--format=tar", "--prefix="+sample.Golden.ID+"/", sample.Commit+":"+packageRoot)
	extractMarketArchive(t, exportRoot, archive)
	pkg, err := PackageFromPath(filepath.Join(exportRoot, sample.Golden.ID))
	if err != nil {
		t.Fatalf("package pinned market Skill: %v", err)
	}
	workspace := t.TempDir()
	installed, err := InstallSkillPackage(InstallRequest{
		Package: pkg, Scope: string(SourceProject), ProjectRoot: workspace, OverwritePolicy: "abort",
	})
	if err != nil {
		t.Fatalf("install pinned market Skill: %v", err)
	}
	if installed.Skill == nil || installed.Skill.ID != sample.Golden.ID {
		t.Fatalf("unexpected installed Skill: %+v", installed)
	}

	command := `python3 .awaken/skills/ui-ux-pro-max/scripts/search.py dashboard --domain product --max-results 1 --json`
	input, err := json.Marshal(map[string]any{"command": command, "timeout": 30_000})
	if err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	outcome, err := (&bashtool.Tool{}).ExecuteOutcome(ctx, string(input), nil)
	if err != nil {
		t.Fatalf("market Skill script transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.ExitCode == nil || *outcome.ExitCode != 0 {
		t.Fatalf("market Skill script did not execute successfully through Bash: %+v", outcome)
	}
	stdout, _ := outcome.Details["stdout"].(string)
	if !strings.Contains(stdout, `"domain": "product"`) || !strings.Contains(stdout, `"count":`) {
		t.Fatalf("unexpected pinned market Skill script output: %q", stdout)
	}
	if snapshotID, _ := outcome.Details["environment_snapshot_id"].(string); strings.TrimSpace(snapshotID) == "" {
		t.Fatalf("script outcome omitted the user-machine environment snapshot: %+v", outcome.Details)
	}
}

func assertMarketGoldenDefinition(t *testing.T, definition *SkillDefinition, golden marketLoadGolden) {
	t.Helper()
	if definition.ID != golden.ID || definition.Name != golden.Name {
		t.Errorf("loaded id/name = %q/%q, want %q/%q", definition.ID, definition.Name, golden.ID, golden.Name)
	}
	if string(definition.State) != golden.State {
		t.Errorf("loaded state = %q, want %q; reason=%s diagnostics=%v", definition.State, golden.State, definition.UnavailableReason, definition.Diagnostics)
	}
	if string(definition.Context) != golden.Context {
		t.Errorf("loaded context = %q, want %q", definition.Context, golden.Context)
	}
	if definition.UserInvocable != golden.UserInvocable {
		t.Errorf("loaded user_invocable = %v, want %v", definition.UserInvocable, golden.UserInvocable)
	}
	if (definition.Body != "") != golden.BodyCached {
		t.Errorf("loaded body_cached = %v, want %v", definition.Body != "", golden.BodyCached)
	}
	if (strings.TrimSpace(definition.Description) != "") != golden.DescriptionNonempty {
		t.Errorf("loaded description_nonempty = %v, want %v", strings.TrimSpace(definition.Description) != "", golden.DescriptionNonempty)
	}
}

func readMarketSkillsManifest(t *testing.T) marketSkillsManifest {
	t.Helper()
	raw, err := os.ReadFile(marketSkillsManifestPath)
	if err != nil {
		t.Fatalf("read market Skills manifest: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest marketSkillsManifest
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode market Skills manifest: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("market Skills manifest has trailing JSON content: %v", err)
	}
	return manifest
}

func validateMarketRepositoryURL(t *testing.T, label string, sample marketSkillSample) {
	t.Helper()
	parsed, err := url.Parse(sample.RepositoryURL)
	if err != nil {
		t.Errorf("%s repository_url %q is invalid: %v", label, sample.RepositoryURL, err)
		return
	}
	wantPath := "/" + sample.SourceID + ".git"
	if parsed.Scheme != "https" || strings.ToLower(parsed.Host) != "github.com" || parsed.Path != wantPath || parsed.RawQuery != "" || parsed.Fragment != "" {
		t.Errorf("%s repository_url = %q, want canonical https://github.com%s", label, sample.RepositoryURL, wantPath)
	}
}

func validateMarketLicense(t *testing.T, label string, license marketSkillLicense) {
	t.Helper()
	switch license.SPDX {
	case "Apache-2.0", "MIT":
		if !safeRepositoryPath(license.Path) {
			t.Errorf("%s license path %q is not repository-relative", label, license.Path)
		}
	case "NOASSERTION":
		if license.Path != "" && !safeRepositoryPath(license.Path) {
			t.Errorf("%s NOASSERTION license path %q is not repository-relative", label, license.Path)
		}
	default:
		t.Errorf("%s license SPDX value %q is not an accepted manifest value", label, license.SPDX)
	}
}

func safeRepositoryPath(value string) bool {
	return value != "" && value == path.Clean(value) && value != "." && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../")
}

func normalizeMarketRepositoryURL(value string) string {
	normalized := strings.TrimSpace(value)
	if strings.HasPrefix(normalized, "git@github.com:") {
		normalized = "https://github.com/" + strings.TrimPrefix(normalized, "git@github.com:")
	}
	normalized = strings.TrimSuffix(normalized, "/")
	normalized = strings.TrimSuffix(normalized, ".git")
	return strings.ToLower(normalized)
}

func discoverMarketRepositories(t *testing.T, cloneRoot string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(cloneRoot)
	if err != nil {
		t.Fatalf("read clone root %s: %v", cloneRoot, err)
	}
	repositories := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repositoryDir := filepath.Join(cloneRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(repositoryDir, ".git")); err != nil {
			continue
		}
		command := exec.Command("git", "-C", repositoryDir, "remote", "get-url", "origin")
		output, err := command.Output()
		if err != nil {
			continue
		}
		key := normalizeMarketRepositoryURL(string(output))
		if key != "" {
			repositories[key] = repositoryDir
		}
	}
	return repositories
}

func runMarketGit(t *testing.T, repositoryDir string, args ...string) string {
	t.Helper()
	return string(runMarketGitBytes(t, repositoryDir, args...))
}

func runMarketGitBytes(t *testing.T, repositoryDir string, args ...string) []byte {
	t.Helper()
	commandArgs := append([]string{"-C", repositoryDir}, args...)
	command := exec.Command("git", commandArgs...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return output
}

func extractMarketArchive(t *testing.T, destination string, archive []byte) {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("read Git archive: %v", err)
		}
		name := path.Clean(header.Name)
		if !safeRepositoryPath(name) {
			t.Fatalf("Git archive contains unsafe path %q", header.Name)
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		relative, err := filepath.Rel(destination, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("Git archive path %q escapes extraction root", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("create archive directory %s: %v", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create archive parent %s: %v", filepath.Dir(target), err)
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0o777)
			if err != nil {
				t.Fatalf("create archive file %s: %v", target, err)
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				t.Fatalf("extract archive file %s: %v", target, copyErr)
			}
			if closeErr != nil {
				t.Fatalf("close archive file %s: %v", target, closeErr)
			}
		default:
			t.Fatalf("Git archive contains unsupported entry %q with type %d", header.Name, header.Typeflag)
		}
	}
}

func validateExportedMarketResources(t *testing.T, exportRoot string, sample marketSkillSample) {
	t.Helper()
	packageRoot := filepath.Join(exportRoot, sample.Golden.ID)
	actual := map[string][]string{"scripts": nil, "assets": nil, "references": nil}
	if err := filepath.WalkDir(packageRoot, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || current == packageRoot {
			return nil
		}
		relative, err := filepath.Rel(packageRoot, current)
		if err != nil {
			return err
		}
		resourcePath := filepath.ToSlash(relative)
		switch entry.Name() {
		case "scripts":
			actual["scripts"] = append(actual["scripts"], resourcePath)
		case "assets":
			actual["assets"] = append(actual["assets"], resourcePath)
		case "reference", "references":
			actual["references"] = append(actual["references"], resourcePath)
		}
		return nil
	}); err != nil {
		t.Fatalf("inspect exported package resources: %v", err)
	}
	resources := []struct {
		kind     string
		presence marketResourcePresence
	}{
		{kind: "scripts", presence: sample.PackageResources.Scripts},
		{kind: "assets", presence: sample.PackageResources.Assets},
		{kind: "references", presence: sample.PackageResources.References},
	}
	for _, resource := range resources {
		want := append([]string(nil), resource.presence.Paths...)
		got := actual[resource.kind]
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("exported %s directories = %v, manifest records %v", resource.kind, got, want)
		}
	}
}

func validateMarketLicenseBlob(t *testing.T, license marketSkillLicense, blob []byte) {
	t.Helper()
	text := string(blob)
	switch license.SPDX {
	case "Apache-2.0":
		if !strings.Contains(text, "Apache License") || !strings.Contains(text, "Version 2.0") {
			t.Errorf("license at %s does not contain the Apache License 2.0 markers", license.Path)
		}
	case "MIT":
		if !strings.Contains(text, "MIT License") || !strings.Contains(text, "Permission is hereby granted") {
			t.Errorf("license at %s does not contain the MIT License markers", license.Path)
		}
	case "NOASSERTION":
		// A path, when supplied, is provenance only; no license identity is asserted.
	}
}
