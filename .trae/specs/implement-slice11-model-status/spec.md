# Slice 11：模型状态与只读模型选择 Spec

## Why

用户打开 Desktop 后，需要一眼看出当前 model/provider 是否可用，而不是等点发送后才发现缺 key 或 provider 不可达。当前 RuntimeInfo 已暴露 `configured_model` 和 `has_api_key`，但未暴露 `protocol`，且缺 key 发起 run 时只产生一条 "LLM stream call failed: ..." 的纯文本错误，没有结构化错误码，UI 也无法据此显示 "not ready"。本切片在不引入多 provider profile、UI 内编辑 key、运行时热切换的前提下，把 provider 状态展示和结构化 provider 错误打通。

## What Changes

- Runtime：`RuntimeInfo` 新增 `protocol` 字段（来源于 `llm.Config.Protocol` 解析后的值），让用户能看到当前 provider 协议。
- Runtime：`llmRuntimeInfoProvider` 构造时接收 `protocol`，并在 `GetRuntimeInfo()` 中返回。
- Error：新增结构化错误码 `PROVIDER_AUTH_MISSING`（缺 API key）和 `PROVIDER_REQUEST_FAILED`（provider 请求失败）。
- Agent Core：`runEventEmitter.runErrored` 在错误是 `llm.ErrMissingAPIKey` 时设置 `Code: "PROVIDER_AUTH_MISSING"`；是 `llm.ErrRequestFailed` 时设置 `Code: "PROVIDER_REQUEST_FAILED"`。
- UI：顶部状态区在 `has_api_key=false` 时显示 `Not ready` pill，禁用发送或提示用户配置 key；`has_api_key=true` 时显示 `Ready`。
- UI：run error 事件携带 `code` 时，错误展示区分 `PROVIDER_AUTH_MISSING`（提示配置 API key）和 `PROVIDER_REQUEST_FAILED`（提示 provider 请求失败），而非笼统的 "Run failed"。
- Config：继续读取 `config.toml` 和 `.env`，不新增配置入口。
- Safety：不把 API key 写入 renderer state、日志或文档；`protocol`、`configured_model`、`has_api_key` 均为非敏感摘要。

## Impact

- 受影响能力：模型/provider 状态展示、provider 错误可读性、run 错误结构化。
- 受影响代码：
  - `src/server/runtime_info.go`（`RuntimeInfo` 加 `protocol` 字段；`llmRuntimeInfoProvider` 加 `protocol`）
  - `cmd/awaken-core/main.go`（`NewLLMRuntimeInfoProvider` 传入解析后的 `protocol`）
  - `src/agentcore/runner.go`（`runErrored` 根据 error 类型设置 `Code`）
  - `src/agentcore/runner_test.go`（验证 `Code` 设置）
  - `apps/desktop/src/shared/runtime.ts`（`RuntimeInfoWire` / `RuntimeInfo` 加 `protocol`；`normalizeRuntimeInfo` 映射）
  - `apps/desktop/src/renderer/App.tsx`（顶部 `Ready` / `Not ready` pill；发送前检查 `hasApiKey`）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（run error 按 `code` 给出可读描述）
  - `apps/desktop/src/renderer/runtime/reducer.test.ts`（验证 `PROVIDER_AUTH_MISSING` 描述）

## ADDED Requirements

### Requirement: RuntimeInfo 暴露 protocol

The system SHALL include the resolved LLM protocol in the runtime info response so the user can see which provider protocol is active.

#### Scenario: Status shows protocol

- **WHEN** a client sends `GET /v1/runtime/info`
- **THEN** the response body contains a `protocol` field
- **AND** the value is the resolved protocol string (`openai_chat` or `anthropic_messages`)
- **AND** no API key or full base URL is included alongside the protocol

#### Scenario: Protocol derived from config

- **WHEN** `config.toml` specifies `protocol = "anthropic_messages"`
- **THEN** `RuntimeInfo.protocol` is `anthropic_messages`
- **AND** when `config.toml` omits `protocol`, the value follows `llm.resolveProtocol` auto-detection

### Requirement: 缺 key 时 UI 显示 Not ready

The system SHALL show a `Not ready` status in the Desktop top bar when the configured provider has no API key, and a `Ready` status when the key is present.

#### Scenario: Missing API key

- **WHEN** `getRuntimeInfo()` returns `has_api_key=false`
- **THEN** the top bar shows a `Not ready` pill next to the model name
- **AND** the user is informed that an API key is required before starting a run

#### Scenario: API key present

- **WHEN** `getRuntimeInfo()` returns `has_api_key=true`
- **THEN** the top bar shows a `Ready` pill
- **AND** the user can start a real run

### Requirement: 缺 key 发 run 返回结构化错误

The system SHALL emit a `RunError` event with `code=PROVIDER_AUTH_MISSING` when a run is started without a configured API key, instead of a generic error message.

#### Scenario: Run without API key

- **WHEN** a run is started and `llm.ErrMissingAPIKey` is returned by the LLM client
- **THEN** the `RunError` event's `code` field is `PROVIDER_AUTH_MISSING`
- **AND** the event `error` message explains that the API key is missing
- **AND** no API key value appears in the message

### Requirement: Provider 请求失败返回结构化错误

The system SHALL emit a `RunError` event with `code=PROVIDER_REQUEST_FAILED` when the LLM provider returns a request failure.

#### Scenario: Provider request fails

- **WHEN** a run is started with a valid API key
- **AND** the LLM provider returns a non-2xx response or transport error
- **THEN** the `RunError` event's `code` field is `PROVIDER_REQUEST_FAILED`
- **AND** the event `error` message is readable and does not contain the API key

### Requirement: UI 按 code 显示可读 provider 错误

The system SHALL render provider error codes as readable, actionable messages in the run error UI.

#### Scenario: Auth missing error display

- **WHEN** a `RunError` event with `code=PROVIDER_AUTH_MISSING` is received
- **THEN** the run error display shows a message indicating the API key is missing
- **AND** the message guides the user to configure `LLM_API_KEY` in `.env`

#### Scenario: Request failed error display

- **WHEN** a `RunError` event with `code=PROVIDER_REQUEST_FAILED` is received
- **THEN** the run error display shows a message indicating the provider request failed
- **AND** the underlying error detail is visible without leaking the API key

### Requirement: 不泄露 Secret

The system SHALL prevent API keys and full base URLs from appearing in renderer state, logs, or documentation.

#### Scenario: Runtime info response

- **WHEN** `GET /v1/runtime/info` is called
- **THEN** the response contains `has_api_key` (boolean) but never the key value
- **AND** the response contains `base_url_host` but never the full base URL

#### Scenario: Run error event

- **WHEN** a `RunError` event is emitted due to provider auth or request failure
- **THEN** the event `error` and `code` fields MUST NOT contain the API key

## MODIFIED Requirements

无

## REMOVED Requirements

无
