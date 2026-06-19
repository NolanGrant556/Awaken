# Slice 12：Desktop 管理 awaken-core Spec

## Why

打包版用户双击 Awaken 后，不应需要手动在终端启动 runtime。当前 Desktop 依赖外部 `go run ./cmd/awaken-core`，且配置从 cwd 读取，无法支撑打包分发。本切片让 Electron main 自动启动/停止 bundled `awaken-core` 子进程，分配端口，收集日志，并在 crash 后允许重启，同时把配置迁移到 Application Support，使打包版开箱即用。

## What Changes

- Build：新增 npm script 交叉编译 `awaken-core` binary 到 `apps/desktop/resources/bin/<platform>/awaken-core`（macOS 优先），并新增 electron-builder 配置把 resources 打包进 app。
- Electron main：新增 `RuntimeSupervisor` 模块，负责 spawn/stop bundled `awaken-core` 子进程，分配随机 localhost 端口（避免冲突），轮询 `/health` 等待 ready，收集 stdout/stderr 到日志缓冲，检测子进程异常退出。
- Electron main：`app.whenReady` 时启动 supervisor；`before-quit` 时优雅停止子进程；`window-all-closed`（非 macOS）时也停止。
- Electron main：新增 IPC 通道 `awaken:runtime-logs`（推送日志行）、`awaken:restart-runtime`（重启子进程）、`awaken:runtime-supervisor-status`（查询 supervisor 状态：starting/running/crashed/stopped）。
- Runtime：`llm.LoadConfig` 新增对 `AWAKEN_CONFIG_DIR` 环境变量的支持，优先从该目录读取 `.env` 和 `config.toml`；未设置时保持原有 cwd 行为（向后兼容）。
- Runtime：`cmd/awaken-core/main.go` 的 `default_workspace` 不再无条件用 `os.Getwd()`，改为优先用 `AWAKEN_DEFAULT_WORKSPACE` 环境变量，未设置时回退 cwd。Electron main 启动子进程时把 `AWAKEN_DEFAULT_WORKSPACE` 设为用户 home 或上次选择的 workspace。
- Preload：`window.awaken` 新增 `getRuntimeLogs()`、`restartRuntime()`、`getSupervisorStatus()`、`onRuntimeLogs(callback)` 方法。
- UI：顶部状态区显示 supervisor 状态（Starting / Running / Crashed / Stopped）；新增 runtime logs 面板（可折叠）；新增 Restart Runtime 按钮；crashed 时显示错误并允许重启。
- Config：Electron main 在 `app.getPath('userData')` 下创建配置目录，首次启动时若 `.env`/`config.toml` 不存在则写入默认模板（不含 key，提示用户填写），启动子进程时传入 `AWAKEN_CONFIG_DIR`。
- Safety：runtime policy 仍在 Go runtime/tool 层；Electron main 只管理进程生命周期，不实现 policy。

## Impact

- 受影响能力：打包分发、runtime 生命周期管理、配置持久化、runtime 日志可见性。
- 受影响代码：
  - `apps/desktop/src/main/index.ts`（新增 RuntimeSupervisor，改造 app 生命周期钩子）
  - `apps/desktop/src/main/runtime-supervisor.ts`（**新增**：进程管理、端口分配、健康检查、日志收集）
  - `apps/desktop/src/main/config-bootstrap.ts`（**新增**：Application Support 配置目录初始化）
  - `apps/desktop/src/preload/index.ts`（暴露 logs/restart/supervisor API）
  - `apps/desktop/src/shared/runtime.ts`（新增 supervisor 状态类型、logs 相关类型）
  - `apps/desktop/src/renderer/runtime/client.ts`（新增 supervisor/logs/restart 方法）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（supervisor 状态、logs 缓冲）
  - `apps/desktop/src/renderer/App.tsx`（supervisor 状态展示、logs 面板、restart 按钮）
  - `apps/desktop/package.json`（新增 electron-builder 依赖与配置、build:binary script）
  - `apps/desktop/electron-builder.yml`（**新增**：打包配置，extraResources 包含 binary）
  - `package.json`（根，新增 `build:binary` 聚合 script）
  - `src/llm/client.go`（`LoadConfig` 支持 `AWAKEN_CONFIG_DIR`）
  - `src/llm/llm_test.go`（验证 `AWAKEN_CONFIG_DIR` 读取）
  - `cmd/awaken-core/main.go`（`default_workspace` 支持 `AWAKEN_DEFAULT_WORKSPACE`）

## ADDED Requirements

### Requirement: 构建 bundled awaken-core binary

The system SHALL provide a build step that cross-compiles `awaken-core` into a platform-specific binary and places it under `apps/desktop/resources/bin/<platform-arch>/awaken-core` so it can be bundled into the Electron app.

#### Scenario: Build binary for macOS

- **WHEN** the developer runs `npm run build:binary` (or `npm run build:binary:mac`)
- **THEN** a binary is produced at `apps/desktop/resources/bin/darwin-arm64/awaken-core` (and/or `darwin-x64`)
- **AND** the binary is executable
- **AND** the binary respects `AWAKEN_CORE_ADDR` for listen address

#### Scenario: Binary absent in dev

- **WHEN** the binary does not exist and `AWAKEN_DESKTOP_RUNTIME_URL` is set
- **THEN** Electron main falls back to the external runtime URL (dev mode) instead of spawning a child process

### Requirement: Electron main 启动并管理 runtime 子进程

The system SHALL spawn the bundled `awaken-core` as a child process on app startup, allocate a random localhost port, wait for it to become healthy, and stop it on app quit.

#### Scenario: Start runtime on launch

- **WHEN** the Electron app starts (packaged mode, no `AWAKEN_DESKTOP_RUNTIME_URL`)
- **THEN** the main process spawns the bundled `awaken-core` with `AWAKEN_CORE_ADDR=127.0.0.1:<random>`
- **AND** polls `GET /health` until it returns 200 or a timeout is reached
- **AND** once healthy, the renderer connects to the runtime at the allocated port

#### Scenario: Stop runtime on quit

- **WHEN** the user quits the app (`before-quit`)
- **THEN** the main process gracefully terminates the child process (SIGTERM then SIGKILL after grace period)
- **AND** no `awaken-core` process remains after the app exits

#### Scenario: Runtime crash detection

- **WHEN** the `awaken-core` child process exits unexpectedly (non-zero, not triggered by app quit)
- **THEN** the supervisor transitions to `crashed` state
- **AND** the renderer is notified with the exit code and last log lines
- **AND** the user can restart the runtime via a Restart button

#### Scenario: Dev mode bypass

- **WHEN** `AWAKEN_DESKTOP_RUNTIME_URL` is set (dev mode)
- **THEN** the supervisor does NOT spawn a child process
- **AND** the renderer connects to the external runtime URL as before

### Requirement: 端口分配避免冲突

The system SHALL allocate a random available localhost port for the runtime to avoid conflicts with existing services.

#### Scenario: Random port allocation

- **WHEN** the supervisor starts the runtime
- **THEN** it binds an ephemeral port on `127.0.0.1` to discover a free port, then passes it to the child via `AWAKEN_CORE_ADDR`
- **AND** if the runtime fails to bind that port, the supervisor retries with a new port (up to a small limit)

### Requirement: Runtime 日志收集与展示

The system SHALL collect `awaken-core` stdout/stderr into a ring buffer and expose it to the renderer for display.

#### Scenario: Logs streamed to renderer

- **WHEN** the runtime writes to stdout or stderr
- **THEN** the supervisor appends the line to an in-memory ring buffer (capped, e.g. 1000 lines)
- **AND** forwards new lines to the renderer via `awaken:runtime-logs` IPC
- **AND** the renderer displays them in a collapsible logs panel

#### Scenario: Logs available after restart

- **WHEN** the user clicks Restart Runtime
- **THEN** the supervisor clears the old buffer, restarts the child, and resumes streaming new logs

### Requirement: 重启 runtime

The system SHALL allow the user to restart the `awaken-core` child process from the UI.

#### Scenario: User restarts runtime

- **WHEN** the user clicks "Restart Runtime"
- **THEN** the supervisor stops the current child (if running) and starts a new one
- **AND** the supervisor status transitions through `stopping -> starting -> running`
- **AND** the renderer reflects the new status

### Requirement: 配置迁移到 Application Support

The system SHALL store runtime configuration (`config.toml`, `.env`) under the platform's Application Support directory (`app.getPath('userData')`) instead of the project cwd.

#### Scenario: First launch config bootstrap

- **WHEN** the app starts and `AWAKEN_CONFIG_DIR` target directory has no `config.toml`
- **THEN** the main process creates the directory and writes a default `config.toml` template (non-sensitive defaults)
- **AND** if `.env` does not exist, writes a template `.env` with `LLM_API_KEY=` placeholder and a comment guiding the user
- **AND** starts the child with `AWAKEN_CONFIG_DIR=<userData>/config`

#### Scenario: Existing config preserved

- **WHEN** the app starts and `config.toml` / `.env` already exist in the config dir
- **THEN** they are NOT overwritten
- **AND** the runtime reads the existing values

### Requirement: Runtime 支持配置目录与默认 workspace 环境变量

The system SHALL allow the runtime to read configuration from a specified directory and accept an explicit default workspace.

#### Scenario: AWAKEN_CONFIG_DIR respected

- **WHEN** `awaken-core` is started with `AWAKEN_CONFIG_DIR=/path/to/config`
- **THEN** `llm.LoadConfig` reads `.env` and `config.toml` from that directory
- **AND** when `AWAKEN_CONFIG_DIR` is unset, behavior is unchanged (reads from cwd)

#### Scenario: AWAKEN_DEFAULT_WORKSPACE respected

- **WHEN** `awaken-core` is started with `AWAKEN_DEFAULT_WORKSPACE=/Users/foo/projects`
- **THEN** `RuntimeInfo.default_workspace` is `/Users/foo/projects`
- **AND** when unset, `default_workspace` falls back to cwd (existing behavior)

### Requirement: UI 展示 supervisor 状态与错误恢复

The system SHALL display the runtime supervisor status in the top bar and provide recovery actions on failure.

#### Scenario: Status shown in top bar

- **WHEN** the supervisor is in `starting`, `running`, `crashed`, or `stopped` state
- **THEN** the top bar shows the corresponding status pill
- **AND** in `crashed` state, an error notice is shown with the exit code and a Restart button

#### Scenario: Restart from crashed

- **WHEN** the supervisor is `crashed` and the user clicks Restart
- **THEN** a new child process is started
- **AND** if it becomes healthy, the status returns to `running` and the error notice is cleared

## MODIFIED Requirements

### Requirement: Electron main 代理 runtime 信息

The existing `getRuntimeInfo` bridge SHALL continue to work when the runtime is managed by the supervisor, connecting to the supervisor-allocated port instead of a hardcoded URL.

#### Scenario: Runtime info via supervisor

- **WHEN** the supervisor has started the runtime and it is healthy
- **THEN** `getRuntimeInfo()` requests against the supervisor-allocated port
- **AND** returns the same `RuntimeInfo` structure as before (including `protocol` from Slice 11)

## REMOVED Requirements

无
