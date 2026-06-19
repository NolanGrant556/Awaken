# Slice 9：断线恢复 Spec

## Why

当 SSE stream 断开或 Desktop renderer 刷新后，用户当前 run 状态完全丢失——UI 无法找回正在运行的 run，也无法从断点继续接收事件。本切片为事件流增加 seq 序号、`after_seq` 增量回放、单 run 状态查询，以及 UI 自动重连与恢复逻辑，使用户在断线或刷新后能无缝恢复 run 状态。

## What Changes

- Runtime：`EventBus.Append` 为每个事件分配单调递增 `Seq`；`EventBus.Subscribe` 支持按 `afterSeq` 过滤历史；`RunService` 暴露公开 `GetRun(runID)` 方法。
- API：新增 `GET /v1/runs/{run_id}` 返回 run 快照；`GET /v1/runs/{run_id}/events` 支持 `after_seq` query 参数。
- Protocol：`runtime.Event`、`agui.Event`、TS `AguiEvent` 新增 `seq` 字段；SSE 帧输出 `id:` 行。
- UI：持久化 `runId` 到 localStorage；挂载时恢复 run 状态（fetch run + after_seq 订阅）；断线自动重连带 last seq；reducer 全量 replay 前先 reset 状态。
- 测试：after_seq replay + live tail happy path；断线不白屏 failure path。

## Impact

- 受影响能力：事件流订阅、run 状态恢复、UI 断线重连。
- 受影响代码：
  - `src/runtime/event_bus.go`（seq 分配 + afterSeq 过滤）
  - `src/runtime/session.go`（Event.Seq 字段）
  - `src/runtime/run_service.go`（GetRun 公开方法 + Subscribe afterSeq 参数）
  - `src/server/handlers.go`（GET /v1/runs/{run_id} + after_seq 解析）
  - `src/server/router.go`（新路由）
  - `src/protocol/agui/events.go`（agui.Event.Seq 字段）
  - `src/protocol/agui/encoder.go`（SSE id: 行）
  - `src/protocol/agui/mapper.go`（seq 映射）
  - `apps/desktop/src/shared/agui.ts`（AguiEvent.seq）
  - `apps/desktop/src/shared/runtime.ts`（RuntimeClient.getRun）
  - `apps/desktop/src/renderer/runtime/client.ts`（getRun + 重连逻辑）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（replay reset）
  - `apps/desktop/src/renderer/App.tsx`（恢复逻辑 + localStorage）
  - `apps/desktop/src/main/index.ts`（IPC getRun + 重连）

## ADDED Requirements

### Requirement: 事件携带单调递增 seq

The system SHALL assign a monotonically increasing sequence number to each event emitted by the runtime.

#### Scenario: Events have seq

- **WHEN** the runtime emits events
- **THEN** each event has a `seq` field that is a non-negative integer
- **AND** seq values are strictly increasing within a run
- **AND** the seq is present in the AG-UI protocol event and SSE frame

### Requirement: after_seq 增量回放

The system SHALL support replaying events from a given sequence number.

#### Scenario: Replay from after_seq

- **WHEN** a client subscribes to `GET /v1/runs/{run_id}/events?after_seq=n`
- **THEN** the server replays all events with seq > n from history
- **AND** then continues with live tail for new events
- **AND** no events with seq <= n are included

#### Scenario: after_seq omitted

- **WHEN** no `after_seq` query parameter is provided
- **THEN** the server replays all history events from seq 0
- **AND** continues with live tail

### Requirement: 单 run 状态查询

The system SHALL provide an endpoint to query a single run's current state.

#### Scenario: Get run state

- **WHEN** `GET /v1/runs/{run_id}` is called
- **THEN** the server returns the current run snapshot (status, error, timestamps, etc.)
- **AND** if the run does not exist, returns 404 with a structured error

### Requirement: SSE 帧包含 id 行

The system SHALL include an `id:` field in SSE frames for browser-native reconnection support.

#### Scenario: SSE frame format

- **WHEN** the server writes an SSE event
- **THEN** the frame includes `id: <seq>\n` before the `data:` line
- **AND** the browser EventSource can use Last-Event-ID for automatic reconnection

### Requirement: UI 断线自动重连

The system SHALL automatically reconnect to the event stream after a disconnection, resuming from the last received seq.

#### Scenario: Auto-reconnect with after_seq

- **WHEN** the SSE stream disconnects unexpectedly (not due to terminal event)
- **THEN** the UI automatically reconnects with `after_seq` set to the last received seq
- **AND** replayed events do not duplicate existing state
- **AND** the UI shows a "Reconnecting..." status during reconnection

#### Scenario: Reconnect after terminal event

- **WHEN** the stream closes after a terminal event (RUN_FINISHED or RUN_ERROR)
- **THEN** no reconnection is attempted

### Requirement: UI 刷新后恢复

The system SHALL recover run state after a renderer refresh.

#### Scenario: Refresh recovery

- **WHEN** the Desktop renderer refreshes while a run is in progress
- **THEN** the UI reads the persisted `runId` from localStorage
- **AND** fetches the run snapshot via `GET /v1/runs/{run_id}`
- **AND** subscribes to events with `after_seq=0` to rebuild full state
- **AND** the timeline, approvals, and artifacts are restored without duplication

#### Scenario: No run to recover

- **WHEN** the renderer refreshes and no `runId` is persisted
- **THEN** the UI shows the idle state normally

### Requirement: Reducer replay 幂等性

The system SHALL handle event replay without duplicating accumulated state.

#### Scenario: Full replay after reset

- **WHEN** the UI performs a full replay (after_seq=0) to recover state
- **THEN** the reducer resets to initial state before applying replayed events
- **AND** text deltas are accumulated correctly without duplication
- **AND** steps, approvals, and artifacts are upserted without duplication

### Requirement: 断线不白屏

The system SHALL not crash or show a blank screen when the runtime disconnects.

#### Scenario: Runtime disconnect

- **WHEN** the runtime connection fails during a run
- **THEN** the UI shows a readable recovery prompt
- **AND** the UI does not crash or show a blank screen
- **AND** the user can retry or start a new run

## MODIFIED Requirements

无

## REMOVED Requirements

无
