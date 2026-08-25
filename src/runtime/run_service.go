package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"awaken/src/agentcore"
	"awaken/src/computer"
	convcontext "awaken/src/context"
	"awaken/src/desktopdata"
	"awaken/src/hookruntime"
	"awaken/src/instructions"
	"awaken/src/llm"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	bashtool "awaken/src/tools/bash"
)

var (
	ErrInvalidInput                 = errors.New("input is required")
	ErrRunNotFound                  = errors.New("run not found")
	ErrRunTerminal                  = errors.New("run is already finished")
	ErrRunCancelling                = errors.New("run is being cancelled")
	ErrThreadNotFound               = errors.New("thread not found")
	ErrThreadWorkspaceMismatch      = errors.New("thread workspace mismatch")
	ErrThreadBusy                   = errors.New("thread already has an active run")
	ErrThreadArchived               = errors.New("thread is archived")
	ErrThreadArchiveUnsupported     = errors.New("thread does not support archiving")
	ErrThreadLabelInvalid           = errors.New("thread label is required")
	ErrThreadModelPreferenceInvalid = errors.New("thread model preference is invalid")
	ErrMissingModelProfile          = errors.New("model profile is required")
	ErrUnsupportedInputAttachment   = errors.New("model does not support image input")
	ErrUserQuestionNotFound         = errors.New("user question not found")
	ErrExternalInteractionNotFound  = errors.New("external interaction not found")
	ErrPersistenceUnavailable       = agentcore.ErrPersistenceUnavailable
)

const maxRetainedThreads = 100

var threadTitleRetryDelays = []time.Duration{
	2 * time.Second,
	8 * time.Second,
}

type TaskScope struct {
	ScopeID string `json:"scope_id,omitempty"`
	// ProjectID names the project this scope belongs to. It is what lets a
	// project with no folder bound still resolve a working directory, and it
	// spares callers from deriving a project identity out of a path.
	ProjectID string `json:"project_id,omitempty"`
	// ProjectName is the display name the user gave this project. It exists so
	// the agent can name the project it is working in; nothing resolves paths or
	// permissions from it, and it is absent for runs that belong to no project.
	ProjectName string `json:"project_name,omitempty"`
	// WorkspaceRoot is the primary folder, and the base for relative paths. It
	// may be empty for a folder-less project or a project-less conversation;
	// normalizeWorkspaceForThread resolves the managed session workspace.
	WorkspaceRoot  string            `json:"workspace_root"`
	ReadableRoots  []string          `json:"readable_roots,omitempty"`
	WritableRoots  []string          `json:"writable_roots,omitempty"`
	OutputRoots    []string          `json:"output_roots,omitempty"`
	AttachedFiles  []tool.ScopedFile `json:"attached_files,omitempty"`
	ProtectedRoots []string          `json:"protected_roots,omitempty"`
	// ProtectedRootExceptions releases narrow subtrees of an otherwise
	// protected root. It is populated only when a root of this scope is an
	// Awaken-managed project workspace, which lives inside the protected
	// business-data root.
	ProtectedRootExceptions []string `json:"protected_root_exceptions,omitempty"`
	TemporaryWorkRoot       string   `json:"temporary_work_root,omitempty"`
	// SessionWorkspace reports that WorkspaceRoot is the Awaken-managed session
	// directory rather than a folder the user picked. Folder-less projects are
	// project-keyed and project-less conversations are thread-keyed. Normalizing
	// always assigns it, so a value supplied by a client never survives.
	SessionWorkspace bool `json:"session_workspace,omitempty"`
	// ExecutionProfile is the unified policy bundle applied to every tool
	// call made by agents running under this task scope. When nil, the
	// agent falls back to profile.DefaultProfile() (WorkspaceWrite).
	// Callers that want to override the default (e.g. to run a guardian
	// agent with GuardianReadOnly) set this field on StartRunRequest.Workspace.
	ExecutionProfile *profile.ExecutionProfile `json:"execution_profile,omitempty"`
}

type WorkspaceConfig = TaskScope

type ClientInfo struct {
	SessionID     string `json:"session_id"`
	RequestID     string `json:"request_id"`
	EntryPoint    string `json:"entrypoint"`
	Source        string `json:"source,omitempty"`
	ClientName    string `json:"client_name,omitempty"`
	ClientVersion string `json:"client_version,omitempty"`
}

type StartRunRequest struct {
	Input          string               `json:"input"`
	Attachments    []RunInputAttachment `json:"attachments,omitempty"`
	ComposerTokens []ComposerToken      `json:"composer_tokens,omitempty"`
	Workspace      WorkspaceConfig      `json:"workspace"`
	Client         ClientInfo           `json:"client"`
	ThreadID       string               `json:"thread_id,omitempty"`
	ModelProfile   *ModelProfileConfig  `json:"model_profile,omitempty"`
	// Title pins the initial thread title for a newly created thread instead
	// of deriving it from the prompt. Host-only (scheduled tasks); HTTP
	// handlers never map it. It also suppresses LLM title generation.
	Title string `json:"-"`
	// reanchorWorkspace lets a trusted host caller (scheduled tasks) move a
	// dedicated thread onto the project's current workspace when the project
	// folders changed, bypassing the client-mistake workspace-mismatch guard.
	// HTTP handlers never set it.
	reanchorWorkspace bool
}

type ComposerToken struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// LeadingText is the prompt text segment immediately before this token.
	// It keeps the token's inline position durable across canonical
	// user_message reflow and thread replay.
	LeadingText string               `json:"leadingText,omitempty"`
	Label       string               `json:"label"`
	Payload     ComposerTokenPayload `json:"payload,omitempty"`
}

type ComposerTokenPayload struct {
	CapabilityID  string `json:"capabilityId,omitempty"`
	SkillID       string `json:"skillId,omitempty"`
	PluginName    string `json:"pluginName,omitempty"`
	PluginDigest  string `json:"pluginDigest,omitempty"`
	SourceOrigin  string `json:"sourceOrigin,omitempty"`
	SkillFilePath string `json:"skillFilePath,omitempty"`
	RootDir       string `json:"rootDir,omitempty"`
	Path          string `json:"path,omitempty"`
	EntryKind     string `json:"entryKind,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	FolderPath    string `json:"folderPath,omitempty"`
	RelativePath  string `json:"relativePath,omitempty"`
}

type ModelProfileConfig struct {
	ProfileID           string                         `json:"profile_id,omitempty"`
	ProviderID          string                         `json:"provider_id,omitempty"`
	DisplayName         string                         `json:"display_name,omitempty"`
	ModelID             string                         `json:"model_id,omitempty"`
	CapabilityModelID   string                         `json:"capability_model_id,omitempty"`
	EndpointSelector    string                         `json:"endpoint_selector,omitempty"`
	ModelSelectorKind   string                         `json:"model_selector_kind,omitempty"`
	BaseURL             string                         `json:"base_url,omitempty"`
	InferenceEndpoint   string                         `json:"inference_endpoint,omitempty"`
	Protocol            string                         `json:"protocol,omitempty"`
	AuthScheme          string                         `json:"auth_scheme,omitempty"`
	APIKey              string                         `json:"api_key,omitempty"`
	Temperature         float64                        `json:"temperature,omitempty"`
	MaxTokens           int                            `json:"max_tokens,omitempty"`
	ContextWindow       int                            `json:"context_window,omitempty"`
	MaxInputTokens      int                            `json:"max_input_tokens,omitempty"`
	PromptCacheKey      string                         `json:"prompt_cache_key,omitempty"`
	CredentialRevision  uint64                         `json:"credential_revision,omitempty"`
	SupportsVision      bool                           `json:"supports_vision,omitempty"`
	SupportsNativePDF   bool                           `json:"supports_native_pdf,omitempty"`
	SupportsVideo       bool                           `json:"supports_video,omitempty"`
	SupportsTools       string                         `json:"supports_tools,omitempty"`
	SupportsPromptCache bool                           `json:"supports_prompt_cache,omitempty"`
	SupportsComputerUse bool                           `json:"supports_computer_use,omitempty"`
	ReasoningEffort     string                         `json:"reasoning_effort,omitempty"`
	PurposeProfiles     map[string]*ModelProfileConfig `json:"purpose_profiles,omitempty"`
}

type Executor interface {
	Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error)
}

type ExecutorFunc func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)

func (f ExecutorFunc) Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
	return f(ctx, input, sink)
}

type RunService struct {
	mu                sync.RWMutex
	projectMutationMu sync.RWMutex
	contextSnapshotMu sync.Mutex
	// runWG tracks in-flight run goroutines. Terminal run status is
	// observable before the final transcript flush completes; tests and
	// shutdown paths use drainRunWorkers to join those trailing writes.
	runWG    sync.WaitGroup
	sessions map[string]*Session
	threads  map[string]*Thread
	// projectFences maps a folder to the projects currently fencing it. A folder
	// stays fenced while any of them holds a fence.
	projectFences           map[string]map[string]struct{}
	defaultWorkspace        WorkspaceConfig
	newAgent                func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newProfileAgent         func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent
	newExecutor             func(agent *agentcore.Agent) Executor
	threadStore             ThreadStore
	projectService          *ProjectService
	journals                *journalManager
	allowNoopRecorder       bool
	skillActivator          *skillruntime.Activator
	composerPluginValidator func(name, digest string) error
	// computerDriverFactory creates a real Computer driver when local CUA is
	// enabled. If nil or it returns an error, RunService falls back to the
	// unavailable driver so run startup is not blocked.
	computerDriverFactory func(computer.Binding) (computer.Driver, error)
	// computerMediaVault isolates and bounds the lifetime of Computer media.
	// The historical field name is retained while the store API is migrated.
	computerMediaVault computer.MediaVault
	// computerHostProbe probes the global Computer Host sidecar for settings
	// and readiness checks that are not tied to a run.
	computerHostProbe computer.HostProbe
	// computerHostPermissionRequester asks the actual Host identity to request
	// every OS permission required by Computer Use.
	computerHostPermissionRequester computer.HostPermissionRequester
	// computerSettings holds user-configurable Computer Use preferences. It
	// gates whether runs can create a ComputerBinding and what policy defaults
	// apply.
	computerSettings computer.Settings
	// computerLeaseManager is shared by every run so a physical desktop can
	// have only one active Computer controller at a time.
	computerLeaseManager *computer.LeaseManager
	hookStore            *hookruntime.Store
	instructionLoader    *instructions.Loader
	globalInstructions   *instructions.GlobalDocumentStore
}

// drainRunWorkers blocks until every launched run goroutine has fully
// returned, including the trailing transcript flush that runs after the
// terminal status became observable.
func (s *RunService) drainRunWorkers() {
	if s == nil {
		return
	}
	s.runWG.Wait()
}

// SetSkillActivator injects the shared SkillActivator used for explicit
// composer-token pre-activation before the first LLM request.
func (s *RunService) SetSkillActivator(activator *skillruntime.Activator) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.skillActivator = activator
	s.mu.Unlock()
	s.restoreSkillInvocationHistory(activator)
}

// SetComposerPluginValidator installs the host-owned resolver used to prove a
// structured Plugin token still refers to the same enabled package at run
// start. The validator never trusts Renderer component lists or paths.
func (s *RunService) SetComposerPluginValidator(validator func(name, digest string) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.composerPluginValidator = validator
	s.mu.Unlock()
}

// SetComputerDriverFactory registers a factory that creates the run-bound
// Computer driver. A nil factory or a factory error causes RunService to keep
// using the unavailable driver.
func (s *RunService) SetComputerDriverFactory(factory func(computer.Binding) (computer.Driver, error)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computerDriverFactory = factory
}

// SetComputerMediaVault registers the session media store used by run-bound
// Computer runtimes.
func (s *RunService) SetComputerMediaVault(vault computer.MediaVault) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computerMediaVault = vault
}

// SetComputerHostProbe registers a global probe for Computer Host availability.
func (s *RunService) SetComputerHostProbe(probe computer.HostProbe) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computerHostProbe = probe
}

// SetComputerHostPermissionRequester registers the trusted Host permission
// request path used by the Desktop setup flow.
func (s *RunService) SetComputerHostPermissionRequester(requester computer.HostPermissionRequester) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computerHostPermissionRequester = requester
}

// SetComputerSettings registers user-configurable Computer Use preferences.
func (s *RunService) SetComputerSettings(settings computer.Settings) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.computerSettings = settings
}

// ComputerSettings returns the current Computer Use user settings.
func (s *RunService) ComputerSettings() computer.Settings {
	if s == nil {
		return computer.DefaultSettings()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.computerSettings
}

// ComputerHostStatus probes the global Computer Host sidecar. It returns an
// unavailable status when no probe is registered.
func (s *RunService) ComputerHostStatus(ctx context.Context) (computer.HostStatus, error) {
	if s == nil {
		return computer.HostStatus{Available: false, Message: "run service not initialized"}, nil
	}
	s.mu.RLock()
	probe := s.computerHostProbe
	s.mu.RUnlock()
	if probe == nil {
		return computer.HostStatus{Available: false, Message: "computer host probe not configured"}, nil
	}
	return probe.Probe(ctx)
}

// RequestComputerHostPermissions delegates to the real Host process so TCC
// grants attach to the process that performs screen capture and input.
func (s *RunService) RequestComputerHostPermissions(ctx context.Context) (computer.HostStatus, error) {
	if s == nil {
		return computer.HostStatus{Available: false, Message: "run service not initialized"}, nil
	}
	s.mu.RLock()
	requester := s.computerHostPermissionRequester
	s.mu.RUnlock()
	if requester == nil {
		return computer.HostStatus{Available: false, Message: "computer host permission requester not configured"}, nil
	}
	return requester.RequestPermissions(ctx)
}

func (s *RunService) restoreSkillInvocationHistory(activator *skillruntime.Activator) {
	if s == nil || activator == nil || activator.Registry == nil || s.threadStore == nil {
		return
	}
	if preparer, ok := s.threadStore.(interface{ Prepare() error }); ok {
		if err := preparer.Prepare(); err != nil {
			return
		}
	}
	threads, err := s.threadStore.ListThreads(ThreadListViewAll)
	if err != nil {
		return
	}
	latest := map[string]time.Time{}
	for _, thread := range threads {
		_, restored, restoreErr := s.threadStore.RestoreThread(thread.ThreadID)
		if restoreErr != nil {
			continue
		}
		for skillID, invokedAt := range restored.SkillInvokedAt {
			if previous, exists := latest[skillID]; !exists || invokedAt.After(previous) {
				latest[skillID] = invokedAt
			}
		}
	}
	for skillID, invokedAt := range latest {
		activator.Registry.MarkInvoked(skillID, invokedAt)
	}
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID                 string
	projectID                string
	agent                    *agentcore.Agent
	workspace                WorkspaceConfig
	label                    string
	titleSource              ThreadTitleSource
	titleStatus              ThreadTitleStatus
	titleUpdatedAt           time.Time
	titleSeedRunID           string
	latestRunID              string
	activeRunID              string
	runIDs                   []string
	modelProfileKey          string
	selectedModelProfileID   string
	reasoningEffortByProfile map[string]string
	createdAt                time.Time
	updatedAt                time.Time
	archived                 bool
	archivedAt               time.Time
	deletionRequested        bool
	source                   string
	clientName               string
	clientVersion            string
	acpSessionID             string
}

type ThreadTitleSource string

const (
	ThreadTitleSourceFirstUser ThreadTitleSource = "first_user"
	ThreadTitleSourceLocal     ThreadTitleSource = "local"
	ThreadTitleSourceLLM       ThreadTitleSource = "llm"
	ThreadTitleSourceManual    ThreadTitleSource = "manual"
	// ThreadTitleSourceTask marks a title owned by a scheduled task: the
	// initial thread title is the task name and task renames keep following
	// it until the user renames the thread manually.
	ThreadTitleSourceTask ThreadTitleSource = "task"
)

type ThreadTitleStatus string

const (
	ThreadTitleStatusPending ThreadTitleStatus = "pending"
	ThreadTitleStatusReady   ThreadTitleStatus = "ready"
	ThreadTitleStatusFailed  ThreadTitleStatus = "failed"
)

type ThreadSnapshot struct {
	ThreadID                 string            `json:"thread_id"`
	ProjectID                string            `json:"project_id,omitempty"`
	Label                    string            `json:"label"`
	TitleSource              ThreadTitleSource `json:"title_source,omitempty"`
	TitleStatus              ThreadTitleStatus `json:"title_status,omitempty"`
	TitleUpdatedAt           time.Time         `json:"title_updated_at,omitempty"`
	TitleSeedRunID           string            `json:"title_seed_run_id,omitempty"`
	RunID                    string            `json:"run_id,omitempty"`
	ActiveRunID              string            `json:"active_run_id,omitempty"`
	Workspace                WorkspaceConfig   `json:"workspace"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
	Archived                 bool              `json:"archived"`
	ArchivedAt               time.Time         `json:"archived_at,omitempty"`
	Source                   string            `json:"source,omitempty"`
	ClientName               string            `json:"client_name,omitempty"`
	ClientVersion            string            `json:"client_version,omitempty"`
	ACPSessionID             string            `json:"acp_session_id,omitempty"`
	SelectedModelProfileID   string            `json:"selected_model_profile_id,omitempty"`
	ReasoningEffortByProfile map[string]string `json:"reasoning_effort_by_profile,omitempty"`
}

type ThreadTranscriptRun struct {
	Run             Run
	Events          []Event
	SemanticRecords []agentcore.SemanticRecord
	StreamCursor    uint64
}

type ThreadTranscript struct {
	Thread ThreadSnapshot
	Runs   []ThreadTranscriptRun
}

func NewRunService(
	newAgent func(workspace WorkspaceConfig, threadID string) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
) *RunService {
	store := newThreadStore()
	pathFn := func(threadID string) string {
		if repo, ok := store.(ThreadRepository); ok {
			return repo.ThreadTranscriptPath(threadID)
		}
		return transcriptPath(threadID)
	}
	service := &RunService{
		sessions:             make(map[string]*Session),
		threads:              make(map[string]*Thread),
		projectFences:        make(map[string]map[string]struct{}),
		newAgent:             newAgent,
		newExecutor:          newExecutor,
		threadStore:          store,
		journals:             newJournalManager(pathFn),
		computerLeaseManager: computer.NewLeaseManager(),
		instructionLoader:    instructions.NewLoader(""),
		globalInstructions:   instructions.NewGlobalDocumentStore(""),
	}
	// Project lifecycle is available whenever the thread store also satisfies
	// the domain repository contract; no concrete storage type checks.
	if repo, ok := store.(ProjectLifecycleRepository); ok {
		service.projectService = newProjectService(service, repo)
	}
	return service
}

func (s *RunService) GetGlobalInstructions() (instructions.GlobalAgentsDocument, error) {
	if s == nil || s.globalInstructions == nil {
		return instructions.GlobalAgentsDocument{}, errors.New("global instructions store is unavailable")
	}
	return s.globalInstructions.Get()
}

func (s *RunService) PutGlobalInstructions(expectedRevision, content string) (instructions.GlobalAgentsDocument, error) {
	if s == nil || s.globalInstructions == nil {
		return instructions.GlobalAgentsDocument{}, errors.New("global instructions store is unavailable")
	}
	return s.globalInstructions.Put(expectedRevision, content)
}

func (s *RunService) DeleteGlobalInstructions(expectedRevision string) (instructions.GlobalAgentsDocument, error) {
	if s == nil || s.globalInstructions == nil {
		return instructions.GlobalAgentsDocument{}, errors.New("global instructions store is unavailable")
	}
	return s.globalInstructions.Delete(expectedRevision)
}

func NewRunServiceWithModelProfiles(
	newAgent func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
) *RunService {
	service := NewRunService(nil, newExecutor)
	service.newProfileAgent = newAgent
	service.newAgent = func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
		return newAgent(workspace, threadID, nil)
	}
	return service
}

func (s *RunService) ProjectService() *ProjectService {
	if s == nil {
		return nil
	}
	return s.projectService
}

func (s *RunService) SetHookStore(store *hookruntime.Store) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.hookStore = store
	s.mu.Unlock()
}

func hookProjectRoots(workspace WorkspaceConfig) []string {
	if workspace.SessionWorkspace {
		return nil
	}
	roots := make([]string, 0, len(workspace.ReadableRoots)+1)
	seen := map[string]struct{}{}
	for _, root := range append([]string{workspace.WorkspaceRoot}, workspace.ReadableRoots...) {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "." || !filepath.IsAbs(root) {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots
}

func (s *RunService) buildHookSnapshot(workspace WorkspaceConfig) (hookruntime.AvailabilitySnapshot, error) {
	s.mu.RLock()
	store := s.hookStore
	s.mu.RUnlock()
	if store == nil {
		return hookruntime.EmptySnapshot(), nil
	}
	return store.BuildSnapshot(scopeProjectID(workspace), hookProjectRoots(workspace), workspace.SessionWorkspace)
}

func (s *RunService) HookStatus(projectID string) (hookruntime.AvailabilitySnapshot, error) {
	s.mu.RLock()
	store := s.hookStore
	s.mu.RUnlock()
	if store == nil {
		return hookruntime.AvailabilitySnapshot{}, errors.New("hook store is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return store.Inspect("", nil, false)
	}
	if s.projectService == nil {
		return hookruntime.AvailabilitySnapshot{}, ErrProjectNotFound
	}
	projects, err := s.projectService.ListProjects(false)
	if err != nil {
		return hookruntime.AvailabilitySnapshot{}, err
	}
	for _, project := range projects {
		if project.ProjectID == projectID {
			return store.Inspect(projectID, project.AllRoots(), false)
		}
	}
	return hookruntime.AvailabilitySnapshot{}, ErrProjectNotFound
}

func (s *RunService) TrustProjectHook(projectID, sourcePath, digest string) (hookruntime.AvailabilitySnapshot, error) {
	status, err := s.HookStatus(projectID)
	if err != nil {
		return status, err
	}
	found := false
	for _, source := range status.Sources {
		if source.Kind.RequiresProjectTrust() && source.Path == filepath.Clean(sourcePath) && source.Digest == strings.TrimSpace(digest) && source.Status == hookruntime.StatusUntrusted {
			found = true
			break
		}
	}
	if !found {
		return status, errors.New("hook source or digest is not the current untrusted project config")
	}
	s.mu.RLock()
	store := s.hookStore
	s.mu.RUnlock()
	if err := store.TrustStore().Trust(projectID, sourcePath, digest); err != nil {
		return status, err
	}
	return s.HookStatus(projectID)
}

func (s *RunService) RevokeProjectHook(projectID, sourcePath string) (hookruntime.AvailabilitySnapshot, error) {
	status, err := s.HookStatus(projectID)
	if err != nil {
		return status, err
	}
	found := false
	for _, source := range status.Sources {
		if source.Kind.RequiresProjectTrust() && source.Path == filepath.Clean(sourcePath) {
			found = true
			break
		}
	}
	if !found {
		return status, errors.New("hook source is not attached to this project")
	}
	s.mu.RLock()
	store := s.hookStore
	s.mu.RUnlock()
	if err := store.TrustStore().Revoke(projectID, sourcePath); err != nil {
		return status, err
	}
	return s.HookStatus(projectID)
}

func (s *RunService) lockProjectMutation() {
	s.projectMutationMu.Lock()
}

func (s *RunService) unlockProjectMutation() {
	s.projectMutationMu.Unlock()
}

// setProjectDeletionFence blocks or unblocks run creation on one folder on
// behalf of one project. The fence records which projects raised it, because a
// folder can be shared: if project A releases its fence, a fence B raised on the
// same folder must stay up.
func (s *RunService) setProjectDeletionFence(projectID, workspaceRoot string, fenced bool) {
	if s == nil {
		return
	}
	key := normalizeProjectWorkspaceRoot(workspaceRoot)
	owner := strings.TrimSpace(projectID)
	if key == "" || owner == "" {
		return
	}
	s.mu.Lock()
	if s.projectFences == nil {
		s.projectFences = make(map[string]map[string]struct{})
	}
	if fenced {
		if s.projectFences[key] == nil {
			s.projectFences[key] = make(map[string]struct{})
		}
		s.projectFences[key][owner] = struct{}{}
	} else if owners := s.projectFences[key]; owners != nil {
		delete(owners, owner)
		if len(owners) == 0 {
			delete(s.projectFences, key)
		}
	}
	s.mu.Unlock()
}

func (s *RunService) threadIDsForWorkspace(workspaceRoot string) []string {
	key := normalizeProjectWorkspaceRoot(workspaceRoot)
	s.mu.RLock()
	defer s.mu.RUnlock()
	threadIDs := make([]string, 0)
	for threadID, thread := range s.threads {
		if normalizeProjectWorkspaceRoot(thread.workspace.WorkspaceRoot) == key {
			threadIDs = append(threadIDs, threadID)
		}
	}
	return threadIDs
}

// threadIDsForProjectFolder collects live threads anchored on one folder of one
// project. A folder can be shared between projects, so detaching it from one of
// them must not reach into the threads of the others.
// clearProjectDeletionFences reopens a folder for everyone. It is the counter
// part of the per-project fence: a folder that has just become part of a live
// project is live work again, whichever project fenced it earlier. Releasing
// only the attaching project's own fence would leave a folder one project
// detached permanently unusable for the next one to pick it up.
func (s *RunService) clearProjectDeletionFences(workspaceRoot string) {
	if s == nil {
		return
	}
	key := normalizeProjectWorkspaceRoot(workspaceRoot)
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.projectFences, key)
	s.mu.Unlock()
}

func (s *RunService) threadIDsForProjectFolder(projectID, workspaceRoot string) []string {
	projectID = strings.TrimSpace(projectID)
	key := normalizeProjectWorkspaceRoot(workspaceRoot)
	if projectID == "" || key == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	threadIDs := make([]string, 0)
	for threadID, thread := range s.threads {
		if thread.projectID == projectID && normalizeProjectWorkspaceRoot(thread.workspace.WorkspaceRoot) == key {
			threadIDs = append(threadIDs, threadID)
		}
	}
	return threadIDs
}

// threadIDsForProject collects live threads by project identity rather than by
// a single workspace path, so a project holding several folders is covered
// whichever folder a thread was started in.
func (s *RunService) threadIDsForProject(projectID string) []string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	threadIDs := make([]string, 0)
	for threadID, thread := range s.threads {
		if thread.projectID == projectID {
			threadIDs = append(threadIDs, threadID)
		}
	}
	return threadIDs
}

func (s *RunService) markThreadsDeletionRequested(threadIDs []string) {
	if len(threadIDs) == 0 {
		return
	}
	set := make(map[string]struct{}, len(threadIDs))
	for _, threadID := range threadIDs {
		set[threadID] = struct{}{}
	}
	s.mu.Lock()
	for threadID, thread := range s.threads {
		if _, ok := set[threadID]; ok {
			thread.deletionRequested = true
		}
	}
	s.mu.Unlock()
}

func (s *RunService) cancelRunsForThreads(threadIDs []string) {
	if len(threadIDs) == 0 {
		return
	}
	set := make(map[string]struct{}, len(threadIDs))
	for _, threadID := range threadIDs {
		set[threadID] = struct{}{}
	}
	s.mu.RLock()
	var sessions []*Session
	for _, thread := range s.threads {
		if _, ok := set[thread.threadID]; !ok || thread.activeRunID == "" {
			continue
		}
		if session := s.sessions[thread.activeRunID]; session != nil {
			sessions = append(sessions, session)
		}
	}
	s.mu.RUnlock()
	for _, session := range sessions {
		session.RequestCancel()
	}
	for _, threadID := range threadIDs {
		bashtool.CancelThreadTasks(threadID)
	}
}

func (s *RunService) anyThreadActive(threadIDs []string) bool {
	for _, threadID := range threadIDs {
		if s.isThreadActive(threadID) {
			return true
		}
	}
	return false
}

func (s *RunService) isThreadActive(threadID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread := s.threads[threadID]
	return thread != nil && thread.activeRunID != ""
}

// ActiveRunCount returns the number of threads that currently have an active run.
func (s *RunService) ActiveRunCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, thread := range s.threads {
		if thread.activeRunID != "" {
			count++
		}
	}
	return count
}

// RunActivitySnapshot reports how many runs are currently in flight. Active
// counts every thread with an active run, including runs that are already
// being cancelled; Cancelling counts the subset whose session status is
// RunStatusCancelling. Native sub-runs execute inside their parent's active
// run and are not counted separately.
type RunActivitySnapshot struct {
	Active     int
	Cancelling int
}

// ActivitySnapshot returns the current in-flight run counts across all run
// sources (normal, scheduled and ACP runs all go through StartRun).
func (s *RunService) ActivitySnapshot() RunActivitySnapshot {
	s.mu.RLock()
	var snapshot RunActivitySnapshot
	sessions := make([]*Session, 0, len(s.threads))
	for _, thread := range s.threads {
		if thread.activeRunID == "" {
			continue
		}
		snapshot.Active++
		if session := s.sessions[thread.activeRunID]; session != nil {
			sessions = append(sessions, session)
		}
	}
	s.mu.RUnlock()
	for _, session := range sessions {
		if session.Snapshot().Status == RunStatusCancelling {
			snapshot.Cancelling++
		}
	}
	return snapshot
}

func (s *RunService) closeAndEvictDeletedThread(threadID string) error {
	s.mu.RLock()
	thread := s.threads[threadID]
	if thread != nil && thread.activeRunID != "" {
		s.mu.RUnlock()
		return ErrThreadBusy
	}
	s.mu.RUnlock()
	if s.journals != nil {
		if err := s.journals.Flush(context.Background(), threadID); err != nil {
			return err
		}
		if err := s.journals.CloseThread(threadID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	if thread = s.threads[threadID]; thread != nil {
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return ErrThreadBusy
		}
		for _, runID := range thread.runIDs {
			delete(s.sessions, runID)
		}
		delete(s.threads, threadID)
	}
	s.mu.Unlock()
	return nil
}

func (s *RunService) onRunTerminal(threadID string) {
	if s.projectService != nil {
		s.projectService.Wake()
	}
}

func (s *RunService) SetDefaultWorkspace(workspace WorkspaceConfig) error {
	normalized, err := normalizeWorkspace(workspace)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.defaultWorkspace = normalized
	s.mu.Unlock()
	return nil
}

func (s *RunService) StartRun(req StartRunRequest) (Run, error) {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" && len(req.Attachments) == 0 && len(req.ComposerTokens) == 0 {
		return Run{}, ErrInvalidInput
	}
	if s.newExecutor == nil {
		return Run{}, fmt.Errorf("run executor is not configured")
	}
	if s.newProfileAgent != nil && req.ModelProfile == nil {
		return Run{}, ErrMissingModelProfile
	}
	if s.journals == nil && !s.allowNoopRecorder {
		return Run{}, ErrPersistenceUnavailable
	}
	if preparer, ok := s.threadStore.(interface{ Prepare() error }); ok {
		if err := preparer.Prepare(); err != nil {
			return Run{}, fmt.Errorf("prepare thread store: %w", err)
		}
	}

	runID := newRuntimeID("run")
	threadID := strings.TrimSpace(req.ThreadID)
	hasRequestedThread := threadID != ""
	if threadID == "" {
		threadID = newRuntimeID("thread")
	}

	workspace, err := normalizeWorkspaceForThread(req.Workspace, threadID)
	if err != nil {
		return Run{}, err
	}
	instructionSnapshot := s.instructionLoader.Load(workspace.WorkspaceRoot, workspace.SessionWorkspace)
	hookSnapshot, err := s.buildHookSnapshot(workspace)
	if err != nil {
		return Run{}, fmt.Errorf("build hook snapshot: %w", err)
	}
	if err := validateComposerFileTokens(req.ComposerTokens, workspace); err != nil {
		return Run{}, err
	}
	if err := validateComposerCapabilityTokens(req.ComposerTokens); err != nil {
		return Run{}, err
	}
	s.mu.RLock()
	composerPluginValidator := s.composerPluginValidator
	s.mu.RUnlock()
	if err := validateComposerPluginTokens(req.ComposerTokens, composerPluginValidator); err != nil {
		return Run{}, err
	}
	s.projectMutationMu.RLock()
	defer s.projectMutationMu.RUnlock()
	if s.projectService != nil {
		allowed, err := s.projectService.CanStartRun(workspace.WorkspaceRoot)
		if err != nil {
			return Run{}, err
		}
		if !allowed {
			return Run{}, ErrProjectOperationConflict
		}
	}
	s.mu.RLock()
	projectFenced := len(s.projectFences[normalizeProjectWorkspaceRoot(workspace.WorkspaceRoot)]) > 0
	s.mu.RUnlock()
	if projectFenced {
		return Run{}, ErrProjectOperationConflict
	}
	if err := validateRunInputAttachments(req.Attachments); err != nil {
		return Run{}, err
	}

	now := time.Now().UTC()
	var agent *agentcore.Agent
	modelProfileKey := modelProfileCacheKey(req.ModelProfile)
	var restored *restoredTranscript
	var restoredMeta *storedThreadRecord
	if hasRequestedThread {
		s.mu.RLock()
		_, threadInMemory := s.threads[threadID]
		s.mu.RUnlock()
		if !threadInMemory {
			meta, loaded, err := s.threadStore.RestoreThread(threadID)
			if err == nil {
				restoredMeta = &meta
				restored = &loaded
			} else if !errors.Is(err, ErrThreadNotFound) {
				return Run{}, err
			}
		}
	}
	var mediaArtifactReferences []tool.ScopedMediaArtifact
	if hasRequestedThread {
		transcript, transcriptErr := s.threadStore.GetTranscript(threadID)
		if transcriptErr == nil {
			mediaArtifactReferences = mediaArtifactReferencesFromTranscript(transcript)
		} else if !errors.Is(transcriptErr, ErrThreadNotFound) {
			return Run{}, transcriptErr
		}
	}
	persistedAttachments, err := persistRunInputAttachments(threadID, req.Attachments)
	if err != nil {
		return Run{}, err
	}
	workspace = mergeTaskScopeAttachments(workspace, persistedAttachments)
	effectiveInput := effectiveInputForComposerTokens(trimmed, req.ComposerTokens)
	effectiveInput = effectiveInputForMediaArtifactReferences(effectiveInput, mediaArtifactReferences)
	threadLabel := threadLabelForInputAndTokens(trimmed, persistedAttachments, req.ComposerTokens)
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, trimmed, persistedAttachments, req.ComposerTokens, cancel)
	session.SetHookSnapshotID(hookSnapshot.SnapshotID)
	session.SetInstructionSnapshot(instructionSnapshot.SafeSummary())
	if req.ModelProfile != nil {
		session.SetModelSelection(req.ModelProfile.ProfileID, req.ModelProfile.ReasoningEffort)
	}
	var threadSnapshot ThreadSnapshot
	shouldGenerateThreadTitle := false
	var previousThread *Thread
	rollbackStart := func(cause error, metadataMayBeTouched bool) (Run, error) {
		if cr := session.ComputerRuntime(); cr != nil {
			_ = cr.Close(context.Background(), "start_rollback")
		}
		rollbackErr := s.rollbackFailedStartRun(
			threadID,
			runID,
			!hasRequestedThread,
			metadataMayBeTouched,
			previousThread,
			restoredMeta,
			persistedAttachments,
			cancel,
		)
		if rollbackErr != nil {
			return Run{}, fmt.Errorf("%w (start rollback failed: %v)", cause, rollbackErr)
		}
		return Run{}, cause
	}

	s.mu.Lock()
	if hasRequestedThread {
		thread, ok := s.threads[threadID]
		if !ok {
			// Restore again while holding the RunService mutation lock. The
			// optimistic read above hydrates large transcript state without
			// blocking unrelated mutations, but archive/delete may have won in
			// between. This second read is the authority check that serializes a
			// durable-only thread with SetThreadArchived.
			latestMeta, latestRestored, latestErr := s.threadStore.RestoreThread(threadID)
			if latestErr != nil {
				s.mu.Unlock()
				if errors.Is(latestErr, ErrThreadNotFound) {
					return rollbackStart(ErrThreadNotFound, false)
				}
				return rollbackStart(latestErr, false)
			}
			restoredMeta = &latestMeta
			restored = &latestRestored
			// Archived threads reject new runs even when the caller bypasses
			// the UI; the user must unarchive first (archive contract §4.4).
			if restoredMeta.Snapshot.Archived {
				s.mu.Unlock()
				return rollbackStart(ErrThreadArchived, false)
			}
			agent = s.createAgent(workspace, threadID, req.ModelProfile)
			if agent != nil {
				agent.GetConversation().Context.SetMessages(restored.Messages)
				cfg := agent.GetConfig()
				cfg.SkillContextBlocks = append([]agentcore.ContextBlock(nil), restored.SkillContextBlocks...)
				agent.Reconfigure(cfg)
			}
			runIDs := append([]string(nil), restored.RunIDs...)
			runIDs = append(runIDs, runID)
			s.threads[threadID] = &Thread{
				threadID:  threadID,
				projectID: firstNonEmpty(restoredMeta.Snapshot.ProjectID, scopeProjectID(workspace)),
				agent:     agent,
				workspace: workspace,
				// thread label is only the creation-time fallback and must not overwrite
				// a later manual/generated rename after restart.
				// FileStore metadata owns mutable title state. The append-only journal's
				// thread label is only the creation-time fallback and must not overwrite
				// a later manual/generated rename after restart.
				// thread label is only the creation-time fallback and must not overwrite
				// a later manual/generated rename after restart.
				label:                    firstNonEmpty(restoredMeta.Snapshot.Label, restored.Label),
				titleSource:              restoredMeta.Snapshot.TitleSource,
				titleStatus:              restoredMeta.Snapshot.TitleStatus,
				titleUpdatedAt:           restoredMeta.Snapshot.TitleUpdatedAt,
				titleSeedRunID:           restoredMeta.Snapshot.TitleSeedRunID,
				latestRunID:              runID,
				activeRunID:              runID,
				runIDs:                   runIDs,
				modelProfileKey:          modelProfileKey,
				selectedModelProfileID:   restoredMeta.Snapshot.SelectedModelProfileID,
				reasoningEffortByProfile: cloneStringMap(restoredMeta.Snapshot.ReasoningEffortByProfile),
				createdAt:                firstNonZeroTime(restored.CreatedAt, restoredMeta.Snapshot.CreatedAt, now),
				updatedAt:                now,
				archived:                 restoredMeta.Snapshot.Archived,
				archivedAt:               restoredMeta.Snapshot.ArchivedAt,
				source:                   restoredMeta.Snapshot.Source,
				clientName:               restoredMeta.Snapshot.ClientName,
				clientVersion:            restoredMeta.Snapshot.ClientVersion,
				acpSessionID:             restoredMeta.Snapshot.ACPSessionID,
			}
			applyThreadModelPreference(s.threads[threadID], req.ModelProfile)
			s.sessions[runID] = session
			threadSnapshot = s.threads[threadID].snapshot()
			s.mu.Unlock()
			configureAgentForRun(agent, modelRequestPurposeForClient(req.Client), hookSnapshot, &instructionSnapshot)
			if err := s.bindAgentRecorder(agent, session, threadID, runID, req.Client.SessionID, threadSnapshot.Label); err != nil {
				return rollbackStart(err, false)
			}
			if err := persistInstructionSnapshot(session, req.Client.SessionID); err != nil {
				return rollbackStart(err, false)
			}
			s.bindSubagentController(ctx, agent, session, subagentModelProfileID(req.ModelProfile))
			if err := s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabel); err != nil {
				return rollbackStart(err, true)
			}
			if err := s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments); err != nil {
				return rollbackStart(err, true)
			}
			executor := s.newExecutor(agent)
			s.runWG.Add(1)
			go func() {
				defer s.runWG.Done()
				s.executeWithMediaArtifactReferences(session, executor, agent, effectiveInput, trimmed, persistedAttachments, mediaArtifactReferences, ctx)
			}()
			return session.Snapshot(), nil
		}
		previousThread = cloneThread(thread)
		if !sameWorkspaceRoot(thread.workspace, workspace) && !req.reanchorWorkspace {
			s.mu.Unlock()
			return rollbackStart(ErrThreadWorkspaceMismatch, false)
		}
		if thread.deletionRequested {
			s.mu.Unlock()
			return rollbackStart(ErrThreadNotFound, false)
		}
		// Archived threads reject new runs even when the caller bypasses the
		// UI; the user must unarchive first (archive contract §4.4).
		if thread.archived {
			s.mu.Unlock()
			return rollbackStart(ErrThreadArchived, false)
		}
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return rollbackStart(ErrThreadBusy, false)
		}
		agent = thread.agent
		if modelProfileKey != thread.modelProfileKey || !sameTaskScope(thread.workspace, workspace) {
			agent = s.createAgent(workspace, threadID, req.ModelProfile)
			if agent != nil && thread.agent != nil {
				agent.GetConversation().Context.SetMessages(thread.agent.GetConversation().Context.GetMessages())
				cfg := agent.GetConfig()
				cfg.SkillContextBlocks = append([]agentcore.ContextBlock(nil), thread.agent.GetConfig().SkillContextBlocks...)
				agent.Reconfigure(cfg)
			}
			thread.agent = agent
			thread.modelProfileKey = modelProfileKey
			thread.workspace = workspace
		}
		thread.activeRunID = runID
		thread.latestRunID = runID
		thread.runIDs = append(thread.runIDs, runID)
		thread.updatedAt = now
		applyThreadModelPreference(thread, req.ModelProfile)
		threadSnapshot = thread.snapshot()
	} else {
		agent = s.createAgent(workspace, threadID, req.ModelProfile)
		titleSource := ThreadTitleSourceFirstUser
		titleStatus := ThreadTitleStatusReady
		if pinned := trimThreadLabel(req.Title); pinned != "" {
			// Host-pinned title (scheduled tasks): the task name owns the
			// thread title until the user renames it manually.
			threadLabel = pinned
			titleSource = ThreadTitleSourceTask
			shouldGenerateThreadTitle = false
		} else if req.ModelProfile != nil && shouldGenerateThreadTitleForClient(req.Client) {
			titleStatus = ThreadTitleStatusPending
			shouldGenerateThreadTitle = true
		}
		s.threads[threadID] = &Thread{
			threadID:                 threadID,
			projectID:                scopeProjectID(workspace),
			agent:                    agent,
			workspace:                workspace,
			label:                    threadLabel,
			titleSource:              titleSource,
			titleStatus:              titleStatus,
			titleSeedRunID:           runID,
			titleUpdatedAt:           now,
			latestRunID:              runID,
			activeRunID:              runID,
			runIDs:                   []string{runID},
			modelProfileKey:          modelProfileKey,
			selectedModelProfileID:   profileID(req.ModelProfile),
			reasoningEffortByProfile: reasoningEffortMap(req.ModelProfile),
			createdAt:                now,
			updatedAt:                now,
			source:                   req.Client.Source,
			clientName:               req.Client.ClientName,
			clientVersion:            req.Client.ClientVersion,
			acpSessionID:             req.Client.SessionID,
		}
		s.pruneInactiveThreadsLocked()
		threadSnapshot = s.threads[threadID].snapshot()
	}
	s.sessions[runID] = session
	s.mu.Unlock()
	configureAgentForRun(agent, modelRequestPurposeForClient(req.Client), hookSnapshot, &instructionSnapshot)

	s.mu.RLock()
	computerSettings := s.computerSettings
	computerDriverFactory := s.computerDriverFactory
	computerMediaVault := s.computerMediaVault
	computerLeaseManager := s.computerLeaseManager
	s.mu.RUnlock()
	if agent != nil && req.ModelProfile != nil && req.ModelProfile.SupportsVision && computerSettings.Enabled {
		policy, policyErr := computer.NewPolicySnapshot(computer.PolicyFromSettings(computerSettings))
		if policyErr != nil {
			return rollbackStart(fmt.Errorf("create computer policy snapshot: %w", policyErr), false)
		}
		bind, bindErr := computer.NewBinding(runID, threadID, runID, "host://local/desktop", policy)
		if bindErr != nil {
			return rollbackStart(fmt.Errorf("create computer binding: %w", bindErr), false)
		}
		var driver computer.Driver = computer.NewUnavailableDriver()
		if computerDriverFactory != nil {
			if cuaDriver, driverErr := computerDriverFactory(bind); driverErr != nil {
				log.Printf("[cua] failed to create CUA driver, falling back to unavailable: %v", driverErr)
			} else {
				driver = cuaDriver
			}
		}
		runtime := computer.NewRuntimeWithLeaseManager(bind, driver, computerLeaseManager)
		runtime.SetMediaVault(computerMediaVault)
		session.SetComputerRuntime(runtime)
		agent.SetComputerRuntime(runtime)
	}

	if err := s.bindAgentRecorder(agent, session, threadID, runID, req.Client.SessionID, threadSnapshot.Label); err != nil {
		return rollbackStart(err, false)
	}
	if err := persistInstructionSnapshot(session, req.Client.SessionID); err != nil {
		return rollbackStart(err, false)
	}
	if cr := session.ComputerRuntime(); cr != nil {
		cr.SetRecordFact(func(ctx context.Context, kind string, payload map[string]any) (string, error) {
			recorder := session.semanticRecorderSnapshot()
			if recorder == nil {
				return "", agentcore.ErrPersistenceUnavailable
			}
			record := agentcore.NewSemanticRecord(agentcore.SemanticKind(kind), threadID, runID, "", "", req.Client.SessionID, payload)
			if durable, ok := recorder.(interface {
				AppendFlushed(context.Context, agentcore.SemanticRecord) (agentcore.SemanticRecord, error)
			}); ok {
				assigned, err := durable.AppendFlushed(ctx, record)
				if err != nil {
					return "", err
				}
				return assigned.RecordID, nil
			}
			if err := recorder.Append(ctx, record); err != nil {
				return "", err
			}
			if err := recorder.Flush(ctx); err != nil {
				return "", err
			}
			return record.RecordID, nil
		})
	}
	s.bindSubagentController(ctx, agent, session, subagentModelProfileID(req.ModelProfile))
	createThreadErr := s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabel)
	if createThreadErr != nil {
		return rollbackStart(createThreadErr, true)
	}
	appendInputErr := s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
	if appendInputErr != nil {
		return rollbackStart(appendInputErr, true)
	}
	if shouldGenerateThreadTitle {
		titleProfile := req.ModelProfile
		if req.ModelProfile != nil {
			if routed := req.ModelProfile.PurposeProfiles["thread_title"]; routed != nil {
				titleProfile = routed
			}
		}
		go s.scheduleThreadTitleGeneration(context.Background(), threadID, runID, trimmed, persistedAttachments, req.ComposerTokens, titleProfile)
	}
	executor := s.newExecutor(agent)
	s.runWG.Add(1)
	go func() {
		defer s.runWG.Done()
		s.executeWithMediaArtifactReferences(session, executor, agent, effectiveInput, trimmed, persistedAttachments, mediaArtifactReferences, ctx)
	}()

	return session.Snapshot(), nil
}

func cloneThread(thread *Thread) *Thread {
	if thread == nil {
		return nil
	}
	cloned := *thread
	cloned.runIDs = append([]string(nil), thread.runIDs...)
	cloned.reasoningEffortByProfile = cloneStringMap(thread.reasoningEffortByProfile)
	return &cloned
}

func (s *RunService) rollbackFailedStartRun(
	threadID string,
	runID string,
	newThread bool,
	metadataMayBeTouched bool,
	previousThread *Thread,
	restoredMeta *storedThreadRecord,
	persistedAttachments []RunInputAttachment,
	cancel context.CancelFunc,
) error {
	if cancel != nil {
		cancel()
	}
	s.mu.Lock()
	delete(s.sessions, runID)
	if previousThread != nil {
		s.threads[threadID] = cloneThread(previousThread)
	} else {
		delete(s.threads, threadID)
	}
	s.mu.Unlock()

	var rollbackErrs []error
	if err := cleanupPersistedRunInputAttachments(persistedAttachments); err != nil {
		rollbackErrs = append(rollbackErrs, err)
	}

	if newThread {
		if s.journals != nil {
			if err := s.journals.CloseThread(threadID); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		if metadataMayBeTouched {
			if err := s.threadStore.DeleteThread(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		if s.journals != nil && s.journals.pathFn != nil {
			if err := os.Remove(s.journals.pathFn(threadID)); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
	} else if metadataMayBeTouched {
		var snapshot ThreadSnapshot
		var profileKey string
		var preview string
		switch {
		case previousThread != nil:
			snapshot = previousThread.snapshot()
			profileKey = previousThread.modelProfileKey
			preview = previousThread.label
		case restoredMeta != nil:
			snapshot = restoredMeta.Snapshot
			profileKey = restoredMeta.ModelProfileKey
			preview = restoredMeta.Preview
		}
		if snapshot.ThreadID != "" {
			if err := s.threadStore.CreateThread(snapshot, profileKey, preview); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
	}
	return errors.Join(rollbackErrs...)
}

func (s *RunService) bindAgentRecorder(agent *agentcore.Agent, session *Session, threadID, runID, sessionID, label string) error {
	if agent == nil {
		return nil
	}
	// A run gets one immutable view of both the user-machine environment and
	// the executable catalog. Later MCP/Connector refreshes only affect a new
	// run (or an explicit future refresh contract), never an in-flight call.
	agent.RefreshRunAvailabilityWithReason(context.Background(), "run_start")
	agent.SetRunIdentity(runID, sessionID)
	if s.journals == nil {
		if s.allowNoopRecorder {
			recorder := agentcore.NoopSemanticRecorder{}
			agent.SetSemanticRecorder(recorder)
			if session != nil {
				session.SetSemanticRecorder(recorder)
			}
			return nil
		}
		return ErrPersistenceUnavailable
	}
	recorder, err := s.journals.OpenRecorder(threadID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	if journal, ok := recorder.(*ExecutionJournal); ok {
		if err := ensureThreadJournalRecord(context.Background(), journal, threadID, label); err != nil {
			return fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
		}
	}
	if assigned, ok := recorder.(agentcore.AssignedSemanticRecorder); ok && session != nil {
		recorder = newLiveSemanticRecorder(assigned, session)
	}
	agent.SetSemanticRecorder(recorder)
	if session != nil {
		session.SetSemanticRecorder(recorder)
	}
	return nil
}

func (s *RunService) bindSubagentController(
	ctx context.Context,
	agent *agentcore.Agent,
	session *Session,
	modelProfileID string,
) {
	if agent == nil || session == nil {
		return
	}
	agentConfig := agent.GetConfig()
	controller := subagentruntime.NewManager(ctx, subagentruntime.Config{
		MaxTasks: 8, MaxConcurrent: 4, ModelProfileID: strings.TrimSpace(modelProfileID),
		ParentWorkspaceRoot: agentConfig.Workspace.Root,
		ParentWritableRoots: append([]string(nil), agentConfig.Workspace.WritableRoots...),
		Starter: func(childCtx context.Context, request subagentruntime.StartRequest) (string, error) {
			return agent.RunNativeSubagent(childCtx, request, sessionEventSink{session: session})
		},
		Persist: func(persistCtx context.Context, event subagentruntime.PersistEvent) error {
			recorder := session.semanticRecorderSnapshot()
			if recorder == nil {
				return agentcore.ErrPersistenceUnavailable
			}
			snapshot := event.Snapshot
			record := agentcore.NewSemanticRecord(
				agentcore.SemanticKind(event.Kind), snapshot.ThreadID, snapshot.RunID,
				snapshot.TurnID, snapshot.ParentToolCallID, "", agentcore.PersistSubagentPayload(event),
			)
			if err := recorder.Append(persistCtx, record); err != nil {
				return err
			}
			return recorder.Flush(persistCtx)
		},
	})
	session.SetSubagentController(controller)
	agent.SetSubagentController(controller)
}

func subagentModelProfileID(profile *ModelProfileConfig) string {
	if profile == nil {
		return ""
	}
	if routed := profile.PurposeProfiles["subagent"]; routed != nil && strings.TrimSpace(routed.ProfileID) != "" {
		return routed.ProfileID
	}
	return profile.ProfileID
}

func modelRequestPurposeForClient(client ClientInfo) llm.RequestPurpose {
	if strings.EqualFold(strings.TrimSpace(client.Source), ScheduledClientSource) ||
		strings.EqualFold(strings.TrimSpace(client.EntryPoint), ScheduledClientSource) {
		return llm.RequestPurposeScheduledTask
	}
	return llm.RequestPurposeAgentTurn
}

func configureAgentForRun(agent *agentcore.Agent, purpose llm.RequestPurpose, snapshot hookruntime.AvailabilitySnapshot, instructionSnapshot *instructions.Snapshot) {
	if agent == nil {
		return
	}
	cfg := agent.GetConfig()
	cfg.ModelRequestPurpose = purpose
	cfg.HookAvailabilitySnapshot = &snapshot
	cfg.InstructionSnapshot = instructionSnapshot
	agent.Reconfigure(cfg)
}

func persistInstructionSnapshot(session *Session, sessionID string) error {
	if session == nil {
		return nil
	}
	snapshot := session.Snapshot()
	if snapshot.InstructionSnapshot == nil {
		return nil
	}
	recorder := session.semanticRecorderSnapshot()
	if recorder == nil {
		// Lightweight unit services explicitly opt into a no-op recorder. The
		// production StartRun guard rejects missing persistence before this point.
		return nil
	}
	record := agentcore.InstructionSnapshotSemanticRecord(snapshot.ThreadID, snapshot.RunID, sessionID, *snapshot.InstructionSnapshot)
	if err := recorder.Append(context.Background(), record); err != nil {
		return err
	}
	return recorder.Flush(context.Background())
}

func subagentSemanticPayload(snapshot subagentruntime.TaskSnapshot) map[string]any {
	return map[string]any{
		"task_id": snapshot.TaskID, "agent_id": snapshot.AgentID,
		"parent_agent_id": snapshot.ParentAgentID, "parent_tool_call_id": snapshot.ParentToolCallID,
		"thread_id": snapshot.ThreadID, "run_id": snapshot.RunID, "turn_id": snapshot.TurnID,
		"description": snapshot.Description, "agent_type": snapshot.AgentType,
		"workspace_mode": snapshot.WorkspaceMode, "model_profile_id": snapshot.ModelProfileID,
		"status": string(snapshot.Status), "current_activity": snapshot.CurrentActivity,
		"tool_use_count": snapshot.ToolUseCount, "estimated_tokens": snapshot.EstimatedTokens,
		"input_tokens": snapshot.InputTokens, "output_tokens": snapshot.OutputTokens,
		"total_tokens":   snapshot.TotalTokens,
		"result_preview": snapshot.ResultPreview, "error_code": snapshot.ErrorCode,
		"error_message": snapshot.ErrorMessage, "side_effect_state": snapshot.SideEffectState,
		"worktree":    snapshot.Worktree,
		"created_at":  subagentTimeString(snapshot.CreatedAt),
		"started_at":  subagentOptionalTimeString(snapshot.StartedAt),
		"finished_at": subagentOptionalTimeString(snapshot.FinishedAt),
	}
}

func subagentTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func subagentOptionalTimeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return subagentTimeString(*value)
}

func (s *RunService) createAgent(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
	if s.newProfileAgent != nil {
		return s.newProfileAgent(workspace, threadID, profile)
	}
	if s.newAgent != nil {
		return s.newAgent(workspace, threadID)
	}
	return nil
}

func shouldGenerateThreadTitleForClient(client ClientInfo) bool {
	return strings.EqualFold(strings.TrimSpace(client.EntryPoint), "desktop")
}

func (s *RunService) scheduleThreadTitleGeneration(ctx context.Context, threadID, seedRunID, firstUserMessage string, attachments []RunInputAttachment, composerTokens []ComposerToken, profile *ModelProfileConfig) {
	if title := localThreadTitle(firstUserMessage, attachments, composerTokens); title != "" {
		s.applyAutomaticThreadTitle(threadID, seedRunID, title, ThreadTitleSourceLocal)
		return
	}
	if profile == nil {
		log.Printf("[thread-title] skip generation: missing model profile thread=%s run=%s", threadID, seedRunID)
		s.markThreadTitleFailed(threadID, seedRunID)
		return
	}

	prompt := buildThreadTitlePrompt(firstUserMessage, attachments, composerTokens, profile)
	authority := s.threadModelCallAuthority(threadID, seedRunID)
	attempts := len(threadTitleRetryDelays) + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		if !s.shouldAttemptGeneratedThreadTitle(threadID, seedRunID) {
			return
		}
		titleCtx, cancel := context.WithTimeout(ctx, threadTitleGenerationTimeoutForProfile(profile))
		title, err := generateThreadTitleWithProfileFn(titleCtx, profile, prompt, authority, attempt-1)
		cancel()
		title = validGeneratedThreadTitle(title, firstUserMessage)
		if err == nil && title != "" {
			s.ApplyGeneratedThreadTitle(threadID, seedRunID, title)
			return
		}

		reason := "empty_or_low_quality_title"
		if err != nil {
			reason = err.Error()
		}
		log.Printf("[thread-title] generation attempt failed thread=%s run=%s attempt=%d/%d reason=%s", threadID, seedRunID, attempt, attempts, reason)
		if attempt == attempts {
			s.markThreadTitleFailed(threadID, seedRunID)
			return
		}
		timer := time.NewTimer(threadTitleRetryDelays[attempt-1])
		select {
		case <-ctx.Done():
			timer.Stop()
			s.markThreadTitleFailed(threadID, seedRunID)
			return
		case <-timer.C:
		}
	}
}

func (s *RunService) threadModelCallAuthority(threadID, runID string) agentcore.ModelCallAuthority {
	s.mu.RLock()
	session := s.sessions[runID]
	s.mu.RUnlock()
	if session == nil {
		return agentcore.ModelCallAuthority{ThreadID: threadID, RunID: runID}
	}
	return agentcore.ModelCallAuthority{
		ThreadID: threadID, RunID: runID, SessionID: "", Recorder: session.semanticRecorderSnapshot(),
		Sink: sessionEventSink{session: session},
	}
}

func (s *RunService) ApplyGeneratedThreadTitle(threadID, seedRunID, title string) {
	s.applyAutomaticThreadTitle(threadID, seedRunID, title, ThreadTitleSourceLLM)
}

func (s *RunService) applyAutomaticThreadTitle(threadID, seedRunID, title string, source ThreadTitleSource) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	title = trimThreadLabel(cleanGeneratedThreadTitle(title))
	if threadID == "" || seedRunID == "" || title == "" {
		return
	}

	now := time.Now().UTC()

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
	} else if thread.titleSource == ThreadTitleSourceManual {
		s.mu.RUnlock()
		return
	} else if thread.titleSource != ThreadTitleSourceFirstUser || thread.titleStatus != ThreadTitleStatusPending || thread.titleSeedRunID != seedRunID {
		s.mu.RUnlock()
		return
	} else {
		s.mu.RUnlock()
	}

	var persisted ThreadSnapshot
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		persisted, err = s.threadStore.UpdateThreadTitleMetadata(threadID, title, source, ThreadTitleStatusReady, seedRunID, now)
		if err == nil &&
			persisted.TitleSource == source &&
			persisted.TitleStatus == ThreadTitleStatusReady &&
			persisted.TitleSeedRunID == seedRunID &&
			persisted.Label == title {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil ||
		persisted.TitleSource != source ||
		persisted.TitleStatus != ThreadTitleStatusReady ||
		persisted.TitleSeedRunID != seedRunID ||
		persisted.Label != title {
		return
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok &&
		thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID {
		thread.label = persisted.Label
		thread.titleSource = persisted.TitleSource
		thread.titleStatus = persisted.TitleStatus
		thread.titleUpdatedAt = persisted.TitleUpdatedAt
		thread.updatedAt = persisted.UpdatedAt
	}
	s.mu.Unlock()
}

func (s *RunService) shouldAttemptGeneratedThreadTitle(threadID, seedRunID string) bool {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" || seedRunID == "" {
		return false
	}
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	s.mu.RUnlock()
	if !ok {
		return true
	}
	return thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID
}

func (s *RunService) markThreadTitleFailed(threadID, seedRunID string) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" || seedRunID == "" {
		return
	}

	now := time.Now().UTC()

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
	} else if thread.titleSource == ThreadTitleSourceManual {
		s.mu.RUnlock()
		return
	} else if thread.titleSource != ThreadTitleSourceFirstUser || thread.titleStatus != ThreadTitleStatusPending || thread.titleSeedRunID != seedRunID {
		s.mu.RUnlock()
		return
	} else {
		s.mu.RUnlock()
	}

	var persisted ThreadSnapshot
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		persisted, err = s.threadStore.UpdateThreadTitleMetadata(threadID, "", ThreadTitleSourceFirstUser, ThreadTitleStatusFailed, seedRunID, now)
		if err == nil &&
			persisted.TitleSource == ThreadTitleSourceFirstUser &&
			persisted.TitleStatus == ThreadTitleStatusFailed &&
			persisted.TitleSeedRunID == seedRunID {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil ||
		persisted.TitleSource != ThreadTitleSourceFirstUser ||
		persisted.TitleStatus != ThreadTitleStatusFailed ||
		persisted.TitleSeedRunID != seedRunID {
		return
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok &&
		thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID {
		thread.titleStatus = persisted.TitleStatus
		thread.titleUpdatedAt = persisted.TitleUpdatedAt
		thread.updatedAt = persisted.UpdatedAt
	}
	s.mu.Unlock()
}

func modelProfileCacheKey(profile *ModelProfileConfig) string {
	if profile == nil {
		return ""
	}
	parts := []string{
		profile.ProfileID,
		profile.ProviderID,
		profile.ModelID,
		profile.CapabilityModelID,
		profile.EndpointSelector,
		profile.ModelSelectorKind,
		profile.BaseURL,
		profile.InferenceEndpoint,
		profile.Protocol,
		profile.AuthScheme,
		hashSecret(profile.APIKey),
		strconv.FormatFloat(profile.Temperature, 'g', -1, 64),
		strconv.Itoa(profile.MaxTokens),
		strconv.Itoa(profile.ContextWindow),
		strconv.Itoa(profile.MaxInputTokens),
		strconv.FormatUint(profile.CredentialRevision, 10),
		strconv.FormatBool(profile.SupportsVision),
		strconv.FormatBool(profile.SupportsNativePDF),
		strconv.FormatBool(profile.SupportsVideo),
		profile.SupportsTools,
		strconv.FormatBool(profile.SupportsPromptCache),
		profile.ReasoningEffort,
	}
	purposes := make([]string, 0, len(profile.PurposeProfiles))
	for purpose := range profile.PurposeProfiles {
		purposes = append(purposes, purpose)
	}
	sort.Strings(purposes)
	for _, purpose := range purposes {
		parts = append(parts, "purpose:"+purpose, modelProfileCacheKey(profile.PurposeProfiles[purpose]))
	}
	return strings.Join(parts, "\x00")
}

func profileID(profile *ModelProfileConfig) string {
	if profile == nil {
		return ""
	}
	return strings.TrimSpace(profile.ProfileID)
}

func reasoningEffortMap(profile *ModelProfileConfig) map[string]string {
	if profile == nil || strings.TrimSpace(profile.ProfileID) == "" || strings.TrimSpace(profile.ReasoningEffort) == "" {
		return nil
	}
	return map[string]string{strings.TrimSpace(profile.ProfileID): strings.TrimSpace(profile.ReasoningEffort)}
}

func applyThreadModelPreference(thread *Thread, profile *ModelProfileConfig) {
	if thread == nil || profile == nil || strings.TrimSpace(profile.ProfileID) == "" {
		return
	}
	thread.selectedModelProfileID = strings.TrimSpace(profile.ProfileID)
	if thread.reasoningEffortByProfile == nil {
		thread.reasoningEffortByProfile = make(map[string]string)
	}
	if effort := strings.TrimSpace(profile.ReasoningEffort); effort != "" {
		thread.reasoningEffortByProfile[thread.selectedModelProfileID] = effort
	} else {
		delete(thread.reasoningEffortByProfile, thread.selectedModelProfileID)
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func hashSecret(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *RunService) ListThreads(view ThreadListView) []ThreadSnapshot {
	s.mu.RLock()
	threads := make([]ThreadSnapshot, 0, len(s.threads))
	seen := make(map[string]bool, len(s.threads))
	for _, thread := range s.threads {
		if thread.deletionRequested {
			continue
		}
		switch view {
		case ThreadListViewActive:
			if thread.archived {
				continue
			}
		case ThreadListViewArchived:
			if !thread.archived {
				continue
			}
		}
		snapshot := thread.snapshot()
		threads = append(threads, snapshot)
		seen[snapshot.ThreadID] = true
	}
	s.mu.RUnlock()

	durableThreads, err := s.threadStore.ListThreads(view)
	if err == nil {
		for _, thread := range durableThreads {
			if seen[thread.ThreadID] {
				continue
			}
			threads = append(threads, thread)
			seen[thread.ThreadID] = true
		}
	}
	sort.Slice(threads, func(i, j int) bool {
		if view == ThreadListViewArchived && !threads[i].ArchivedAt.Equal(threads[j].ArchivedAt) {
			return threads[i].ArchivedAt.After(threads[j].ArchivedAt)
		}
		if !threads[i].UpdatedAt.Equal(threads[j].UpdatedAt) {
			return threads[i].UpdatedAt.After(threads[j].UpdatedAt)
		}
		if !threads[i].CreatedAt.Equal(threads[j].CreatedAt) {
			return threads[i].CreatedAt.After(threads[j].CreatedAt)
		}
		return threads[i].ThreadID < threads[j].ThreadID
	})
	return threads
}

// SetThreadArchived flips one thread's archived state with memory/disk
// consistency: the RunService mutation lock stays held across the durable
// write, so StartRun cannot observe a half-applied archive state. The same
// target state is idempotent, a thread with an active run reports
// ErrThreadBusy (running|cancelling both hold activeRunID), scheduled-task
// dedicated threads reject with ErrThreadArchiveUnsupported, and archiving
// never moves updated_at.
func (s *RunService) SetThreadArchived(threadID string, archived bool) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.deletionRequested {
			return ThreadSnapshot{}, ErrThreadNotFound
		}
		if thread.activeRunID != "" {
			return ThreadSnapshot{}, ErrThreadBusy
		}
		if thread.source == ScheduledClientSource {
			return ThreadSnapshot{}, ErrThreadArchiveUnsupported
		}
		if thread.archived == archived {
			return thread.snapshot(), nil
		}
		durable, err := s.threadStore.SetThreadArchived(threadID, archived, now)
		if err != nil {
			return ThreadSnapshot{}, err
		}
		thread.archived = durable.Archived
		thread.archivedAt = durable.ArchivedAt
		return thread.snapshot(), nil
	}

	// Durable-only thread: keeping s.mu held across restore + mutation makes
	// this pair atomic with StartRun's locked authority re-read.
	record, _, err := s.threadStore.RestoreThread(threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	if record.Snapshot.Source == ScheduledClientSource {
		return ThreadSnapshot{}, ErrThreadArchiveUnsupported
	}
	return s.threadStore.SetThreadArchived(threadID, archived, now)
}

func (s *RunService) RenameThread(threadID, label string) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	normalizedLabel := trimThreadLabel(strings.TrimSpace(label))
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if normalizedLabel == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}

	now := time.Now().UTC()

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.deletionRequested {
			s.mu.Unlock()
			return ThreadSnapshot{}, ErrThreadNotFound
		}
		thread.label = normalizedLabel
		thread.titleSource = ThreadTitleSourceManual
		thread.titleStatus = ThreadTitleStatusReady
		thread.titleUpdatedAt = now
		thread.updatedAt = now
		snapshot := thread.snapshot()
		s.mu.Unlock()
		return s.threadStore.RenameThread(threadID, normalizedLabel, snapshot.UpdatedAt)
	}
	s.mu.Unlock()
	return s.threadStore.RenameThread(threadID, normalizedLabel, now)
}

func (s *RunService) UpdateThreadModelPreference(threadID, profileID, reasoningEffort string) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	profileID = strings.TrimSpace(profileID)
	reasoningEffort = strings.TrimSpace(reasoningEffort)
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if profileID == "" || (reasoningEffort != "" && reasoningEffort != "low" && reasoningEffort != "high" && reasoningEffort != "max") {
		return ThreadSnapshot{}, ErrThreadModelPreferenceInvalid
	}
	now := time.Now().UTC()

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.deletionRequested {
			s.mu.Unlock()
			return ThreadSnapshot{}, ErrThreadNotFound
		}
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return ThreadSnapshot{}, ErrThreadBusy
		}
		previous := cloneThread(thread)
		thread.selectedModelProfileID = profileID
		if thread.reasoningEffortByProfile == nil {
			thread.reasoningEffortByProfile = make(map[string]string)
		}
		if reasoningEffort == "" {
			delete(thread.reasoningEffortByProfile, profileID)
		} else {
			thread.reasoningEffortByProfile[profileID] = reasoningEffort
		}
		thread.updatedAt = now
		snapshot := thread.snapshot()
		profileKey := thread.modelProfileKey
		preview := thread.label
		s.mu.Unlock()
		if err := s.threadStore.CreateThread(snapshot, profileKey, preview); err != nil {
			s.mu.Lock()
			s.threads[threadID] = previous
			s.mu.Unlock()
			return ThreadSnapshot{}, err
		}
		return snapshot, nil
	}
	s.mu.Unlock()

	record, _, err := s.threadStore.RestoreThread(threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	record.Snapshot.SelectedModelProfileID = profileID
	if record.Snapshot.ReasoningEffortByProfile == nil {
		record.Snapshot.ReasoningEffortByProfile = make(map[string]string)
	}
	if reasoningEffort == "" {
		delete(record.Snapshot.ReasoningEffortByProfile, profileID)
	} else {
		record.Snapshot.ReasoningEffortByProfile[profileID] = reasoningEffort
	}
	record.Snapshot.UpdatedAt = now
	if err := s.threadStore.CreateThread(record.Snapshot, record.ModelProfileKey, record.Preview); err != nil {
		return ThreadSnapshot{}, err
	}
	return record.Snapshot, nil
}

func (s *RunService) FlushTranscripts() error {
	s.mu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.RUnlock()

	var firstErr error
	for _, session := range sessions {
		run := session.Snapshot()
		snapshot, ok := s.threadSnapshot(run.ThreadID)
		if !ok {
			continue
		}
		s.mu.RLock()
		thread, ok := s.threads[run.ThreadID]
		s.mu.RUnlock()
		if !ok || thread.agent == nil {
			continue
		}
		if err := s.flushAgentTranscript(session, thread.agent); err != nil && firstErr == nil {
			firstErr = err
		}
		_ = snapshot
	}
	if s.journals != nil {
		if err := s.journals.FlushAll(context.Background()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *RunService) DeleteThread(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	if s.projectService != nil {
		if err := s.projectService.DeleteThread(threadID); err != nil {
			return err
		}
		bashtool.CancelThreadTasks(threadID)
		_, err := s.projectService.ProcessCleanupJobs(context.Background(), 16)
		return err
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return ErrThreadBusy
		}
		runIDs := append([]string(nil), thread.runIDs...)
		delete(s.threads, threadID)
		for _, runID := range runIDs {
			delete(s.sessions, runID)
		}
		s.mu.Unlock()
		bashtool.CancelThreadTasks(threadID)
		return s.threadStore.DeleteThread(threadID)
	}
	s.mu.Unlock()
	bashtool.CancelThreadTasks(threadID)
	return s.threadStore.DeleteThread(threadID)
}

// ThreadDeleteFailure preserves the thread identity for a failed member of a
// batch delete. Batch deletion is intentionally per-thread rather than
// transactional: project-less conversations are removed directly from the
// FileStore, so a later filesystem failure cannot roll back an earlier delete.
type ThreadDeleteFailure struct {
	ThreadID string
	Err      error
}

// DeleteThreads applies the ordinary durable delete contract to each thread
// and returns an explicit per-thread result. Callers must not interpret a
// partial success as an all-or-nothing transaction.
func (s *RunService) DeleteThreads(threadIDs []string) ([]string, []ThreadDeleteFailure) {
	deleted := make([]string, 0, len(threadIDs))
	failed := make([]ThreadDeleteFailure, 0)
	for _, threadID := range threadIDs {
		if err := s.DeleteThread(threadID); err != nil {
			failed = append(failed, ThreadDeleteFailure{ThreadID: threadID, Err: err})
			continue
		}
		deleted = append(deleted, threadID)
	}
	return deleted, failed
}

func (s *RunService) Subscribe(runID string, afterSeq uint64) (Subscription, error) {
	session, err := s.session(runID)
	if err != nil {
		return Subscription{}, err
	}
	return session.Subscribe(afterSeq), nil
}

func (s *RunService) GetRun(runID string) (Run, error) {
	session, err := s.session(runID)
	if err != nil {
		return Run{}, err
	}
	return session.Snapshot(), nil
}

func (s *RunService) GetThreadTranscript(threadID string) (ThreadTranscript, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return s.threadStore.GetTranscript(threadID)
	}
	if thread.deletionRequested {
		s.mu.RUnlock()
		return ThreadTranscript{}, ErrThreadNotFound
	}
	snapshot := thread.snapshot()
	runIDs := append([]string(nil), thread.runIDs...)
	sessionByRunID := make(map[string]*Session, len(runIDs))
	for _, runID := range runIDs {
		session, ok := s.sessions[runID]
		if !ok {
			continue
		}
		sessionByRunID[runID] = session
	}
	s.mu.RUnlock()

	type activeRunSnapshot struct {
		run          Run
		events       []Event
		streamCursor uint64
		publishedIDs map[string]struct{}
	}
	activeRuns := make(map[string]activeRunSnapshot, len(sessionByRunID))
	for runID, session := range sessionByRunID {
		events, cursor := session.StreamSnapshot()
		published := make(map[string]struct{})
		for _, event := range events {
			if event.Kind == EventKindSemantic && event.SemanticRecord.RecordID != "" {
				published[event.SemanticRecord.RecordID] = struct{}{}
			}
		}
		activeRuns[runID] = activeRunSnapshot{
			run:          session.Snapshot(),
			events:       events,
			streamCursor: cursor,
			publishedIDs: published,
		}
	}

	durableRuns := map[string]ThreadTranscriptRun{}
	if s.journals != nil {
		if journal, found := s.journals.Get(threadID); found {
			records, err := journal.SnapshotRecords()
			if err != nil {
				return ThreadTranscript{}, err
			}
			durableRuns = durableRunsFromSemanticRecords(threadID, records)
		}
	}
	if len(durableRuns) == 0 && len(sessionByRunID) < len(runIDs) {
		loaded, err := s.threadStore.GetTranscript(threadID)
		if err == nil {
			for _, run := range loaded.Runs {
				durableRuns[run.Run.RunID] = run
			}
		} else if !errors.Is(err, ErrThreadNotFound) {
			return ThreadTranscript{}, err
		}
	}

	runs := make([]ThreadTranscriptRun, 0, len(runIDs))
	for _, runID := range runIDs {
		if active, ok := activeRuns[runID]; ok {
			if durable, exists := durableRuns[runID]; exists {
				published := make([]agentcore.SemanticRecord, 0, len(durable.SemanticRecords))
				for _, record := range durable.SemanticRecords {
					_, visible := active.publishedIDs[record.RecordID]
					if visible || isActiveTranscriptReplaySupplement(record.Kind) {
						published = append(published, record)
					}
				}
				durable.Run = active.run
				durable.SemanticRecords = append([]agentcore.SemanticRecord(nil), published...)
				durable.StreamCursor = active.streamCursor
				durable.Events = canonicalizeActiveTranscriptEvents(active.events, durable.SemanticRecords)
				runs = append(runs, durable)
				continue
			}
			runs = append(runs, ThreadTranscriptRun{
				Run:          active.run,
				Events:       active.events,
				StreamCursor: active.streamCursor,
			})
			continue
		}
		if durable, ok := durableRuns[runID]; ok {
			runs = append(runs, durable)
		}
	}

	return ThreadTranscript{
		Thread: snapshot,
		Runs:   runs,
	}, nil
}

func isActiveTranscriptReplaySupplement(kind agentcore.SemanticKind) bool {
	switch kind {
	case agentcore.SemanticKindContextSnapshot,
		agentcore.SemanticKindContextEpoch,
		agentcore.SemanticKindInstructionSnapshot,
		agentcore.SemanticKindModelUsage:
		return true
	default:
		return false
	}
}

func canonicalizeActiveTranscriptEvents(events []Event, durableRecords []agentcore.SemanticRecord) []Event {
	durableByID := make(map[string]agentcore.SemanticRecord, len(durableRecords))
	assistantMessages := make(map[string]struct{})
	reasoningMessages := make(map[string]struct{})
	toolCalls := make(map[string]struct{})
	for _, record := range durableRecords {
		durableByID[record.RecordID] = record
		switch record.Kind {
		case agentcore.SemanticKindReasoningSummary:
			if id := stringPayload(record.Payload, "message_id"); id != "" {
				reasoningMessages[id] = struct{}{}
			}
		case agentcore.SemanticKindAssistantMessage:
			if id := stringPayload(record.Payload, "message_id"); id != "" {
				assistantMessages[id] = struct{}{}
			}
		case agentcore.SemanticKindToolCall:
			if id := stringPayload(record.Payload, "tool_call_id"); id != "" {
				toolCalls[id] = struct{}{}
			}
		}
	}

	result := make([]Event, 0, len(events))
	visibleSemantic := make(map[string]struct{}, len(durableRecords))
	var nextSeq uint64
	for _, event := range events {
		if event.Seq >= nextSeq {
			nextSeq = event.Seq + 1
		}
		if event.Kind == EventKindSemantic {
			if canonical, ok := durableByID[event.SemanticRecord.RecordID]; ok {
				event.SemanticRecord = canonical
			}
			visibleSemantic[event.SemanticRecord.RecordID] = struct{}{}
			result = append(result, event)
			continue
		}
		if event.Kind == EventKindAgent {
			switch event.AgentEvent.Type {
			case agentcore.ReasoningStartedEventType,
				agentcore.ReasoningDeltaEventType,
				agentcore.ReasoningFinishedEventType:
				if _, superseded := reasoningMessages[event.AgentEvent.MessageID]; superseded {
					continue
				}
			case agentcore.TextMessageStartedEventType,
				agentcore.TextDeltaEventType,
				agentcore.TextMessageFinishedEventType:
				if _, superseded := assistantMessages[event.AgentEvent.MessageID]; superseded {
					continue
				}
			case agentcore.ToolCallStartedEventType,
				agentcore.ToolCallArgsDeltaEventType,
				agentcore.ToolCallArgsEventType,
				agentcore.ToolCallFinishedEventType:
				if _, superseded := toolCalls[event.AgentEvent.ToolCallID]; superseded {
					continue
				}
			}
		}
		result = append(result, event)
	}
	for _, record := range durableRecords {
		if _, visible := visibleSemantic[record.RecordID]; visible {
			continue
		}
		result = append(result, Event{
			Kind: EventKindSemantic, Seq: nextSeq, Timestamp: record.Timestamp,
			ThreadID: record.ThreadID, RunID: record.RunID, SemanticRecord: record,
		})
		nextSeq++
	}
	return result
}

func threadSnapshotFromRestored(threadID string, workspace WorkspaceConfig, restored restoredTranscript) ThreadSnapshot {
	latestRunID := ""
	if len(restored.RunIDs) > 0 {
		latestRunID = restored.RunIDs[len(restored.RunIDs)-1]
	}
	return ThreadSnapshot{
		ThreadID:  threadID,
		ProjectID: scopeProjectID(workspace),
		Label:     restored.Label,
		RunID:     latestRunID,
		Workspace: workspace,
		CreatedAt: restored.CreatedAt,
		UpdatedAt: restored.UpdatedAt,
	}
}

func (s *RunService) CancelRun(runID string) (Run, error) {
	session, err := s.session(runID)
	if err != nil {
		return Run{}, err
	}
	run := session.RequestCancel()
	bashtool.CancelThreadTasks(run.ThreadID)
	return run, nil
}

func (s *RunService) ListSubagents(runID string) ([]subagentruntime.TaskSnapshot, error) {
	session, err := s.session(runID)
	if err != nil {
		return nil, err
	}
	controller := session.SubagentController()
	if controller == nil {
		return []subagentruntime.TaskSnapshot{}, nil
	}
	return controller.List(), nil
}

func (s *RunService) GetSubagent(runID, taskID string) (subagentruntime.TaskSnapshot, error) {
	session, err := s.session(runID)
	if err != nil {
		return subagentruntime.TaskSnapshot{}, err
	}
	controller := session.SubagentController()
	if controller == nil {
		return subagentruntime.TaskSnapshot{}, &subagentruntime.Error{Code: subagentruntime.ErrorNotFound, Message: "subagent task not found"}
	}
	return controller.Status(taskID)
}

func (s *RunService) MessageSubagent(ctx context.Context, runID, taskID, content, idempotencyKey string) (subagentruntime.MessageReceipt, error) {
	session, err := s.session(runID)
	if err != nil {
		return subagentruntime.MessageReceipt{}, err
	}
	controller := session.SubagentController()
	if controller == nil {
		return subagentruntime.MessageReceipt{}, &subagentruntime.Error{Code: subagentruntime.ErrorNotFound, Message: "subagent task not found"}
	}
	if manager, ok := controller.(*subagentruntime.Manager); ok {
		return manager.MessageIdempotent(ctx, taskID, content, idempotencyKey)
	}
	return controller.Message(ctx, taskID, content)
}

func (s *RunService) CancelSubagent(ctx context.Context, runID, taskID, reason string) (subagentruntime.TaskSnapshot, error) {
	session, err := s.session(runID)
	if err != nil {
		return subagentruntime.TaskSnapshot{}, err
	}
	controller := session.SubagentController()
	if controller == nil {
		return subagentruntime.TaskSnapshot{}, &subagentruntime.Error{Code: subagentruntime.ErrorNotFound, Message: "subagent task not found"}
	}
	return controller.Cancel(ctx, taskID, reason)
}

func (s *RunService) ResolveUserQuestion(runID, questionID, answer string) error {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return ErrRunNotFound
	}
	return session.ResolveUserQuestion(questionID, answer)
}

func (s *RunService) CancelUserQuestion(runID, questionID string) error {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return ErrRunNotFound
	}
	return session.CancelUserQuestion(questionID)
}

func (s *RunService) ResolveExternalInteraction(runID, interactionID string, resolution agentcore.ExternalInteractionResolution) error {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return ErrRunNotFound
	}
	return session.ResolveExternalInteraction(interactionID, resolution)
}

func partialAssistantResult(agent *agentcore.Agent) string {
	if agent == nil {
		return ""
	}
	messages := agent.GetConversation().Context.GetMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == convcontext.RoleAssistant && strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func (s *RunService) execute(session *Session, executor Executor, agent *agentcore.Agent, input string, displayInput string, attachments []RunInputAttachment, ctx context.Context) {
	s.executeWithMediaArtifactReferences(session, executor, agent, input, displayInput, attachments, nil, ctx)
}

func (s *RunService) executeWithMediaArtifactReferences(session *Session, executor Executor, agent *agentcore.Agent, input string, displayInput string, attachments []RunInputAttachment, mediaArtifactReferences []tool.ScopedMediaArtifact, ctx context.Context) {
	snapshot := session.Snapshot()
	baselineSkillConfig, hasBaselineSkillConfig := snapshotAgentSkillConfig(agent)
	var baselineMediaArtifactReferences []tool.ScopedMediaArtifact
	if agent != nil {
		baselineMediaArtifactReferences = append([]tool.ScopedMediaArtifact(nil), agent.GetConfig().MediaArtifactReferences...)
		cfg := agent.GetConfig()
		cfg.MediaArtifactReferences = append([]tool.ScopedMediaArtifact(nil), mediaArtifactReferences...)
		agent.Reconfigure(cfg)
	}
	defer func() {
		if agent != nil {
			agent.ClearPreparedUserMessage()
		}
		if hasBaselineSkillConfig {
			restoreAgentSkillConfig(agent, baselineSkillConfig)
		}
		if agent != nil {
			cfg := agent.GetConfig()
			cfg.MediaArtifactReferences = baselineMediaArtifactReferences
			agent.Reconfigure(cfg)
		}
		restoreAgentDisplayInput(agent, input, displayInput)
		s.flushAgentTranscript(session, agent)
		if cr := session.ComputerRuntime(); cr != nil {
			_ = cr.Close(context.Background(), "run_ended")
		}
		s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)
		s.onRunTerminal(snapshot.ThreadID)
	}()

	if executor == nil {
		session.CompleteError("run executor is not configured", "executor_missing")
		return
	}
	if agent != nil {
		agent.SetPendingUserAttachments(inputAttachmentsToContextAttachments(attachments))
		agent.SetPendingComposerTokens(semanticComposerTokensForRun(snapshot.ComposerTokens))
		if input != displayInput || len(snapshot.ComposerTokens) > 0 {
			if err := agent.PrepareUserMessageWithDisplayInput(
				ctx,
				input,
				displayInput,
				sessionEventSink{session: session},
			); err != nil {
				code := "persistence_error"
				if errors.Is(err, agentcore.ErrPersistenceUnavailable) {
					code = "persistence_unavailable"
				}
				session.CompleteError(err.Error(), code)
				return
			}
		}
		agent.ConfigureHookDispatcher()
		if agent.HookPreflightRequiresPreparedUserMessage() && input == displayInput && len(snapshot.ComposerTokens) == 0 {
			if err := agent.PrepareUserMessageWithDisplayInput(ctx, input, displayInput, sessionEventSink{session: session}); err != nil {
				session.CompleteError(err.Error(), "persistence_error")
				return
			}
		}
		if err := agent.RunHookPreflight(ctx, sessionEventSink{session: session}, displayInput); err != nil {
			code := "hook_failed"
			if errors.Is(err, agentcore.ErrHookBlocked) {
				code = "hook_blocked"
			} else if errors.Is(err, hookruntime.ErrPersistence) || errors.Is(err, agentcore.ErrPersistenceFailed) {
				code = "persistence_error"
			}
			session.CompleteError(err.Error(), code)
			return
		}
		if err := s.preActivateComposerSkillsWithSink(ctx, agent, snapshot, sessionEventSink{session: session}); err != nil {
			session.CompleteError(err.Error(), skillActivationErrorCode(err))
			return
		}
	}

	result, err := executor.Run(ctx, input, sessionEventSink{session: session})
	if err == nil {
		if session.CancelRequested() {
			session.CompleteCancelled("你停止了生成本次回答", partialAssistantResult(agent))
			s.flushAgentTranscript(session, agent)
			return
		}
		session.CompleteFinished(result)
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) || session.CancelRequested() {
		session.CompleteCancelled("你停止了生成本次回答", partialAssistantResult(agent))
		s.flushAgentTranscript(session, agent)
		return
	}

	session.CompleteError(err.Error(), "")
}

func skillActivationErrorCode(err error) string {
	if errors.Is(err, agentcore.ErrPersistenceUnavailable) {
		return "persistence_unavailable"
	}
	var compat *skillruntime.SkillCompatibilityError
	if errors.As(err, &compat) && compat != nil && strings.TrimSpace(compat.Code) != "" {
		return compat.Code
	}
	return "skill_activation_failed"
}

func (s *RunService) preActivateComposerSkills(ctx context.Context, agent *agentcore.Agent, snapshot Run) error {
	return s.preActivateComposerSkillsWithSink(ctx, agent, snapshot, nil)
}

func (s *RunService) preActivateComposerSkillsWithSink(ctx context.Context, agent *agentcore.Agent, snapshot Run, sink agentcore.EventSink) error {
	tokens := skillComposerTokens(snapshot.ComposerTokens)
	if len(tokens) == 0 || agent == nil {
		return nil
	}
	s.mu.RLock()
	activator := s.skillActivator
	s.mu.RUnlock()
	if activator == nil {
		err := fmt.Errorf("skill activator unavailable")
		if recorder := agent.SemanticRecorder(); recorder != nil {
			if persistErr := persistSkillPreActivationFailure(ctx, recorder, snapshot, nil, err); persistErr != nil {
				return fmt.Errorf("persist skill pre-activation failure: %w", persistErr)
			}
		}
		return err
	}

	cfg := agent.GetConfig()
	blocks := append([]agentcore.ContextBlock(nil), cfg.SkillContextBlocks...)
	recorder := agent.SemanticRecorder()
	plans := make([]*skillruntime.SkillActivationPlan, 0, len(tokens))

	for _, token := range tokens {
		skillID := composerTokenSkillID(token)
		if skillID == "" {
			continue
		}
		plan, err := activator.Activate(skillruntime.ActivationRequest{
			Context: ctx, SkillID: skillID, RawArgs: snapshot.Input,
			InvocationSource: skillruntime.InvocationUser,
		})
		if err != nil {
			failurePlans := plans
			if plan != nil {
				failurePlans = append(append([]*skillruntime.SkillActivationPlan(nil), plans...), plan.Clone())
			}
			if recorder != nil {
				if persistErr := persistSkillPreActivationFailure(ctx, recorder, snapshot, failurePlans, err); persistErr != nil {
					return fmt.Errorf("persist skill pre-activation failure: %w", persistErr)
				}
			}
			return err
		}
		if err := validateComposerSkillToken(token, plan, activator.Registry); err != nil {
			if recorder != nil {
				if persistErr := persistSkillPreActivationFailure(ctx, recorder, snapshot, plans, err); persistErr != nil {
					return fmt.Errorf("persist skill pre-activation failure: %w", persistErr)
				}
			}
			return err
		}
		plans = append(plans, plan.Clone())
		if plan.Context != skills.ContextFork {
			blocks = append(blocks, agentcore.ContextBlock{
				ID:         "skill_activation_" + plan.SkillID,
				SkillID:    plan.SkillID,
				SourcePath: filepath.Join(plan.RootDir, "SKILL.md"),
				InvokedAt:  time.Now().UTC(),
				Content:    plan.MainBody,
			})
		}
	}
	if recorder != nil {
		activationRecords := make([]agentcore.SemanticRecord, 0, len(plans))
		for _, plan := range plans {
			activationRecords = append(activationRecords, skillActivationRecords(snapshot, plan)...)
		}
		if err := recorder.AppendBatch(ctx, activationRecords); err != nil {
			persistenceErr := fmt.Errorf("%w: persist composer skill activations: %v", agentcore.ErrPersistenceUnavailable, err)
			if terminalErr := persistSkillPreActivationFailure(ctx, recorder, snapshot, nil, persistenceErr); terminalErr != nil {
				return fmt.Errorf("%w; persist failure terminal: %v", persistenceErr, terminalErr)
			}
			return persistenceErr
		}
	}
	if activator.Registry != nil {
		for _, plan := range plans {
			activator.Registry.MarkInvoked(plan.SkillID, time.Now().UTC())
		}
	}
	for _, plan := range plans {
		if err := agent.RegisterActivatedSkillHooks(plan); err != nil {
			return fmt.Errorf("register activated Skill Hook %q: %w", plan.SkillID, err)
		}
	}
	for _, plan := range plans {
		if plan.Context != skills.ContextFork {
			continue
		}
		if sink == nil {
			return &skillruntime.SkillCompatibilityError{Code: "skill_fork_unavailable", Message: "composer fork Skill requires a live run event sink"}
		}
		child, err := agent.RunForkedSkill(ctx, skillruntime.SkillForkRequest{
			SkillID: plan.SkillID, Body: plan.MainBody, RootDir: plan.RootDir,
			SkillPath: plan.SkillFilePath, Agent: plan.Agent, Model: plan.Model,
			Effort: plan.Effort, ToolHints: append([]skills.ToolHint(nil), plan.ToolHints...),
		}, sink)
		if err != nil {
			return fmt.Errorf("fork composer Skill %q: %w", plan.SkillID, err)
		}
		blocks = append(blocks, agentcore.ContextBlock{
			ID: "skill_fork_result_" + plan.SkillID, SkillID: plan.SkillID,
			AgentID: child.ChildID, InvokedAt: time.Now().UTC(),
			Content: "Forked Skill child result (already executed; do not repeat the child task):\n\n" + child.Content,
		})
	}

	cfg.SkillContextBlocks = blocks
	cfg.ComposerSkillSelection = composerSkillSelectionContext(snapshot.RunID, plans)
	agent.Reconfigure(cfg)
	return nil
}

func composerTokenSkillID(token ComposerToken) string {
	skillID := strings.TrimSpace(firstNonEmpty(token.Payload.SkillID, token.ID))
	if !strings.HasPrefix(skillID, "skill:") {
		return skillID
	}
	parts := strings.Split(skillID, ":")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func validateComposerSkillToken(token ComposerToken, plan *skillruntime.SkillActivationPlan, registry *skills.SkillRegistry) error {
	if plan == nil {
		return &skillruntime.SkillCompatibilityError{Code: "skill_composer_token_invalid", Message: "composer Skill activation returned no plan"}
	}
	if registry == nil {
		return &skillruntime.SkillCompatibilityError{Code: "skill_composer_token_invalid", Message: "composer Skill registry unavailable"}
	}
	definition := registry.Get(plan.SkillID)
	if definition == nil {
		return &skillruntime.SkillCompatibilityError{Code: "skill_composer_token_invalid", Message: fmt.Sprintf("composer Skill %q is no longer registered", plan.SkillID)}
	}
	if !definition.UserInvocable {
		return &skillruntime.SkillCompatibilityError{Code: "skill_user_invocation_disabled", Message: fmt.Sprintf("skill %q is not user-invocable", plan.SkillID)}
	}
	if plan.Source != skills.SourceMcp &&
		(!filepath.IsAbs(strings.TrimSpace(plan.RootDir)) || !filepath.IsAbs(strings.TrimSpace(plan.SkillFilePath))) {
		return &skillruntime.SkillCompatibilityError{
			Code:    "skill_composer_token_invalid",
			Message: fmt.Sprintf("composer Skill %q registry identity must use absolute root and SKILL.md paths", plan.SkillID),
		}
	}
	if claimed := strings.TrimSpace(token.Payload.SourceOrigin); claimed != "" &&
		!strings.EqualFold(claimed, string(plan.Source)) {
		return &skillruntime.SkillCompatibilityError{
			Code:    "skill_composer_token_mismatch",
			Message: fmt.Sprintf("composer Skill %q source changed from %q to %q", plan.SkillID, claimed, plan.Source),
		}
	}
	if claimed := strings.TrimSpace(token.Payload.SkillFilePath); claimed != "" &&
		!sameComposerSkillPath(claimed, plan.SkillFilePath) {
		return &skillruntime.SkillCompatibilityError{
			Code:    "skill_composer_token_mismatch",
			Message: fmt.Sprintf("composer Skill %q entrypoint changed", plan.SkillID),
		}
	}
	if claimed := strings.TrimSpace(token.Payload.RootDir); claimed != "" &&
		!sameComposerSkillPath(claimed, plan.RootDir) {
		return &skillruntime.SkillCompatibilityError{
			Code:    "skill_composer_token_mismatch",
			Message: fmt.Sprintf("composer Skill %q root changed", plan.SkillID),
		}
	}
	return nil
}

func sameComposerSkillPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return left == right
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func composerSkillSelectionContext(runID string, plans []*skillruntime.SkillActivationPlan) *agentcore.ComposerSkillSelectionContext {
	selections := make([]agentcore.ComposerSkillSelection, 0, len(plans))
	for _, plan := range plans {
		if plan == nil || strings.TrimSpace(plan.SkillID) == "" || strings.TrimSpace(string(plan.Source)) == "" {
			continue
		}
		selections = append(selections, agentcore.ComposerSkillSelection{
			SkillID:        plan.SkillID,
			Source:         string(plan.Source),
			ActivationMode: string(plan.Context),
			RootDir:        plan.RootDir,
			SkillFilePath:  plan.SkillFilePath,
		})
	}
	if len(selections) == 0 {
		return nil
	}
	return &agentcore.ComposerSkillSelectionContext{
		RunID:      runID,
		Selections: selections,
	}
}

func skillActivationRecords(snapshot Run, plan *skillruntime.SkillActivationPlan) []agentcore.SemanticRecord {
	if plan == nil {
		return nil
	}
	payload := plan.DurablePayload()
	payload["skill_path"] = filepath.Join(plan.RootDir, "SKILL.md")
	payload["content"] = plan.MainBody
	payload["invoked_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	records := []agentcore.SemanticRecord{agentcore.NewSemanticRecord(
		agentcore.SemanticKindSkillActivated,
		snapshot.ThreadID,
		snapshot.RunID,
		"",
		"",
		"",
		payload,
	)}
	for _, receipt := range plan.ResourceReceipts {
		records = append(records, agentcore.NewSemanticRecord(
			agentcore.SemanticKindResourceReceipt,
			snapshot.ThreadID,
			snapshot.RunID,
			"",
			"",
			"",
			receipt.DurablePayload(),
		))
	}
	return records
}

func persistSkillPreActivationFailure(ctx context.Context, recorder agentcore.SemanticRecorder, snapshot Run, plans []*skillruntime.SkillActivationPlan, activationErr error) error {
	if recorder == nil {
		return nil
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	records := []agentcore.SemanticRecord{agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunStarted,
		snapshot.ThreadID,
		snapshot.RunID,
		"",
		"",
		"",
		map[string]any{},
	)}
	for _, plan := range plans {
		records = append(records, skillActivationRecords(snapshot, plan)...)
	}
	code := skillActivationErrorCode(activationErr)
	records = append(records, agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal,
		snapshot.ThreadID,
		snapshot.RunID,
		"",
		"",
		"",
		map[string]any{
			"status":          "error",
			"error":           activationErr.Error(),
			"code":            code,
			"terminal_reason": "skill_activation_failed",
		},
	))
	if err := recorder.AppendBatch(persistCtx, records); err != nil {
		return err
	}
	return recorder.Flush(persistCtx)
}

type agentSkillConfigSnapshot struct {
	skillContextBlocks     []agentcore.ContextBlock
	composerSkillSelection *agentcore.ComposerSkillSelectionContext
}

func snapshotAgentSkillConfig(agent *agentcore.Agent) (agentSkillConfigSnapshot, bool) {
	if agent == nil {
		return agentSkillConfigSnapshot{}, false
	}
	cfg := agent.GetConfig()
	snapshot := agentSkillConfigSnapshot{
		skillContextBlocks:     append([]agentcore.ContextBlock(nil), cfg.SkillContextBlocks...),
		composerSkillSelection: cloneAgentComposerSkillSelection(cfg.ComposerSkillSelection),
	}
	return snapshot, true
}

func restoreAgentSkillConfig(agent *agentcore.Agent, snapshot agentSkillConfigSnapshot) {
	if agent == nil {
		return
	}
	cfg := agent.GetConfig()
	cfg.SkillContextBlocks = append([]agentcore.ContextBlock(nil), snapshot.skillContextBlocks...)
	cfg.ComposerSkillSelection = cloneAgentComposerSkillSelection(snapshot.composerSkillSelection)
	agent.Reconfigure(cfg)
}

func cloneAgentComposerSkillSelection(value *agentcore.ComposerSkillSelectionContext) *agentcore.ComposerSkillSelectionContext {
	if value == nil {
		return nil
	}
	return &agentcore.ComposerSkillSelectionContext{
		RunID:      value.RunID,
		Selections: append([]agentcore.ComposerSkillSelection(nil), value.Selections...),
	}
}

const computerUseComposerInstruction = "请优先使用 Computer Use 观察并操作本机图形界面完成以下任务："

func pluginComposerInstruction(name string) string {
	return fmt.Sprintf("用户已明确选择已启用的 Awaken Plugin %q。请优先使用该插件提供的 Skill 与 MCP 工具完成以下任务：", name)
}

func composerTokenModelExpansion(token ComposerToken) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(token.Type)) {
	case "file":
		return strings.TrimSpace(token.Payload.Path), true
	case "capability":
		if strings.EqualFold(strings.TrimSpace(token.Payload.CapabilityID), "computer-use") {
			return computerUseComposerInstruction + "\n", true
		}
	case "plugin":
		if name := strings.TrimSpace(token.Payload.PluginName); name != "" {
			return pluginComposerInstruction(name) + "\n", true
		}
	}
	return "", false
}

func effectiveInputForComposerTokens(input string, tokens []ComposerToken) string {
	hasModelExpansion := false
	visiblePrefix := strings.Builder{}
	fallbackInstructions := make([]string, 0, len(tokens))
	fallbackReferences := make([]string, 0, len(tokens))
	for _, token := range tokens {
		visiblePrefix.WriteString(token.LeadingText)
		expansion, expands := composerTokenModelExpansion(token)
		if !expands {
			continue
		}
		hasModelExpansion = true
		if strings.EqualFold(strings.TrimSpace(token.Type), "capability") ||
			strings.EqualFold(strings.TrimSpace(token.Type), "plugin") {
			fallbackInstructions = append(fallbackInstructions, strings.TrimSpace(expansion))
		} else {
			fallbackReferences = append(fallbackReferences, strings.TrimSpace(expansion))
		}
	}
	if !hasModelExpansion {
		// Explicit Skill identity enters the host-generated system context. The
		// model-facing user message normally remains byte-for-byte the accepted
		// input. A token-only Skill turn still needs a non-empty provider message;
		// the durable/display user message remains empty and the Skill body stays
		// in host-owned system context.
		if strings.TrimSpace(input) == "" {
			for _, token := range tokens {
				if strings.EqualFold(strings.TrimSpace(token.Type), "skill") {
					return "Follow the explicitly selected Skill instructions for this turn."
				}
			}
		}
		return input
	}

	prefix := visiblePrefix.String()
	prefixCandidates := []string{
		prefix,
		strings.TrimRightFunc(prefix, unicode.IsSpace),
		strings.TrimLeftFunc(prefix, unicode.IsSpace),
		strings.TrimSpace(prefix),
	}
	tail := ""
	matched := false
	for _, candidate := range prefixCandidates {
		if strings.HasPrefix(input, candidate) {
			tail = strings.TrimPrefix(input, candidate)
			matched = true
			break
		}
	}
	if !matched {
		// A malformed or older client must not make validated capabilities or
		// references vanish from the model input.
		fallback := append([]string{}, fallbackInstructions...)
		fallback = append(fallback, input)
		fallback = append(fallback, fallbackReferences...)
		return strings.TrimSpace(strings.Join(fallback, "\n"))
	}

	effective := strings.Builder{}
	for _, token := range tokens {
		effective.WriteString(token.LeadingText)
		if expansion, expands := composerTokenModelExpansion(token); expands {
			effective.WriteString(expansion)
		}
	}
	effective.WriteString(tail)
	return strings.TrimSpace(effective.String())
}

func validateComposerCapabilityTokens(tokens []ComposerToken) error {
	for _, token := range tokens {
		if !strings.EqualFold(strings.TrimSpace(token.Type), "capability") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(token.Payload.CapabilityID), "computer-use") {
			return fmt.Errorf(
				"%w: composer capability token %q is unsupported",
				ErrInvalidInput,
				token.ID,
			)
		}
	}
	return nil
}

func validateComposerPluginTokens(tokens []ComposerToken, validator func(name, digest string) error) error {
	for _, token := range tokens {
		if !strings.EqualFold(strings.TrimSpace(token.Type), "plugin") {
			continue
		}
		name := strings.TrimSpace(token.Payload.PluginName)
		digest := strings.TrimSpace(token.Payload.PluginDigest)
		if name == "" || digest == "" {
			return fmt.Errorf("%w: composer Plugin token %q identity is incomplete", ErrInvalidInput, token.ID)
		}
		if validator == nil {
			return fmt.Errorf("%w: composer Plugin validation is unavailable", ErrInvalidInput)
		}
		if err := validator(name, digest); err != nil {
			return fmt.Errorf("%w: composer Plugin %q is unavailable: %v", ErrInvalidInput, name, err)
		}
	}
	return nil
}

func validateComposerFileTokens(tokens []ComposerToken, workspace WorkspaceConfig) error {
	for _, token := range tokens {
		if !strings.EqualFold(strings.TrimSpace(token.Type), "file") {
			continue
		}
		if err := validateComposerFileToken(token, workspace); err != nil {
			return fmt.Errorf("%w: composer file token %q: %v", ErrInvalidInput, token.ID, err)
		}
	}
	return nil
}

func validateComposerFileToken(token ComposerToken, workspace WorkspaceConfig) error {
	target := strings.TrimSpace(token.Payload.Path)
	folder := strings.TrimSpace(token.Payload.FolderPath)
	relative := strings.TrimSpace(token.Payload.RelativePath)
	kind := strings.ToLower(strings.TrimSpace(token.Payload.EntryKind))
	if target == "" || folder == "" || relative == "" {
		return errors.New("path identity is incomplete")
	}
	if !filepath.IsAbs(target) || !filepath.IsAbs(folder) {
		return errors.New("paths must be absolute")
	}
	target = filepath.Clean(target)
	folder = filepath.Clean(folder)
	if kind != "file" && kind != "directory" {
		return errors.New("entry kind is invalid")
	}
	if strings.TrimSpace(token.Payload.ProjectID) != strings.TrimSpace(workspace.ProjectID) {
		return errors.New("project identity no longer matches the run")
	}
	cleanRelative := filepath.Clean(relative)
	if filepath.IsAbs(relative) || cleanRelative == "." || cleanRelative == ".." {
		return errors.New("relative path is invalid")
	}
	if filepath.Clean(filepath.Join(folder, cleanRelative)) != target || !tool.PathWithinRoot(target, folder) {
		return errors.New("path is outside its attached folder")
	}

	readableRoot := ""
	for _, root := range workspace.ReadableRoots {
		if tool.PathWithinRoot(folder, root) && tool.PathWithinRoot(root, folder) {
			readableRoot = filepath.Clean(root)
			break
		}
	}
	if readableRoot == "" {
		return errors.New("folder is not an active readable root")
	}

	info, err := os.Lstat(target)
	if err != nil {
		return errors.New("entry is unavailable")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links cannot be added")
	}
	if (kind == "directory") != info.IsDir() {
		return errors.New("entry kind no longer matches")
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return errors.New("entry real path is unavailable")
	}
	realRoot, err := filepath.EvalSymlinks(readableRoot)
	if err != nil || !tool.PathWithinRoot(realTarget, realRoot) {
		return errors.New("entry real path is outside the readable root")
	}
	return nil
}

func skillComposerTokens(tokens []ComposerToken) []ComposerToken {
	result := make([]ComposerToken, 0, len(tokens))
	for _, token := range tokens {
		if strings.EqualFold(strings.TrimSpace(token.Type), "skill") {
			result = append(result, token)
		}
	}
	return result
}

func threadLabelForInputAndTokens(input string, attachments []RunInputAttachment, tokens []ComposerToken) string {
	if strings.TrimSpace(input) != "" || len(attachments) > 0 {
		return threadLabelForInput(input, attachments)
	}
	for _, token := range tokens {
		if label := trimThreadLabel(strings.TrimSpace(token.Label)); label != "" {
			return label
		}
		if label := trimThreadLabel(strings.TrimSpace(firstNonEmpty(token.Payload.SkillID, token.ID))); label != "" {
			return label
		}
	}
	return threadLabelForInput(input, attachments)
}

func restoreAgentDisplayInput(agent *agentcore.Agent, effectiveInput string, displayInput string) {
	if agent == nil || effectiveInput == displayInput {
		return
	}
	messages := agent.GetConversation().Context.GetMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleUser && messages[i].Content == effectiveInput {
			messages[i].Content = displayInput
			agent.GetConversation().Context.SetMessages(messages)
			return
		}
	}
}

func displayConversationMessagesForRun(
	run Run,
	messages []convcontext.Message,
) []convcontext.Message {
	effectiveInput := effectiveInputForComposerTokens(run.Input, run.ComposerTokens)
	if effectiveInput == run.Input {
		return messages
	}
	displayMessages := append([]convcontext.Message(nil), messages...)
	for i := len(displayMessages) - 1; i >= 0; i-- {
		if displayMessages[i].Role == convcontext.RoleUser &&
			displayMessages[i].Content == effectiveInput {
			displayMessages[i].Content = run.Input
			break
		}
	}
	return displayMessages
}

func (s *RunService) flushAgentTranscript(session *Session, agent *agentcore.Agent) error {
	if session == nil || agent == nil {
		return nil
	}
	run := session.Snapshot()
	snapshot, ok := s.threadSnapshot(run.ThreadID)
	if !ok {
		return nil
	}
	var firstErr error
	displayMessages := displayConversationMessagesForRun(
		run,
		agent.GetConversation().Context.GetMessages(),
	)
	if err := s.threadStore.AppendConversation(snapshot, run, displayMessages); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.threadStore.AppendRunEvents(snapshot, run, session.History(0)); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *RunService) clearActiveRun(threadID, runID string) {
	if threadID == "" || runID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	thread, ok := s.threads[threadID]
	if !ok || thread.activeRunID != runID {
		return
	}
	thread.activeRunID = ""
	thread.updatedAt = time.Now().UTC()
}

func (s *RunService) threadSnapshot(threadID string) (ThreadSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok {
		return ThreadSnapshot{}, false
	}
	return thread.snapshot(), true
}

func (s *RunService) session(runID string) (*Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrRunNotFound
	}
	return session, nil
}

// ComputerRuntime returns the run-bound Computer runtime for a run, if any.
func (s *RunService) ComputerRuntime(runID string) (*computer.Runtime, error) {
	session, err := s.session(runID)
	if err != nil {
		return nil, err
	}
	if cr := session.ComputerRuntime(); cr != nil {
		return cr, nil
	}
	return nil, fmt.Errorf("run %s has no computer runtime", runID)
}

var runtimeIDCounter uint64

func newRuntimeID(prefix string) string {
	seq := atomic.AddUint64(&runtimeIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}

func (t *Thread) snapshot() ThreadSnapshot {
	return ThreadSnapshot{
		ThreadID:                 t.threadID,
		ProjectID:                t.projectID,
		Label:                    t.label,
		TitleSource:              t.titleSource,
		TitleStatus:              t.titleStatus,
		TitleUpdatedAt:           t.titleUpdatedAt,
		TitleSeedRunID:           t.titleSeedRunID,
		RunID:                    t.latestRunID,
		ActiveRunID:              t.activeRunID,
		Workspace:                t.workspace,
		CreatedAt:                t.createdAt,
		UpdatedAt:                t.updatedAt,
		Archived:                 t.archived,
		ArchivedAt:               t.archivedAt,
		Source:                   t.source,
		ClientName:               t.clientName,
		ClientVersion:            t.clientVersion,
		ACPSessionID:             t.acpSessionID,
		SelectedModelProfileID:   t.selectedModelProfileID,
		ReasoningEffortByProfile: cloneStringMap(t.reasoningEffortByProfile),
	}
}

func (s *RunService) pruneInactiveThreadsLocked() {
	if len(s.threads) <= maxRetainedThreads {
		return
	}

	inactive := make([]*Thread, 0, len(s.threads))
	for _, thread := range s.threads {
		if thread.activeRunID == "" {
			inactive = append(inactive, thread)
		}
	}
	sort.Slice(inactive, func(i, j int) bool {
		return inactive[i].updatedAt.Before(inactive[j].updatedAt)
	})
	for len(s.threads) > maxRetainedThreads && len(inactive) > 0 {
		thread := inactive[0]
		inactive = inactive[1:]
		delete(s.threads, thread.threadID)
	}
}

func sameWorkspaceRoot(a, b WorkspaceConfig) bool {
	if a.WorkspaceRoot != b.WorkspaceRoot {
		return false
	}
	return true
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func sameTaskScope(a, b WorkspaceConfig) bool {
	if !sameWorkspaceRoot(a, b) {
		return false
	}
	// The project name reaches the model through the system prompt, so a rename
	// has to rebuild the agent instead of leaving the old name in place.
	if a.ProjectName != b.ProjectName {
		return false
	}
	if !sameStringSet(a.ReadableRoots, b.ReadableRoots) {
		return false
	}
	if !sameStringSet(a.WritableRoots, b.WritableRoots) {
		return false
	}
	if !sameStringSet(a.OutputRoots, b.OutputRoots) {
		return false
	}
	if !sameStringSet(a.ProtectedRoots, b.ProtectedRoots) {
		return false
	}
	if !sameStringSet(a.ProtectedRootExceptions, b.ProtectedRootExceptions) {
		return false
	}
	if a.TemporaryWorkRoot != b.TemporaryWorkRoot {
		return false
	}
	if !sameScopedFiles(a.AttachedFiles, b.AttachedFiles) {
		return false
	}
	return reflect.DeepEqual(a.ExecutionProfile, b.ExecutionProfile)
}

func sameScopedFiles(a, b []tool.ScopedFile) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[value.Path]++
	}
	for _, value := range b {
		if seen[value.Path] == 0 {
			return false
		}
		seen[value.Path]--
	}
	return true
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[value]++
	}
	for _, value := range b {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

type sessionEventSink struct {
	session *Session
}

func (s sessionEventSink) Emit(event agentcore.AwakenEvent) {
	if s.session == nil || s.session.CancelRequested() {
		return
	}
	s.session.RecordAgentEvent(event)
}

func (s sessionEventSink) AwaitUserQuestion(ctx context.Context, request agentcore.AwakenEvent) (agentcore.UserQuestionResolution, error) {
	if s.session == nil {
		return agentcore.UserQuestionResolution{}, ErrRunNotFound
	}
	return s.session.AwaitUserQuestion(ctx, request)
}

func (s sessionEventSink) AwaitExternalInteraction(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ExternalInteractionResolution, error) {
	if s.session == nil {
		return "", ErrRunNotFound
	}
	return s.session.AwaitExternalInteraction(ctx, request)
}

// scopeProjectID prefers the project identity the caller supplied. Deriving it
// from the workspace root only holds while a project owns exactly one folder, so
// it remains a fallback for callers that predate the field.
func scopeProjectID(workspace WorkspaceConfig) string {
	if id := strings.TrimSpace(workspace.ProjectID); id != "" {
		return id
	}
	// A managed conversation is intentionally not a project. Its thread ID owns
	// the workspace directory, while ProjectID remains empty in durable metadata
	// so the Renderer can keep it in the independent conversation section.
	if workspace.SessionWorkspace {
		return ""
	}
	return stableProjectID(workspace.WorkspaceRoot)
}

func normalizeWorkspace(ws WorkspaceConfig) (WorkspaceConfig, error) {
	return normalizeWorkspaceForThread(ws, "")
}

func normalizeWorkspaceForThread(ws WorkspaceConfig, threadID string) (WorkspaceConfig, error) {
	root := strings.TrimSpace(ws.WorkspaceRoot)
	projectID := strings.TrimSpace(ws.ProjectID)
	sessionWorkspace := ""
	if root == "" {
		// Folder-less projects are keyed by project; project-less conversations
		// are keyed by thread. Both get a stable cwd and output directory under
		// the business-data root instead of borrowing Core's process directory.
		ownerID := projectID
		if ownerID == "" {
			ownerID = strings.TrimSpace(threadID)
		}
		if ownerID == "" {
			return WorkspaceConfig{}, fmt.Errorf("%w: workspace_root, project_id, or thread_id is required", ErrInvalidInput)
		}
		if !validSessionWorkspaceOwnerID(ownerID) {
			return WorkspaceConfig{}, fmt.Errorf("%w: managed workspace owner id is invalid", ErrInvalidInput)
		}
		home := desktopdata.AwakenHomeRoot()
		if home == "" {
			return WorkspaceConfig{}, fmt.Errorf("%w: a session workspace needs a business-data root", ErrInvalidInput)
		}
		root = desktopdata.SessionWorkspacePath(home, ownerID)
		sessionWorkspace = desktopdata.SessionOutputPath(home, ownerID)
	}
	if !filepath.IsAbs(root) {
		return WorkspaceConfig{}, fmt.Errorf("%w: workspace_root must be an absolute path", ErrInvalidInput)
	}
	root = filepath.Clean(root)
	if sessionWorkspace != "" {
		if err := os.MkdirAll(sessionWorkspace, 0o755); err != nil {
			return WorkspaceConfig{}, fmt.Errorf("prepare session workspace: %w", err)
		}
	}

	// Normalize the client-supplied ExecutionProfile against the server
	// allowlist. NormalizeClientProfile maps nil, tampered, and
	// server-only profile requests onto a known built-in profile value
	// and logs a warning whenever it has to fall back. The pointer we
	// store always points at a fresh built-in value, never at the
	// caller-supplied struct, so clients cannot inject tampered fields.
	normalizedProfile := profile.NormalizeClientProfile(ws.ExecutionProfile)
	readableRoots := normalizeTaskScopeRoots(ws.ReadableRoots, root)
	writableRoots := normalizeTaskScopeRoots(ws.WritableRoots, root)
	outputRoots := normalizeTaskScopeRoots(ws.OutputRoots, root)
	if sessionWorkspace != "" && len(ws.OutputRoots) == 0 {
		// Produced files land in the session's output directory rather than at
		// the top of the session workspace, so what the agent made stays
		// separable from anything else the session keeps.
		outputRoots = []string{filepath.Clean(sessionWorkspace)}
	}
	attachedFiles := normalizeTaskScopeFiles(ws.AttachedFiles, root)
	scopeID := buildTaskScopeID(root, readableRoots, writableRoots, outputRoots, attachedFiles)
	scopeRoots := dedupeTaskScopeRoots(root, readableRoots, writableRoots, outputRoots)
	protectedRoots := buildProtectedRoots(root, scopeRoots)
	temporaryWorkRoot := filepath.Join(os.TempDir(), "awaken-task-scopes", scopeID)
	return WorkspaceConfig{
		ScopeID:                 scopeID,
		ProjectID:               projectID,
		ProjectName:             strings.TrimSpace(ws.ProjectName),
		WorkspaceRoot:           root,
		ReadableRoots:           readableRoots,
		WritableRoots:           writableRoots,
		OutputRoots:             outputRoots,
		AttachedFiles:           attachedFiles,
		ProtectedRoots:          protectedRoots,
		ProtectedRootExceptions: buildProtectedRootExceptions(scopeRoots),
		TemporaryWorkRoot:       temporaryWorkRoot,
		SessionWorkspace:        sessionWorkspace != "",
		ExecutionProfile:        &normalizedProfile,
	}, nil
}

func validSessionWorkspaceOwnerID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// dedupeTaskScopeRoots collects every directory this scope may touch. Boundary
// rules that used to be derived from the single workspace root are derived from
// this set instead, so a project holding several folders is covered.
func dedupeTaskScopeRoots(workspaceRoot string, groups ...[]string) []string {
	roots := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) {
			return
		}
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		roots = append(roots, value)
	}
	add(workspaceRoot)
	for _, group := range groups {
		for _, value := range group {
			add(value)
		}
	}
	return roots
}

// buildProtectedRootExceptions releases the session workspace subtree, and only
// when this scope actually runs inside it. A scope made of user folders never
// receives a carve-out, so the business-data root stays fully protected by
// default.
func buildProtectedRootExceptions(scopeRoots []string) []string {
	home := desktopdata.AwakenHomeRoot()
	if home == "" {
		return nil
	}
	sessionsRoot := filepath.Clean(desktopdata.SessionsRoot(home))
	for _, root := range scopeRoots {
		if tool.PathWithinRoot(root, sessionsRoot) {
			return []string{sessionsRoot}
		}
	}
	return nil
}

func normalizeTaskScopeRoots(values []string, workspaceRoot string) []string {
	if len(values) == 0 {
		return []string{workspaceRoot}
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(workspaceRoot, value)
		}
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{workspaceRoot}
	}
	return normalized
}

func normalizeTaskScopeFiles(values []tool.ScopedFile, workspaceRoot string) []tool.ScopedFile {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]tool.ScopedFile, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value.Path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspaceRoot, path)
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, tool.ScopedFile{
			Path:        path,
			DisplayName: strings.TrimSpace(value.DisplayName),
		})
	}
	return normalized
}

func buildTaskScopeID(
	workspaceRoot string,
	readableRoots []string,
	writableRoots []string,
	outputRoots []string,
	attachedFiles []tool.ScopedFile,
) string {
	parts := []string{workspaceRoot}
	parts = append(parts, readableRoots...)
	parts = append(parts, writableRoots...)
	parts = append(parts, outputRoots...)
	for _, file := range attachedFiles {
		parts = append(parts, file.Path)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "scope_" + hex.EncodeToString(sum[:8])
}

// buildProtectedRoots generates the per-folder protections for every root of the
// scope. Generating them only for the primary folder would leave a secondary
// repository's .git and .awaken directories writable.
func buildProtectedRoots(workspaceRoot string, scopeRoots []string) []string {
	protected := make([]string, 0, len(scopeRoots)*4+24)
	for _, root := range scopeRoots {
		protected = append(protected,
			filepath.Join(root, ".git"),
			filepath.Join(root, ".awaken"),
			filepath.Join(root, ".vscode"),
			filepath.Join(root, ".idea"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		protected = append(protected,
			filepath.Join(home, ".awaken"),
			filepath.Join(home, ".trae-cn"),
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".zprofile"),
			filepath.Join(home, ".zlogin"),
			filepath.Join(home, ".zlogout"),
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".bash_profile"),
			filepath.Join(home, ".bash_logout"),
			filepath.Join(home, ".profile"),
			filepath.Join(home, ".config", "fish", "config.fish"),
			filepath.Join(home, ".vscode"),
			filepath.Join(home, ".cursor"),
			filepath.Join(home, ".idea"),
		)
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		protected = append(protected,
			filepath.Join(configDir, "awaken"),
			filepath.Join(configDir, "Awaken"),
			filepath.Join(configDir, "Code"),
			filepath.Join(configDir, "Cursor"),
		)
	}
	return normalizeTaskScopeRoots(protected, workspaceRoot)
}

func mergeTaskScopeAttachments(scope WorkspaceConfig, attachments []RunInputAttachment) WorkspaceConfig {
	if len(attachments) == 0 {
		return scope
	}
	merged := append([]tool.ScopedFile(nil), scope.AttachedFiles...)
	seen := make(map[string]struct{}, len(merged))
	for _, file := range merged {
		seen[file.Path] = struct{}{}
	}
	for _, attachment := range attachments {
		if attachment.Kind != RunInputAttachmentKindFile {
			continue
		}
		path := strings.TrimSpace(attachment.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		merged = append(merged, tool.ScopedFile{
			Path:        path,
			DisplayName: strings.TrimSpace(attachment.DisplayName),
		})
	}
	scope.AttachedFiles = normalizeTaskScopeFiles(merged, scope.WorkspaceRoot)
	scope.ScopeID = buildTaskScopeID(scope.WorkspaceRoot, scope.ReadableRoots, scope.WritableRoots, scope.OutputRoots, scope.AttachedFiles)
	scope.TemporaryWorkRoot = filepath.Join(os.TempDir(), "awaken-task-scopes", scope.ScopeID)
	return scope
}
