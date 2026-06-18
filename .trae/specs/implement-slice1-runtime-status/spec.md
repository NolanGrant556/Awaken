# Slice 1：打开 Desktop，确认 Runtime 可用 Spec

## Why

用户打开 Awaken Desktop 后，需要立即知道本地 runtime 是否可用，而不是在点击发送后才发现失败。本切片从用户可见的 runtime 状态出发，打通 Runtime 状态 API、Electron main 代理与 Renderer 展示。

## What Changes

- 在 `awaken-core` 新增 `GET /v1/runtime/info` 端点，返回 runtime 版本、API 版本、当前配置模型、`has_api_key`、默认 workspace 以及 base URL host 摘要。
- 在 Electron main 进程新增 `getRuntimeInfo()` bridge 调用，代理到 runtime 的 `/v1/runtime/info`。
- 在 preload/renderer 共享类型中新增 `RuntimeInfo` / `RuntimeInfoWire` 与 bridge 方法。
- 在 Desktop Renderer 顶部状态区显示 `Connected` / `Disconnected`、runtime 地址与基础错误提示。
- 在 App.tsx 中增加定时轮询，runtime 启动后可自动从 `Disconnected` 恢复为 `Connected`。
- 不启动 runtime 时，Desktop 不白屏、不崩溃，明确显示 disconnected 状态。

## Impact

- 受影响能力：Desktop 启动体验、runtime 健康状态展示。
- 受影响代码：
  - `src/server/router.go`
  - `src/server/handlers.go`
  - `cmd/awaken-core/main.go`
  - `apps/desktop/src/main/index.ts`
  - `apps/desktop/src/preload/index.ts`
  - `apps/desktop/src/shared/runtime.ts`
  - `apps/desktop/src/renderer/runtime/client.ts`
  - `apps/desktop/src/renderer/App.tsx`

## ADDED Requirements

### Requirement: Runtime 信息端点

The system SHALL expose a `GET /v1/runtime/info` endpoint that returns runtime metadata without secrets.

#### Scenario: Success case

- **WHEN** a client sends `GET /v1/runtime/info`
- **THEN** the response status is `200 OK`
- **AND** the response body contains `runtime_version`, `api_version`, `configured_model`, `has_api_key`, `default_workspace`, and `base_url_host`
- **AND** `has_api_key` is a boolean derived from whether the configured API key is non-empty
- **AND** no API key value, secret, or full base URL is included in the response

#### Scenario: Failure case

- **WHEN** the runtime HTTP handler fails to serialize the info
- **THEN** the response status is `500 Internal Server Error`
- **AND** the response body contains a readable error message without secrets

### Requirement: Electron main 代理 runtime 信息

The system SHALL allow the renderer to query runtime info through the existing Electron bridge.

#### Scenario: Runtime reachable

- **WHEN** the renderer invokes `window.awaken.getRuntimeInfo()`
- **AND** the runtime is reachable
- **THEN** the main process returns the normalized `RuntimeInfo` object

#### Scenario: Runtime unreachable

- **WHEN** the renderer invokes `window.awaken.getRuntimeInfo()`
- **AND** the runtime is not reachable or returns an error
- **THEN** the main process rejects with a readable error message
- **AND** the rejection does not crash the renderer

### Requirement: Renderer 顶部状态展示

The system SHALL display the runtime connection status in the Desktop top bar.

#### Scenario: Runtime disconnected

- **WHEN** Desktop starts and `getRuntimeInfo()` fails
- **THEN** the top bar shows a `Disconnected` status pill
- **AND** the runtime base URL is still visible
- **AND** a user-visible error notice explains that the runtime is unavailable

#### Scenario: Runtime connected

- **WHEN** `getRuntimeInfo()` succeeds
- **THEN** the top bar shows a `Connected` status pill
- **AND** the configured model is shown next to the status

#### Scenario: Runtime recovers

- **WHEN** the runtime was disconnected
- **AND** the user starts `go run ./cmd/awaken-core`
- **THEN** the next poll detects the runtime
- **AND** the status pill changes from `Disconnected` to `Connected`
- **AND** the error notice is removed

### Requirement: 不泄露 Secret

The system SHALL prevent runtime secrets from being exposed to the renderer or logs.

#### Scenario: Status endpoint

- **WHEN** `GET /v1/runtime/info` is called
- **THEN** the response MUST NOT contain `api_key`, `apikey`, or the full configured `base_url`

#### Scenario: Bridge error

- **WHEN** a runtime request fails
- **THEN** the error message shown to the user MUST NOT contain the API key

## MODIFIED Requirements

无

## REMOVED Requirements

无
