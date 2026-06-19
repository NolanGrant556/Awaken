# Slice 9 断线恢复修复计划

## Summary

修复 Slice 9 code review 中发现的 P0/P1 问题：AG-UI frame 级 seq 共享导致断线丢数据、terminal run 刷新恢复丢失历史、Electron `run-error` 被 `run-closed` 吞掉、reconnect 防重入缺失、recovered 后 timeline 标题错误，并补充 UI/client 层测试。

## Current State Analysis

### 问题 1：多 frame 共享 seq
- `src/protocol/agui/mapper.go` L45-51、L56-61：`ReasoningStartedEventType` → `[start, messageStart]`，`ReasoningFinishedEventType` → `[messageEnd, reasoningEnd]`，各产出 2 个 AG-UI frame。
- `src/server/handlers.go` L217-221：`mapRuntimeEvent` 给同一 runtime event 的所有 mapped frame 赋相同 `event.Seq`。
- `src/runtime/event_bus.go` L35-36：`nextSeq++` 按 runtime event 粒度分配。
- 后果：客户端收到第一条 seq=N 的 frame 后断线，`after_seq=N` 过滤 `seq > N`，第二条 seq=N 的 frame 永远丢失。

### 问题 2：terminal run 刷新恢复丢历史
- `apps/desktop/src/renderer/App.tsx` L153-157：`getRun` 返回 terminal status 时直接 `removeItem` + `return`，跳过 `subscribeRun(after_seq=0)` replay。
- 后果：run 已完成并产出 artifact，刷新后 timeline/artifact 全丢。

### 问题 3：Electron `run-error` 被吞
- `apps/desktop/src/main/index.ts` L157-160：`pumpRunEvents` 的 `finally` 块先 `subscriptions.delete` + 发 `awaken:run-closed`。
- `apps/desktop/src/preload/index.ts` L43-50：收到 `run-closed` 后 `subscriptions.delete` + 调 `onClose`。
- `main/index.ts` L193-208：`.catch()` 随后发 `awaken:run-error`，但 preload 已删 subscription → no-op。
- 后果：真实 stream 错误原因丢失，reconnect 失败时只显示泛化文案。

### 问题 4：attemptReconnect 无防重入
- `apps/desktop/src/renderer/runtime/client.ts` L142-162（electron）、L262-282（browser）：`onError` 和 `onClose` 可能近乎同时触发，两次 `attemptReconnect` 覆盖 `reconnectTimer` 但不清除第一个，导致重复 EventSource/IPC subscription。

### 问题 5：recovered 后 timeline 标题错误
- `apps/desktop/src/renderer/runtime/reducer.ts` L321-322：`run/recovered` → `syncFromRun(initialState, action.run)`，对 running 状态不设 `timelineTitle`，保留 `initialState.timelineTitle`（"等待运行"）。

### 问题 6：UI 层无测试
- `apps/desktop/vitest.config.ts`：include `src/**/*.test.ts`，已有 `reducer.test.ts`。
- 无 client reconnect/recovery 测试，无 reducer replay 幂等性测试。

## Proposed Changes

### Fix 1 (P0)：AG-UI frame 级 seq

**方案：stride 编码 + handler 层 frame 级过滤**

引入 `frameSeqStride = 16`（当前 max frames/event = 2，留余量）。每个 AG-UI frame 的 seq = `runtimeEvent.Seq * stride + frameIndex`。由于 runtime seq 严格递增，`runtimeSeq * stride` 严格递增，sub-index 填充间隙，保证 frame seq 在流顺序上单调递增。

**文件：`src/protocol/agui/mapper.go`**
- 无需改动（mapper 不负责 seq 赋值）。

**文件：`src/server/handlers.go`**
- 新增常量 `frameSeqStride = 16`。
- 修改 `mapRuntimeEvent`：将 `mapped[i].Seq = event.Seq` 改为 `mapped[i].Seq = event.Seq * frameSeqStride + uint64(i)`。对 `EventKindRunCancelled`、`EventKindRunError`、`EventKindRunStatusChanged` 的单 frame 路径，同样改为 `event.Seq * frameSeqStride + 0`（或提取为辅助函数 `frameSeq(event.Seq, i)`）。
- 修改 `handleRunEvents`：
  - 将客户端 `after_seq`（frame 级）转为 EventBus 粗粒度 afterSeq：`coarseAfter = afterSeq / frameSeqStride`；`eventBusAfter = coarseAfter > 0 ? coarseAfter - 1 : 0`（保证包含 afterSeq 所在 runtime event，over-inclusive 至多 1 个 event）。
  - 调用 `h.runService.Subscribe(runID, eventBusAfter)`。
  - History replay 循环改为 frame 级过滤：展开 `mapRuntimeEvent(event)`，跳过 `frame.Seq <= afterSeq` 的 frame，写入其余。
  - Live tail 保持 `writeRuntimeEvent`（不过滤，因 live event 全为新 frame）。

**文件：`src/protocol/agui/encoder.go`**
- 无需改动（已用 `event.Seq` 写 `id:` 行，frame seq > 0 时自动输出）。

**文件：`src/server/server_test.go`**
- 新增 `TestRunEventsMultiFrameSeqUnique`：构造触发 `ReasoningStartedEventType` 的 run，验证 SSE 输出中 `REASONING_START` 和 `REASONING_MESSAGE_START` 拥有不同的 `id:` 值。
- 新增 `TestRunEventsAfterSeqReconnectMultiFrame`：订阅收到第一条 reasoning frame 后断开，用 `after_seq=<第一条frameSeq>` 重连，验证第二条 frame 不丢失。

### Fix 2 (P0)：terminal run 刷新恢复全量 replay

**文件：`apps/desktop/src/renderer/App.tsx`**
- 修改 `recover()` 函数（L148-176）：删除 L153-157 的 terminal status 早返回分支。无论 run 是否 terminal，都执行 `dispatch({ type: "run/recovered", run })` + `subscribeRun(run.runId, handlers, 0)`。
- terminal run 的 replay 会通过 `onEvent` 收到 terminal event，触发 `terminalSeenRef.current = true` + `localStorage.removeItem("awaken:runId")`（L229-232 已有逻辑）。
- terminal run 的 EventBus 已 closed，`Subscribe(0)` 返回 history + `Events=nil`，handler 写完 history 后 stream 正常关闭，`onClose` 因 `terminalSeen=true` 不报错。

### Fix 3 (P0)：Electron `run-error` 排序修复

**文件：`apps/desktop/src/main/index.ts`**
- 修改 `pumpRunEvents`（L108-161）：将 `finally` 块中的 `subscriptions.delete` 和 `sendToRenderer("awaken:run-closed", ...)` 移出，放到 try 块正常完成路径之后（while 循环结束后）。`finally` 仅保留 `reader.releaseLock()`。
- 修改 `.catch()` 回调（L193-208）：先发 `awaken:run-error`，再发 `awaken:run-closed`，最后 `subscriptions.delete`。删除 `.catch()` 中重复的 `run-closed` 发送（原来 finally 已发，现在 finally 不发了，catch 中发一次）。
- 结果：error 路径下 preload 先收到 `run-error`（handlers 仍在 map 中，`onError` 被调用），再收到 `run-closed`（handlers 删除，`onClose` 被调用）。

**文件：`apps/desktop/src/preload/index.ts`**
- 无需改动（`run-closed` handler 已正确 delete + 调 onClose）。

### Fix 4 (P1)：attemptReconnect 防重入

**文件：`apps/desktop/src/renderer/runtime/client.ts`**
- 在 electron `subscribeRun` 和 browser `subscribeRun` 的闭包中新增 `reconnecting: boolean` 标志（初始 false）。
- `attemptReconnect` 开头：`if (disposed || terminalSeen || reconnecting) return;`，然后置 `reconnecting = true`。
- `resubscribe`/`connect` 成功时：`reconnecting = false`。
- `resubscribe`/`connect` 失败再次调 `attemptReconnect` 时：`reconnecting` 仍为 true（由 attemptReconnect 入口判断），但需在设置新 timer 前重置以允许下一次重试 → 在 `attemptReconnect` 设置 timer 后保持 `reconnecting = true`，在 timer fire 调用 `resubscribe`/`connect` 时 `reconnecting` 仍为 true（防止 connect 期间再次触发），成功后置 false。
- 具体：`reconnecting` 在 `attemptReconnect` 入口设 true，在 `resubscribe`/`connect` 成功后设 false，在 `resubscribe`/`connect` 失败调 `attemptReconnect` 时保持 true（入口 guard 直接 return，但失败路径已直接调 `attemptReconnect`，需在调用前重置 `reconnecting = false` 以允许重入）。

  简化实现：用 `reconnectTimer !== null` 作为防重入标志。`attemptReconnect` 开头：`if (reconnectTimer) return;`。设置 timer 时赋值，timer fire 回调中清 null 后调 `resubscribe`/`connect`。这样同一时刻最多一个 pending timer。

### Fix 5 (P1)：UI/client 层恢复测试

**文件：`apps/desktop/src/renderer/runtime/reducer.test.ts`（扩展）**
- 新增 `describe("runtime reducer replay idempotency")`：
  - `run/recovered 后全量 replay 文本不重复`：dispatch `run/recovered` + 多条 `TEXT_MESSAGE_CONTENT`，验证 `assistantText` 等于 delta 之和（无重复）。
  - `run/recovered 后 artifact 不重复`：dispatch `run/recovered` + 两条同 `artifactId` 的 `AWAKEN_ARTIFACT_UPDATED`，验证 artifacts 长度为 1。
  - `run/recovered 后 approval upsert 不重复`：dispatch `run/recovered` + 两条同 `approvalId` 的 `AWAKEN_APPROVAL_REQUESTED`，验证 approvals 长度为 1。
  - `run/recovered terminal run 设置正确 phase`：dispatch `run/recovered` with status=finished，验证 phase=done。
  - `run/recovered 后 timelineTitle 为恢复运行`（配合 Fix 6）。

**文件：`apps/desktop/src/renderer/runtime/client.test.ts`（新建）**
- Mock `EventSource`（vitest `vi.stubGlobal`）和 `setTimeout`（`vi.useFakeTimers`）。
- 测试 browser client：
  - `非 terminal close 后进入 reconnecting`：模拟 EventSource `onerror`，验证 `onReconnect` 被调用、新 EventSource 用 `after_seq=<lastSeq>` 创建。
  - `max retries 后显示可读错误`：模拟连续 5 次 `onerror`，验证 `onError` 收到含 "failed after 5 reconnect attempts" 的 message。
  - `terminal event 后不重连`：发送 `RUN_FINISHED` 后触发 `onerror`，验证无 `onReconnect` 调用。
  - `dispose 后不重连`：调用 unsubscribe 后触发 `onerror`，验证无 `onReconnect`。

### Fix 6 (P1)：recovered 后 timeline 标题

**文件：`apps/desktop/src/renderer/runtime/reducer.ts`**
- 修改 `run/recovered` case（L321-322）：
  ```ts
  case "run/recovered":
    return { ...syncFromRun(initialState, action.run), timelineTitle: "恢复运行" };
  ```
- 不影响 `run/created`（走 `syncFromRun(state, action.run)`，state 已有 prompt title）。

### Fix 7 (P2)：统一 missing run 404 格式 — 暂不实施

### Fix 8 (P2)：非法 after_seq 返回 400 — 暂不实施

### Fix 9 (P2)：Last-Event-ID 取舍 — 暂不实施，保留手动 after_seq 方案

### Fix 10 (P2)：清理 App.tsx onClose dead code

**文件：`apps/desktop/src/renderer/App.tsx`**
- `createSubscribeHandlers` 的 `onClose`（L259-265）：移除 `if (!terminalSeenRef.current)` 分支（新 client 语义下非 terminal close 走 `attemptReconnect` 而非 `onClose`）。保留 `unsubscribeRef.current = null` 清理。
- 加注释说明非 terminal close 由 client 层 `attemptReconnect` 处理。

## Assumptions & Decisions

1. **stride=16**：当前 max frames/event=2，16 提供充分余量。若未来 event 类型映射 >16 frame 需调大。添加注释说明约束。
2. **seq 值变更不兼容旧 localStorage**：slice 9 未发布，无迁移负担。旧 `lastSeq` 值（如 3）会被 `coarseAfter = 3/16 = 0` 正确处理为全量 replay。
3. **EventBus 不改动**：seq 仍按 runtime event 粒度分配，frame 级 seq 在 handler 层派生。保持 runtime/protocol 层分离。
4. **terminal run 恢复**：依赖 EventBus closed 状态下 `Subscribe(0)` 返回完整 history + `Events=nil`，handler 写完 history 后 stream 关闭。已有行为，无需改动 runtime。
5. **client 测试用 fake timers + mock EventSource**：vitest 原生支持 `vi.useFakeTimers` 和 `vi.stubGlobal`。
6. **P2 项（7/8/9）暂不实施**：用户标注可选，核心恢复逻辑不受影响。

## Verification Steps

1. **Go 测试**：`go test ./src/runtime/... ./src/server/... ./src/protocol/...`，确认新增 multi-frame seq 测试通过。
2. **Desktop typecheck**：`cd apps/desktop && npx tsc --noEmit`，无类型错误。
3. **Desktop 测试**：`cd apps/desktop && npm test`，确认 reducer replay 幂等性测试和 client reconnect 测试通过。
4. **手动验证 multi-frame reconnect**（可选，依赖 LLM）：
   - 启动 run 触发 reasoning，SSE 输出 `REASONING_START`(id:16) + `REASONING_MESSAGE_START`(id:17)。
   - 收到 id:16 后断开，用 `after_seq=16` 重连，验证 id:17 被重放。
5. **手动验证 terminal run 恢复**：run 完成后刷新 Desktop，验证 timeline/artifact/final text 恢复。
6. **手动验证 stream error 透传**：模拟 runtime 断连，验证 UI 显示真实错误原因而非泛化文案。

## Implementation Order

1. Fix 1（seq）→ Fix 1 测试（Go 端先验证）
2. Fix 3（Electron error 排序）+ Fix 4（防重入）— 同一文件链路，一起改
3. Fix 2（terminal run 恢复）+ Fix 10（dead code 清理）— 同在 App.tsx
4. Fix 6（timeline title）— reducer.ts
5. Fix 5（UI 测试）— 最后补测试
6. 全量验证：go test + tsc + vitest
