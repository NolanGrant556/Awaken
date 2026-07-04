# Slice 3：普通问答 Run Spec

## Why

用户需要能发起一次不涉及工具的普通 LLM 对话，并看到稳定的 streaming 回复。本切片确保 UI、Bridge、Runtime、LLM 形成一条真实链路，且对空输入等错误有清晰处理。

## What Changes

- UI：Composer 输入框、发送按钮、streaming 回复区域、run 状态展示。
- API：`POST /v1/runs` 继续返回 Run（Slice 2 已升级协议）。
- Runtime：创建 run、调用 LLM、emit 事件、结束 run；空输入返回 `INVALID_INPUT`。
- Event：明确 `RUN_STARTED -> TEXT_MESSAGE_START -> TEXT_MESSAGE_CONTENT -> TEXT_MESSAGE_END -> RUN_FINISHED` 链路。
- Smoke：真实 LLM 输入"请只回复 pong"，最终只显示 `pong`。

## Impact

- 受影响能力：普通问答 run、streaming 渲染、空输入错误处理。
- 受影响代码：
  - `apps/desktop/src/renderer/App.tsx`
  - `apps/desktop/src/renderer/runtime/reducer.ts`
  - `src/runtime/run_service.go`
  - `src/server/handlers.go`
  - `src/agentcore/runner.go`
  - `src/agentcore/events.go`

## ADDED Requirements

### Requirement: 普通问答 UI

The system SHALL provide an input area and a streaming response area for plain Q&A runs.

#### Scenario: User submits a question

- **WHEN** the user types a non-empty question and clicks send
- **THEN** the UI shows a run status indicator
- **AND** the assistant response streams into the response area

#### Scenario: Empty input

- **WHEN** the user tries to send an empty input
- **THEN** the UI prevents the submission
- **AND** a readable error such as "Input is required" is displayed

### Requirement: Runtime 空输入错误

The system SHALL reject empty input with a structured error.

#### Scenario: Empty input submitted to runtime

- **WHEN** `POST /v1/runs` receives an empty `input`
- **THEN** the response status is `400 Bad Request`
- **AND** the response body contains error code `INVALID_INPUT`
- **AND** no run is created

### Requirement: 普通问答事件链路

The system SHALL emit a clean event stream for a plain Q&A run.

#### Scenario: Successful plain Q&A run

- **WHEN** a run completes without tool calls
- **THEN** the event stream contains `RUN_STARTED`
- **AND** `TEXT_MESSAGE_START`, `TEXT_MESSAGE_CONTENT`, `TEXT_MESSAGE_END`
- **AND** `RUN_FINISHED`
- **AND** no `TOOL_CALL_START` or `AWAKEN_APPROVAL_REQUESTED` events appear

### Requirement: 真实 LLM Smoke

The system SHALL complete a real plain Q&A run with a live LLM.

#### Scenario: Ping-pong smoke

- **WHEN** the user input is "请只回复 pong"
- **THEN** the final assistant text is exactly `pong`
- **AND** the run status is `succeeded`
- **AND** no approval or artifact is produced

## MODIFIED Requirements

无

## REMOVED Requirements

无
