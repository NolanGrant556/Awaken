package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"awaken/src/desktopdata"
)

func mustTempDirs(t *testing.T, count int) []string {
	t.Helper()
	roots := make([]string, 0, count)
	for index := 0; index < count; index++ {
		roots = append(roots, t.TempDir())
	}
	return roots
}

func TestCreateProjectKeepsFolderOrderAndHonoursRequestedPrimary(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 3)
	projects := newDurableThreadTestService().ProjectService()

	project, err := projects.CreateProject([]ProjectFolder{
		{Path: roots[0]},
		{Path: roots[1], IsPrimary: true},
		{Path: roots[2]},
	}, "multi")
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 3 {
		t.Fatalf("expected three attached folders, got %+v", project.Folders)
	}
	if project.PrimaryRoot() != normalizeProjectWorkspaceRoot(roots[1]) {
		t.Fatalf("requested primary was not honoured: %+v", project.Folders)
	}
	if project.WorkspaceRoot != project.PrimaryRoot() {
		t.Fatalf("WorkspaceRoot must mirror the primary folder, got %q", project.WorkspaceRoot)
	}
	secondaries := project.SecondaryRoots()
	if len(secondaries) != 2 ||
		secondaries[0] != normalizeProjectWorkspaceRoot(roots[0]) ||
		secondaries[1] != normalizeProjectWorkspaceRoot(roots[2]) {
		t.Fatalf("secondary order must follow attach order, got %+v", secondaries)
	}
	if project.ProjectID == stableProjectID(roots[1]) {
		t.Fatal("a multi-folder project must not take a path-derived identity")
	}
	for _, root := range project.AllRoots() {
		if ok, err := projects.CanStartRun(root); err != nil || !ok {
			t.Fatalf("every attached root must be runnable, root=%s ok=%v err=%v", root, ok, err)
		}
	}
}

// A project is a named piece of work, so it exists before it has a folder and
// no placeholder folder is invented on its behalf.
func TestCreateProjectWithoutFoldersStaysFolderless(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	projects := newDurableThreadTestService().ProjectService()

	project, err := projects.CreateProject(nil, "folderless")
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 0 {
		t.Fatalf("a folderless project must not be given a folder, got %+v", project.Folders)
	}
	if _, ok := project.PrimaryFolder(); ok {
		t.Fatalf("a folderless project has no primary folder, got %+v", project.Folders)
	}
	if project.DisplayName != "folderless" {
		t.Fatalf("display name must survive without a folder, got %q", project.DisplayName)
	}
	if project.State != ProjectStateActive {
		t.Fatalf("a folderless project must be active, got %q", project.State)
	}
}

func TestAttachFirstFolderBecomesPrimaryOfAFolderlessProject(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	userRoot := t.TempDir()
	projects := newDurableThreadTestService().ProjectService()

	project, err := projects.CreateProject(nil, "grows")
	if err != nil {
		t.Fatal(err)
	}

	project, err = projects.AttachProjectFolder(project.ProjectID, userRoot)
	if err != nil {
		t.Fatal(err)
	}
	if project.PrimaryRoot() != normalizeProjectWorkspaceRoot(userRoot) {
		t.Fatalf("the first attached folder must become primary, got %+v", project.Folders)
	}
	if len(project.Folders) != 1 {
		t.Fatalf("attaching one folder must not leave anything else behind, got %+v", project.Folders)
	}
}

// Detaching the last folder returns the project to the folder-less state it
// could have been created in, rather than deleting it or substituting a folder.
func TestDetachingTheLastFolderLeavesTheProjectFolderless(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	userRoot := t.TempDir()
	projects := newDurableThreadTestService().ProjectService()

	marker := filepath.Join(userRoot, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	project, err := projects.CreateProject([]ProjectFolder{{Path: userRoot, IsPrimary: true}}, "shrinks")
	if err != nil {
		t.Fatal(err)
	}
	project, err = projects.DetachProjectFolder(project.ProjectID, userRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 0 {
		t.Fatalf("the project must end up folderless, got %+v", project.Folders)
	}
	if payload, err := os.ReadFile(marker); err != nil || string(payload) != "keep" {
		t.Fatalf("detach must not touch the directory on disk: %q err=%v", payload, err)
	}
	if project.State != ProjectStateActive {
		t.Fatalf("detaching the last folder must not deactivate the project, got %q", project.State)
	}
	if project.DisplayName != "shrinks" {
		t.Fatalf("the project must keep its name, got %q", project.DisplayName)
	}
}

// A directory is shared work, not property: which projects a folder belongs to
// is the user's call, so neither creation nor attach may refuse a folder some
// other project already holds.
func TestAFolderCanBelongToSeveralProjects(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 2)
	projects := newDurableThreadTestService().ProjectService()

	first, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}, {Path: roots[1]}}, "second")
	if err != nil {
		t.Fatalf("creation must accept a folder another project holds, got %v", err)
	}
	if !second.HasFolder(roots[0]) || second.PrimaryRoot() != normalizeProjectWorkspaceRoot(roots[0]) {
		t.Fatalf("the shared folder must join the new project as its primary, got %+v", second.Folders)
	}

	third, err := projects.CreateProject(nil, "third")
	if err != nil {
		t.Fatal(err)
	}
	third, err = projects.AttachProjectFolder(third.ProjectID, roots[0])
	if err != nil {
		t.Fatalf("attach must accept a folder another project holds, got %v", err)
	}
	if !third.HasFolder(roots[0]) {
		t.Fatalf("the shared folder must attach, got %+v", third.Folders)
	}

	// Sharing must not disturb the project that held the folder first.
	reloaded, err := projects.store.ProjectByID(first.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.HasFolder(roots[0]) || reloaded.State != ProjectStateActive {
		t.Fatalf("the first holder must keep the folder, got %+v", reloaded)
	}
	if ok, err := projects.CanStartRun(roots[0]); err != nil || !ok {
		t.Fatalf("a shared folder must stay runnable, ok=%v err=%v", ok, err)
	}
}

// Removing a project releases nothing to reclaim, because there was never an
// exclusive claim; the folder simply has one less holder.
func TestARemovedHolderDoesNotBlockTheFoldersOtherHolders(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 1)
	projects := newDurableThreadTestService().ProjectService()

	removed, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "removed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.DetachProject(removed.ProjectID, "key-remove"); err != nil {
		t.Fatal(err)
	}
	reused, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "reused")
	if err != nil {
		t.Fatalf("a folder held only by a removed project must be reusable, got %v", err)
	}
	if !reused.HasFolder(roots[0]) {
		t.Fatalf("the folder must join the new project, got %+v", reused.Folders)
	}
	if ok, err := projects.CanStartRun(roots[0]); err != nil || !ok {
		t.Fatalf("an active holder must make the folder runnable, ok=%v err=%v", ok, err)
	}
}

func TestSetPrimaryMovesTheAnchorBetweenAttachedFolders(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 2)
	projects := newDurableThreadTestService().ProjectService()

	project, err := projects.CreateProject([]ProjectFolder{
		{Path: roots[0], IsPrimary: true},
		{Path: roots[1]},
	}, "switch")
	if err != nil {
		t.Fatal(err)
	}
	project, err = projects.SetPrimaryProjectFolder(project.ProjectID, roots[1])
	if err != nil {
		t.Fatal(err)
	}
	if project.PrimaryRoot() != normalizeProjectWorkspaceRoot(roots[1]) {
		t.Fatalf("primary switch did not take effect, got %+v", project.Folders)
	}
	if got := project.SecondaryRoots(); len(got) != 1 || got[0] != normalizeProjectWorkspaceRoot(roots[0]) {
		t.Fatalf("previous primary must become a secondary, got %+v", got)
	}
	if _, err := projects.SetPrimaryProjectFolder(project.ProjectID, t.TempDir()); !errors.Is(err, ErrProjectFolderNotFound) {
		t.Fatalf("an unattached folder cannot become primary, got %v", err)
	}
}

func TestOnlyAttachedFoldersCanBeTrashed(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	projects := newDurableThreadTestService().ProjectService()

	project, err := projects.CreateProject(nil, "folderless")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.PrepareTrash(project.ProjectID, t.TempDir(), "trash-unattached"); !errors.Is(err, ErrProjectFolderNotFound) {
		t.Fatalf("only attached folders can be trashed, got %v", err)
	}
}

func TestTrashingASecondaryFolderKeepsPrimaryAndProject(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	parent := t.TempDir()
	primaryRoot := filepath.Join(parent, "primary")
	secondaryRoot := filepath.Join(parent, "secondary")
	for _, root := range []string{primaryRoot, secondaryRoot} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	service := newDurableThreadTestService()
	projects := service.ProjectService()
	project, err := projects.CreateProject([]ProjectFolder{
		{Path: primaryRoot, IsPrimary: true},
		{Path: secondaryRoot},
	}, "mixed")
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := projects.PrepareTrash(project.ProjectID, secondaryRoot, "trash-secondary")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := projects.CanStartRun(primaryRoot); ok {
		t.Fatal("a pending folder trash must fence the whole project")
	}
	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "test-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(staged.TrashStagingPath) != "secondary" {
		t.Fatalf("staging must target the requested folder, got %q", staged.TrashStagingPath)
	}
	if _, err := os.Stat(primaryRoot); err != nil {
		t.Fatalf("the primary folder must be untouched: %v", err)
	}
	if _, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}

	final, err := projects.store.ProjectByID(project.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != ProjectStateActive || final.PrimaryRoot() != normalizeProjectWorkspaceRoot(primaryRoot) {
		t.Fatalf("trashing a secondary must not move the anchor, got %+v", final)
	}
	if final.HasFolder(secondaryRoot) {
		t.Fatalf("trashed folder must be detached, got %+v", final.Folders)
	}
	if ok, err := projects.CanStartRun(primaryRoot); err != nil || !ok {
		t.Fatalf("the project must be runnable again, ok=%v err=%v", ok, err)
	}
}

// Trashing a shared folder deletes the one directory both projects were using,
// so it leaves neither of them pointing at a path that no longer exists, and it
// blocks work in that folder for both while the move is in flight.
func TestTrashingASharedFolderDetachesItFromEveryHolder(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	parent := t.TempDir()
	sharedRoot := filepath.Join(parent, "shared")
	ownRoot := filepath.Join(parent, "own")
	for _, root := range []string{sharedRoot, ownRoot} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projects := newDurableThreadTestService().ProjectService()
	trashing, err := projects.CreateProject([]ProjectFolder{{Path: sharedRoot, IsPrimary: true}}, "trashing")
	if err != nil {
		t.Fatal(err)
	}
	bystander, err := projects.CreateProject([]ProjectFolder{
		{Path: ownRoot, IsPrimary: true},
		{Path: sharedRoot},
	}, "bystander")
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := projects.PrepareTrash(trashing.ProjectID, sharedRoot, "trash-shared")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := projects.CanStartRun(sharedRoot); ok {
		t.Fatal("a folder on its way to the trash must be fenced for every holder")
	}
	// The bystander's own folder is not part of this trash and stays usable.
	if ok, err := projects.CanStartRun(ownRoot); err != nil || !ok {
		t.Fatalf("an unrelated folder must stay runnable, ok=%v err=%v", ok, err)
	}

	claimed, err := projects.ClaimTrashOperation(prepared.OperationID, "test-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := projects.StageTrashOperation(prepared.OperationID, claimed.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.BeginTrashInvocation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(staged.TrashStagingPath); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CompleteTrashOperation(prepared.OperationID, claimed.AttemptID); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []struct {
		projectID string
		label     string
	}{{trashing.ProjectID, "the project that asked"}, {bystander.ProjectID, "the other holder"}} {
		reloaded, err := projects.store.ProjectByID(expected.projectID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.HasFolder(sharedRoot) {
			t.Fatalf("the trashed folder must leave %s, got %+v", expected.label, reloaded.Folders)
		}
		if reloaded.State != ProjectStateActive {
			t.Fatalf("%s must stay active, got %q", expected.label, reloaded.State)
		}
	}
	final, err := projects.store.ProjectByID(bystander.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if final.PrimaryRoot() != normalizeProjectWorkspaceRoot(ownRoot) {
		t.Fatalf("the other holder must keep its own primary, got %+v", final.Folders)
	}
	if ok, err := projects.CanStartRun(ownRoot); err != nil || !ok {
		t.Fatalf("the other holder must be runnable again, ok=%v err=%v", ok, err)
	}
}

// Removing a project from Awaken never touches the disk, so a folder it shared
// with a live project must stay open for that project.
func TestRemovingOneHolderLeavesTheSharedFolderUsable(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 1)
	projects := newDurableThreadTestService().ProjectService()

	leaving, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "leaving")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "staying"); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.DetachProject(leaving.ProjectID, "remove-one-holder"); err != nil {
		t.Fatal(err)
	}
	if ok, err := projects.CanStartRun(roots[0]); err != nil || !ok {
		t.Fatalf("the remaining holder must keep the folder usable, ok=%v err=%v", ok, err)
	}
}

// Detaching a shared folder from one project is a change to that project alone.
func TestDetachingASharedFolderLeavesTheOtherHolderUntouched(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 1)
	projects := newDurableThreadTestService().ProjectService()

	first, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.DetachProjectFolder(first.ProjectID, roots[0]); err != nil {
		t.Fatal(err)
	}
	reloaded, err := projects.store.ProjectByID(second.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.HasFolder(roots[0]) {
		t.Fatalf("the other holder must keep the folder, got %+v", reloaded.Folders)
	}
	if ok, err := projects.CanStartRun(roots[0]); err != nil || !ok {
		t.Fatalf("the folder must stay runnable for the remaining holder, ok=%v err=%v", ok, err)
	}
}

// A folder one project let go of must be usable by the next project that picks
// it up. The deletion fence is raised per project, so reopening the folder has
// to clear it whoever raised it.
func TestAFolderDetachedByOneProjectIsUsableAfterAnotherAttachesIt(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 2)
	service := newDurableThreadTestService()
	// StartRun is asynchronous. Let its journal writer exit before TempDir cleanup.
	t.Cleanup(service.drainRunWorkers)
	projects := service.ProjectService()

	first, err := projects.CreateProject([]ProjectFolder{{Path: roots[0]}}, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.DetachProjectFolder(first.ProjectID, roots[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartRun(StartRunRequest{
		Input:     "after detach",
		Workspace: WorkspaceConfig{WorkspaceRoot: roots[0]},
	}); !errors.Is(err, ErrProjectOperationConflict) {
		t.Fatalf("a folder no project holds must stay fenced, got %v", err)
	}

	second, err := projects.CreateProject([]ProjectFolder{{Path: roots[1]}}, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.AttachProjectFolder(second.ProjectID, roots[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartRun(StartRunRequest{
		Input:     "after attach",
		Workspace: WorkspaceConfig{WorkspaceRoot: roots[0]},
	}); err != nil {
		t.Fatalf("attaching the folder must reopen it: %v", err)
	}
}

func TestTaskScopeProtectsMetadataDirectoriesOfEverySecondaryRoot(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 3)

	scope, err := normalizeWorkspace(WorkspaceConfig{
		WorkspaceRoot: roots[0],
		ReadableRoots: roots,
		WritableRoots: roots,
		OutputRoots:   roots,
	})
	if err != nil {
		t.Fatal(err)
	}
	protected := make(map[string]struct{}, len(scope.ProtectedRoots))
	for _, root := range scope.ProtectedRoots {
		protected[root] = struct{}{}
	}
	for _, root := range roots {
		for _, name := range []string{".git", ".awaken", ".vscode", ".idea"} {
			expected := filepath.Join(normalizeProjectWorkspaceRoot(root), name)
			if _, ok := protected[expected]; !ok {
				t.Fatalf("every root must contribute its own protections, missing %q", expected)
			}
		}
	}
	wantPluginRoot := filepath.Clean(desktopdata.PluginsRoot(os.Getenv(awakenHomeEnv)))
	if len(scope.ProtectedRootExceptions) != 1 || scope.ProtectedRootExceptions[0] != wantPluginRoot {
		t.Fatalf("every run must receive exactly the Plugin repository carve-out, got %+v", scope.ProtectedRootExceptions)
	}
}

// The project display name is model-facing context, so it has to survive
// normalization untouched while never influencing a resolved path.
func TestTaskScopeCarriesTheProjectNameForTheModel(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	roots := mustTempDirs(t, 2)

	scope, err := normalizeWorkspace(WorkspaceConfig{
		ProjectID:     "project_named",
		ProjectName:   "  测试项目  ",
		WorkspaceRoot: roots[0],
		ReadableRoots: roots,
	})
	if err != nil {
		t.Fatal(err)
	}
	if scope.ProjectName != "测试项目" {
		t.Fatalf("project name=%q want %q", scope.ProjectName, "测试项目")
	}
	if scope.SessionWorkspace {
		t.Fatal("a scope anchored to a user folder is not a session workspace")
	}
}

// SessionWorkspace is derived, so a client claiming it never gets to keep the
// claim: the flag has to describe the root the server actually resolved.
func TestTaskScopeDerivesTheSessionWorkspaceFlagFromTheResolvedRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv(awakenHomeEnv, home)

	folderless, err := normalizeWorkspace(WorkspaceConfig{ProjectID: "project_folderless"})
	if err != nil {
		t.Fatal(err)
	}
	if !folderless.SessionWorkspace {
		t.Fatal("a folder-less project runs in its session workspace")
	}

	claimed, err := normalizeWorkspace(WorkspaceConfig{
		WorkspaceRoot:    t.TempDir(),
		SessionWorkspace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.SessionWorkspace {
		t.Fatal("a client-supplied session workspace claim must be overwritten")
	}
}

// A folder-less project resolves to a stable directory of its own instead of
// inheriting whatever directory the core process happens to run in.
func TestFolderlessProjectResolvesToItsSessionWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv(awakenHomeEnv, home)

	scope, err := normalizeWorkspace(WorkspaceConfig{ProjectID: "project_folderless"})
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(desktopdata.SessionWorkspacePath(home, "project_folderless"))
	if scope.WorkspaceRoot != wantRoot {
		t.Fatalf("workspace root=%q want=%q", scope.WorkspaceRoot, wantRoot)
	}
	wantOutput := filepath.Clean(desktopdata.SessionOutputPath(home, "project_folderless"))
	if len(scope.OutputRoots) != 1 || scope.OutputRoots[0] != wantOutput {
		t.Fatalf("output roots=%+v want=[%q]", scope.OutputRoots, wantOutput)
	}
	// Bash needs the cwd to exist before the first command runs.
	if info, err := os.Stat(wantOutput); err != nil || !info.IsDir() {
		t.Fatalf("session output directory must exist: err=%v", err)
	}
	if scope.ProjectID != "project_folderless" {
		t.Fatalf("the scope must carry the project identity, got %q", scope.ProjectID)
	}
	wantExceptions := map[string]bool{
		filepath.Clean(desktopdata.PluginsRoot(home)):  true,
		filepath.Clean(desktopdata.SessionsRoot(home)): true,
	}
	if len(scope.ProtectedRootExceptions) != len(wantExceptions) {
		t.Fatalf("a session workspace needs the Plugin and sessions carve-outs, got %+v", scope.ProtectedRootExceptions)
	}
	for _, exception := range scope.ProtectedRootExceptions {
		if !wantExceptions[exception] {
			t.Fatalf("unexpected protected-root exception %q in %+v", exception, scope.ProtectedRootExceptions)
		}
	}
}

// Two folder-less projects must not share a working directory, otherwise one
// project's output would show up in another.
func TestFolderlessProjectsGetSeparateSessionWorkspaces(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())

	first, err := normalizeWorkspace(WorkspaceConfig{ProjectID: "project_one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeWorkspace(WorkspaceConfig{ProjectID: "project_two"})
	if err != nil {
		t.Fatal(err)
	}
	if first.WorkspaceRoot == second.WorkspaceRoot || first.ScopeID == second.ScopeID {
		t.Fatalf("session workspaces collided: %q vs %q", first.WorkspaceRoot, second.WorkspaceRoot)
	}
}

func TestWorkspaceWithNeitherRootNorProjectIsRejected(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	if _, err := normalizeWorkspace(WorkspaceConfig{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("a scope with no anchor at all must be rejected, got %v", err)
	}
}

func TestProjectlessThreadResolvesToItsOwnSessionWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv(awakenHomeEnv, home)

	scope, err := normalizeWorkspaceForThread(WorkspaceConfig{}, "thread_projectless")
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Clean(desktopdata.SessionWorkspacePath(home, "thread_projectless"))
	if scope.WorkspaceRoot != wantRoot {
		t.Fatalf("workspace root=%q want=%q", scope.WorkspaceRoot, wantRoot)
	}
	if scope.ProjectID != "" || scopeProjectID(scope) != "" {
		t.Fatalf("project-less thread must keep an empty project identity, got %+v", scope)
	}
	if !scope.SessionWorkspace {
		t.Fatal("project-less thread workspace must be marked as managed")
	}
}

func TestManagedWorkspaceRejectsUnsafeOwnerID(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	if _, err := normalizeWorkspaceForThread(WorkspaceConfig{}, "../outside"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe managed workspace owner must be rejected, got %v", err)
	}
}

func TestTaskScopeCarvesOutPluginRepositoryAndManagedSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv(awakenHomeEnv, home)
	session := desktopdata.SessionWorkspacePath(home, "project_folderless")
	if err := os.MkdirAll(desktopdata.SessionOutputPath(home, "project_folderless"), 0o755); err != nil {
		t.Fatal(err)
	}

	scope, err := normalizeWorkspace(WorkspaceConfig{WorkspaceRoot: session})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{
		filepath.Clean(desktopdata.PluginsRoot(home)):  true,
		filepath.Clean(desktopdata.SessionsRoot(home)): true,
	}
	if len(scope.ProtectedRootExceptions) != len(expected) {
		t.Fatalf("expected Plugin and sessions subtrees as carve-outs, got %+v", scope.ProtectedRootExceptions)
	}
	for _, exception := range scope.ProtectedRootExceptions {
		if !expected[exception] {
			t.Fatalf("unexpected protected-root exception %q in %+v", exception, scope.ProtectedRootExceptions)
		}
	}
}

// A snapshot written before multi-folder projects existed carries only
// workspace_root. Opening the store must promote it to a single primary user
// folder without any migration step.
func TestLegacySnapshotWithoutFoldersIsPromotedToASinglePrimaryFolder(t *testing.T) {
	home := t.TempDir()
	legacyRoot := t.TempDir()
	store, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.journal.Append(fileLifecycleMutation{Kind: "EnsureProject", Projects: []Project{{ProjectID: "project_legacy", WorkspaceRoot: normalizeProjectWorkspaceRoot(legacyRoot), DisplayName: "legacy", State: ProjectStateActive, Version: 1}}}); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"applied_seq": 0,
		"projects": map[string]any{
			"project_legacy": map[string]any{
				"project_id":     "project_legacy",
				"workspace_root": normalizeProjectWorkspaceRoot(legacyRoot),
				"display_name":   "legacy",
				"state":          string(ProjectStateActive),
				"version":        1,
			},
		},
		"workspace_index": map[string]string{
			normalizeProjectWorkspaceRoot(legacyRoot): "project_legacy",
		},
	}
	encoded, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath(), append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	reopened, err := newFileDesktopStore(home)
	if err != nil {
		t.Fatal(err)
	}
	project, err := reopened.ProjectByID("project_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Folders) != 1 {
		t.Fatalf("legacy row must synthesize exactly one folder, got %+v", project.Folders)
	}
	folder := project.Folders[0]
	if folder.Path != normalizeProjectWorkspaceRoot(legacyRoot) || folder.Role != ProjectFolderRoleUser || !folder.IsPrimary {
		t.Fatalf("legacy folder must be the primary user folder, got %+v", folder)
	}
	// The snapshot still carries the pre-sharing workspace_index key. It is no
	// longer read — ownership is derived from the folder lists — and decoding it
	// must not fail, or opening an existing store would break.
	holders, err := reopened.ProjectsHoldingFolder(legacyRoot)
	if err != nil || len(holders) != 1 || holders[0].ProjectID != "project_legacy" {
		t.Fatalf("legacy folder must resolve to its project, holders=%+v err=%v", holders, err)
	}
}
