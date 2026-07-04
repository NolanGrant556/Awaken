# Slice 2：选择 Workspace，并启动受限 Run Spec

## Why

用户需要让 Awaken 的项目文件工具被限制在指定 workspace 内，同时保留本地 `shell` 的只读诊断能力。本切片把 workspace 注入 run 生命周期，让 `read_file`、`list_files`、`grep_search` 受 workspace 边界约束，为后续审批、artifact 和文件修改建立可信基础。

## What Changes

- UI：在 Desktop 顶部或侧栏增加 workspace picker，展示当前 workspace，workspace 缺失/无效时给出提示。
- Bridge/API：将 `createRun(input)` 升级为 `startRun(request)`，request 包含 `input`、`workspace`（`workspace_root`、`writable_roots`）与 `client` 信息。
- Runtime：`RunService.StartRun` 接收 `StartRunRequest`，并将 workspace 注入 `AgentConfig.Workspace`。
- Tools：`read_file`、`list_files`、`grep_search` 接入 `PermissionContext`，相对路径基于 workspace root 解析，越界请求被拒绝。
- Safety：`writable_roots` v1 默认等于 `[workspace_root]`，且所有写入路径必须先归一化再判断边界。
- Safety：`shell` 只读命令允许按用户本机权限读取 workspace 外信息；写入、删除、移动、网络或复杂 shell 操作仍需审批或拒绝。

## Impact

- 受影响能力：run 启动协议、workspace 文件工具权限、workspace 写入边界安全。
- 受影响代码：
  - `apps/desktop/src/renderer/App.tsx`
  - `apps/desktop/src/shared/runtime.ts`
  - `apps/desktop/src/main/index.ts`
  - `apps/desktop/src/preload/index.ts`
  - `apps/desktop/src/renderer/runtime/client.ts`
  - `src/runtime/run_service.go`
  - `src/runtime/session.go`
  - `src/agentcore/agent.go`
  - `src/agentcore/runner.go`
  - `src/agentcore/tool_runtime.go`
  - `src/server/router.go`
  - `src/server/handlers.go`
  - `src/tools/file_read/file_read.go`
  - `src/tools/list_files/list_files.go`
  - `src/tools/grep_search/grep_search.go`

## ADDED Requirements

### Requirement: Workspace 选择 UI

The system SHALL provide a workspace picker in the Desktop UI.

#### Scenario: Workspace selected

- **WHEN** the user enters or selects a valid absolute directory path in the workspace picker
- **THEN** the UI displays the current workspace path
- **AND** the workspace is used for subsequent runs

#### Scenario: Workspace missing or invalid

- **WHEN** the workspace path is empty or not an absolute path
- **THEN** the UI shows a visible warning such as "Workspace is required"
- **AND** the user cannot start a run until a valid workspace is provided

### Requirement: Run 启动协议升级

The system SHALL change the run creation API from `createRun(input)` to `startRun(request)`.

#### Scenario: Valid start request

- **WHEN** the renderer calls `startRun({ input, workspace, client })`
- **THEN** the runtime creates a run bound to the provided `workspace_root`
- **AND** the run uses `writable_roots` from the request, defaulting to `[workspace_root]` if omitted

#### Scenario: Invalid workspace

- **WHEN** `workspace_root` is empty or not an absolute path
- **THEN** the runtime returns `INVALID_INPUT` without creating a run

### Requirement: Workspace 注入 Agent Core

The system SHALL inject the request workspace into `AgentConfig.Workspace`.

#### Scenario: Run starts with workspace

- **WHEN** a run is started with `workspace_root=/Users/nolangrant/Awaken`
- **THEN** `AgentConfig.Workspace.Root` equals `/Users/nolangrant/Awaken`
- **AND** `AgentConfig.Workspace.WritableRoots` equals `[/Users/nolangrant/Awaken]`
- **AND** the agent no longer relies solely on the process current working directory

### Requirement: Workspace 文件工具受 Workspace 约束

The system SHALL ensure `read_file`, `list_files`, and `grep_search` respect the workspace boundary.

#### Scenario: Read inside workspace

- **WHEN** the model requests `read_file` with a path inside the workspace
- **THEN** the tool returns the file content

#### Scenario: Read outside workspace

- **WHEN** the model requests `read_file` with `/etc/hosts`
- **THEN** the tool returns a permission-denied error
- **AND** the file content is not silently returned

#### Scenario: List outside workspace

- **WHEN** the model requests `list_files` with `~/Desktop`
- **THEN** the tool returns a permission-denied error

#### Scenario: Relative path resolution

- **WHEN** the model requests `read_file` with `README.md`
- **THEN** the tool resolves it relative to the workspace root
- **AND** the tool result uses paths relative to the workspace root

### Requirement: Shell 只读能力与 Workspace 文件工具分层

The system SHALL treat local read-only `shell` commands as a separate local capability, not as the same policy surface as `read_file`, `list_files`, or `grep_search`.

#### Scenario: Shell read-only access outside workspace

- **WHEN** the model uses `shell` to run a read-only command such as `wc -l /etc/hosts`
- **THEN** the command may execute according to the user's local machine permissions
- **AND** this does not fail Slice 2 workspace file-tool acceptance

#### Scenario: Shell mutation or high-risk operation

- **WHEN** the model uses `shell` for mutation, network access, repository state changes, redirection, pipelines, or other complex operations
- **THEN** the command must enter explicit approval or be rejected by runtime/tool policy

### Requirement: Writable roots 默认策略

The system SHALL default `writable_roots` to `[workspace_root]` and ensure they stay within the workspace.

#### Scenario: Writable root omitted

- **WHEN** the start request omits `writable_roots`
- **THEN** the runtime uses `[workspace_root]` as the writable roots

#### Scenario: Writable root outside workspace

- **WHEN** a writable root is outside the workspace root
- **THEN** the runtime normalizes it and rejects or clamps it to within the workspace

## MODIFIED Requirements

### Requirement: Existing run creation

**Reason**: Slice 2 requires workspace context for every run.

**Migration**: All callers of `createRun(input)` must migrate to `startRun({ input, workspace: { workspace_root, writable_roots }, client: { session_id, request_id, entrypoint } })`. The v1 Desktop is the only caller and will be updated in this slice.

## REMOVED Requirements

无
