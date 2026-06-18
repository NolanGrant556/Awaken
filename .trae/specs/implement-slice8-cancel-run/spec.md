# Slice 8：取消 Run Spec

## Why

用户需要能安全中断正在运行的任务。当前 runtime 层的 cancel 状态机已基本完整，但 Agent Core 层缺少 cancel 时的 synthetic tool result 补齐、事件层未显式发射 cancelling/cancelled 的 `RUN_STATUS_CHANGED`、UI 层把 cancelled 误渲染为 error phase。本切片收口这些缺口，使 cancel 成为一等公民。

## What Changes

- Runtime：`RequestCancel` 发射 `RUN_STATUS_CHANGED(cancelling)`；`CompleteCancelled` 发射 `RUN_STATUS_CHANGED(cancelled)` 再发终态事件。
- Agent Core：`RunStreamWithEvents` 在每轮循环检查 `ctx.Done()`，cancel 时为 pending tool_use 注入 synthetic tool result，避免配对断裂，并早退不再执行新工具。
- Event：`RUN_STATUS_CHANGED` 显式覆盖 cancelling/cancelled 状态，UI 不再仅靠 `RUN_ERROR + code` 区分。
- UI：`RuntimePhase` 新增 `"cancelled"` 终态；`syncFromRun` 和 `RUN_STATUS_CHANGED` handler 正确区分 cancelled 与 error；`formatStatusLabel` 显示 "Cancelled"。
- 测试：Agent Core 新增 cancel 测试（synthetic tool result、不再执行新工具）；UI reducer 验证 cancelled 终态。

## Impact

- 受影响能力：Run 取消、tool_use/tool_result 配对完整性、UI 终态展示。
- 受影响代码：
  - `src/runtime/session.go`（RequestCancel / CompleteCancelled 发射状态事件）
  - `src/agentcore/runner.go`（ctx.Done 检查 + synthetic tool result）
  - `src/agentcore/runner_test.go`（新增 cancel 测试）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（cancelled phase + RUN_STATUS_CHANGED handler）
  - `apps/desktop/src/renderer/App.tsx`（formatStatusLabel 区分 cancelled）

## ADDED Requirements

### Requirement: Runtime 发射 cancelling/cancelled 状态事件

The system SHALL emit `RUN_STATUS_CHANGED` events when entering cancelling and cancelled states.

#### Scenario: RequestCancel emits cancelling status

- **WHEN** `RequestCancel` is called on a running or awaiting_approval run
- **THEN** a `RUN_STATUS_CHANGED` event is emitted with status `"cancelling"`
- **AND** the run context is cancelled

#### Scenario: CompleteCancelled emits cancelled status

- **WHEN** `CompleteCancelled` finalizes the run
- **THEN** a `RUN_STATUS_CHANGED` event is emitted with status `"cancelled"`
- **AND** the terminal `RUN_ERROR` event with `code="RUN_CANCELLED"` follows

### Requirement: Agent Core cancel 时补齐 synthetic tool result

The system SHALL inject synthetic tool results for pending tool_use when cancellation occurs, maintaining tool_use/tool_result pairing.

#### Scenario: Cancel during tool execution

- **WHEN** the run is cancelled while tool calls are pending or executing
- **THEN** synthetic tool results are generated for each unpaired tool_use
- **AND** each synthetic result has `IsError=true` and content indicating cancellation
- **AND** `ValidateToolResultPairing` passes without orphan tool_use

#### Scenario: Cancel between turns

- **WHEN** the run is cancelled between LLM turns
- **THEN** the runner exits early without making new LLM requests
- **AND** no new tool calls are executed

### Requirement: Cancel 后不再执行新工具

The system SHALL NOT execute new tools after cancellation is requested.

#### Scenario: Tools stopped after cancel

- **WHEN** cancel is requested during a multi-turn run
- **THEN** no subsequent `TOOL_CALL_START` events are emitted
- **AND** no file mutations occur after cancel

### Requirement: Approval 等待时 cancel 文件不变

The system SHALL NOT modify files when cancelling during approval wait.

#### Scenario: Cancel during awaiting_approval

- **WHEN** cancel is requested while a run is in `awaiting_approval` state
- **THEN** the approval is discarded
- **AND** no mutation is executed
- **AND** the run transitions to `cancelling` then `cancelled`

### Requirement: UI 显示 cancelled 终态

The system SHALL render cancelled as a distinct terminal state, not as an error.

#### Scenario: Desktop shows cancelled

- **WHEN** a run is cancelled
- **THEN** the UI phase becomes `"cancelled"` (not `"error"`)
- **AND** the status label shows "Cancelled"
- **AND** the timeline shows a "Run cancelled" step
- **AND** no generic error message is displayed

### Requirement: Cancel 不存在的 run 返回结构化错误

The system SHALL return a structured error when cancelling a non-existent run.

#### Scenario: Cancel missing run

- **WHEN** `POST /v1/runs/{run_id}/cancel` is called with an unknown `run_id`
- **THEN** the API returns 404 with a structured error response

## MODIFIED Requirements

无

## REMOVED Requirements

无
