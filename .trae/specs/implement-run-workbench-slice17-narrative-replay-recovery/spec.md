# Slice 17: Narrative Replay 与恢复 Spec

## Why

Run Workbench 已经能把实时事件投影为 Run Narrative，但刷新、SSE 断线重连、切换 thread 后仍需要更明确的恢复语义。Slice 17 让 replay 成为第一等路径：同一批 events 可重复投影，terminal run 可以从历史恢复 done 状态，pending approval 可以恢复 waiting 状态，非终态断线不应被误渲染为普通 run failed。

## What Changes

- Desktop reducer 明确区分 stream interruption/reconnecting 与 runtime terminal error。
- Run recovery 使用稳定 thread/run 线索恢复 prompt card，不只恢复 event stream。
- RunNarrative projector 补齐 replay idempotency 覆盖 prompt、thinking、tool/exploring、approval、artifact、answer。
- Runtime client 的 reconnect/replay 行为继续使用 `after_seq`，并补充 reducer/client 测试。
- Desktop recovery smoke 覆盖 terminal replay、pending approval replay 和 reconnect live tail。

## Impact

- 主要触及 `apps/desktop/src/renderer/runtime/reducer.ts`、`runtime/client.ts`、`App.tsx` 和 reducer/client tests。
- 可能补充 shared runtime type 字段或 reducer action payload，用于恢复 prompt/thread label。
- 不改变 Go runtime 的 event seq / after_seq 协议。
- 不做跨 runtime 重启持久化，不新增数据库。

## Requirements

### Requirement: Replay is idempotent

#### Scenario: Same history is replayed twice

- **WHEN** Desktop reduces the same sequenced event history more than once
- **THEN** prompt、thinking、tool/exploring、approval、artifact、answer items do not duplicate
- **AND** already-seen text/reasoning/tool args deltas are not appended again

### Requirement: Recovery restores prompt and narrative state

#### Scenario: Renderer recovers an existing run

- **WHEN** Desktop dispatches run recovery for a run with a known thread
- **THEN** narrative includes a prompt card restored from the best available thread label or prompt fallback
- **AND** replayed events rebuild committed and active narrative items

### Requirement: Pending approval survives replay

#### Scenario: Replay stops at approval request

- **WHEN** recovered history contains `AWAKEN_APPROVAL_REQUESTED` without a matching resolved or terminal event
- **THEN** approvals state contains the pending approval
- **AND** narrative shows an approval item with status `waiting`

### Requirement: Terminal replay restores done state

#### Scenario: Replay includes terminal answer

- **WHEN** recovered history contains answer deltas followed by `RUN_FINISHED`
- **THEN** answer is committed with status `done`
- **AND** run phase/timeline are terminal done without duplicate active items

### Requirement: Stream interruption is not ordinary run failure

#### Scenario: SSE closes before terminal event

- **WHEN** the runtime client detects a non-terminal stream interruption
- **THEN** reducer enters reconnecting/recovery state without committing a failed answer item
- **AND** successful replay/live tail can continue updating active narrative

### Requirement: after_seq live tail

#### Scenario: Client reconnects after partial history

- **WHEN** reconnect happens after receiving a sequenced event
- **THEN** the next subscription uses `after_seq` for the last seen frame
- **AND** replayed overlap remains idempotent in reducer

## Modified Requirements

- Renderer stream errors caused by transport interruption are surfaced as recovery status, not as assistant answer failures.

## Removed Requirements

- None.
