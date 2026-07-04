# Slice 12：Desktop runtime 生命周期硬化 Spec

## Why

Slice 1 已经把 Desktop 收敛为唯一启动入口。本切片只记录生命周期硬化边界：Electron main 管理 `awaken-core` 子进程，renderer 不展示进程来源、端口、日志或错误细节。

## What Changes

- Build：构建 `awaken-core` binary 到 `apps/desktop/resources/bin/<platform-arch>/awaken-core`，供 Desktop 启动。
- Electron main：`RuntimeSupervisor` 负责启动/停止子进程、分配本地端口、等待 `/health`、收集内部日志、检测异常退出。
- Runtime：`llm.LoadConfig` 支持 `AWAKEN_CONFIG_DIR`；`cmd/awaken-core` 支持 `AWAKEN_DEFAULT_WORKSPACE`。
- Preload / shared types：只向 renderer 暴露净化后的 supervisor 状态。
- UI：启动失败或崩溃时显示 `Awaken needs restart`。
- Safety：runtime policy 仍在 Go runtime/tool 层；Electron main 只管理进程生命周期。

## Requirements

### Requirement: Electron main 管理子进程

The system SHALL spawn `awaken-core` on app startup, wait for `/health`, and stop the child process on app quit.

#### Scenario: Start runtime on launch

- **WHEN** the Electron app starts
- **THEN** main starts `awaken-core` with `AWAKEN_CORE_ADDR=127.0.0.1:<random>`
- **AND** waits until `GET /health` returns success before renderer is usable

#### Scenario: Stop runtime on quit

- **WHEN** the user quits the app
- **THEN** main terminates the child process
- **AND** no Desktop-started `awaken-core` process remains

### Requirement: 端口分配避免冲突

The system SHALL allocate an available localhost port for the runtime and retry startup with a new port when the first startup attempt fails.

### Requirement: 配置目录

The system SHALL store runtime configuration under `app.getPath("userData")/config` and pass that path to `awaken-core` via `AWAKEN_CONFIG_DIR`.

### Requirement: 用户级失败状态

The system SHALL surface startup failure or crash as `Awaken needs restart` without exposing process details in the renderer.

## Removed Requirements

- Renderer restart control.
- Renderer-visible process logs.
- Renderer-visible runtime URL, process source, exit code, or spawn error.
