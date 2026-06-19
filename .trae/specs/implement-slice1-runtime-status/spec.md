# 统一 Desktop 启动体验：删除外部 runtime 默认路径 Spec

## Why

Awaken Desktop 的产品路径应只有一个：用户打开 Desktop，开发者运行 `npm run dev:desktop`。Desktop 自己管理本机 `awaken-core`，不再把“先手动启动 runtime 再连接 Desktop”作为正常流程。

## What Changes

- Electron main 保留内部 `RuntimeSupervisor`，启动时自动分配本地端口、启动 `awaken-core`、等待 `/health` ready。
- Renderer 不接收 runtime URL，不展示 runtime 来源、端口、进程日志、exit code 或恢复工程细节。
- 启动失败或运行中崩溃时，UI 只显示 `Awaken needs restart`，并禁用 run/workspace 操作。
- `npm run dev:desktop` 自动确保当前平台 `awaken-core` binary 存在；缺失时自动 build。
- README 和 Desktop README 的主流程只写 `npm run dev:desktop`。

## Requirements

### Requirement: Desktop 自己管理 runtime

The system SHALL start and stop `awaken-core` from Electron main for both development and app launch flows.

#### Scenario: Dev desktop starts without another terminal

- **WHEN** the developer runs `npm run dev:desktop`
- **THEN** the command ensures the current-platform `awaken-core` binary exists
- **AND** Electron main starts `awaken-core`
- **AND** renderer becomes usable after `/health` is ready

#### Scenario: App launch starts without another terminal

- **WHEN** the user opens `Awaken.app`
- **THEN** Electron main starts `awaken-core`
- **AND** the user can complete a real run from Desktop

### Requirement: 删除外部 runtime 产品分支

The system SHALL NOT expose or document a Desktop path that connects to a manually-started runtime.

#### Scenario: No Desktop override URL

- **WHEN** Desktop starts
- **THEN** it always uses the runtime managed by Electron main
- **AND** renderer does not receive a runtime URL
- **AND** docs do not instruct users to start `awaken-core` before Desktop

### Requirement: 用户级恢复提示

The system SHALL avoid blank screens and avoid exposing process details when startup or runtime lifecycle fails.

#### Scenario: Startup failure or crash

- **WHEN** `awaken-core` fails to start or exits unexpectedly
- **THEN** Desktop shows `Awaken needs restart`
- **AND** Desktop does not show runtime source, port, process logs, exit code, or spawn errors

### Requirement: 退出清理

The system SHALL clean up its child process when Desktop quits.

#### Scenario: App exits

- **WHEN** Desktop quits
- **THEN** Electron main stops its `awaken-core` child process
- **AND** no Desktop-started `awaken-core` process remains
