# Slice 8 Cancel Run — 修复与补测计划

## Summary

Slice 8（取消 Run）已开发完成，但 review 发现 4 个必修问题（cancel 后同批工具继续执行、missing cancel 非结构化错误、cancel 状态标签错误、cancelled timeline 重复步骤）和若干顺手修项。本计划修复全部 10 项，补齐 agentcore 多工具 cancel 测试、server JSON 错误测试、UI reducer cancelled 测试（引入 vitest），并校正 checklist。

## Current State Analysis

测试与 typecheck 当前均通过，但存在以下已确认问题：

1. **cancel 后同批后续工具继续执行**：`RunStreamWithEvents`（[runner.go:122-128](file:///Users/nolangrant/Awaken/src/agentcore/runner.go#L122-L128)）通过 `emitToolCallEvents` 一次性 emit 全部 `TOOL_CALL_START`，再调 `toolRuntime.ExecuteToolCalls` 内部循环执行，循环中无 per-tool `ctx.Err()` 检查。`injectSyntheticToolResultsForCancel` 只扫描已加入 conversation 的消息，而 `ToolExchange` 在执行后才调用，故 in-flight batch 的 tool_use 无法被它捕获。
2. **missing cancel 非结构化错误**：[handlers.go:102-104](file:///Users/nolangrant/Awaken/src/server/handlers.go#L102-L104) 用 `http.NotFound`，返回纯文本 `404 page not found`，违反 spec "结构化错误"。[handlers.go:106](file:///Users/nolangrant/Awaken/src/server/handlers.go#L106) internal error 用 `http.Error`，不一致。现有测试 [server_test.go:277-282](file:///Users/nolangrant/Awaken/src/server/server_test.go#L277-L282) 只断言 status code，未断言 JSON body。
3. **`run/cancelRequested` 状态标签错误**：[reducer.ts:295-301](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L295-L301) 未设置 `status`。approval 等待时点击 cancel，`status` 仍为 `"awaiting_approval"`，而 [App.tsx:51-53](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx#L51-L53) `formatStatusLabel` 优先检查 `status === "awaiting_approval"`，导致显示 "Awaiting approval" 而非 "Cancelling"。
4. **cancelled timeline 重复步骤**：`RUN_STATUS_CHANGED(cancelled)`（[reducer.ts:345-361](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L345-L361)）插入 key `status:cancelled`；`RUN_ERROR/RUN_CANCELLED`（[reducer.ts:530-540](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L530-L540)）插入 key `run-cancelled`。两个不同 key 都产生 "Run cancelled" 步骤。
5. **冗余表达式**：[reducer.ts:359](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L359) `status: status === "cancelled" ? "done" : "done"` 恒为 `"done"`。
6. **UI reducer 测试缺失**：`apps/desktop/` 无任何测试文件，无测试框架（package.json 无 test script，无 vitest/jest 依赖）。checklist 中 `[x] UI reducer 验证 cancelled 终态` 标记不实。

**错误码约定**（来自现有代码）：`INVALID_REQUEST`、`INVALID_INPUT`、`INTERNAL_ERROR`、`APPROVAL_NOT_FOUND`、`RUN_TERMINAL`、`RUN_CANCELLING`。新增 `RUN_NOT_FOUND` 与 `APPROVAL_NOT_FOUND` 平行。

**工具执行链路**：`runner.executeToolCalls` → `toolRuntime.ExecuteToolCalls` → 循环 `rt.Execute(ctx, call)`。`rt.Execute`（[tool_runtime.go:44](file:///Users/nolangrant/Awaken/src/agentcore/tool_runtime.go#L44)）是单个工具的纯执行器，不含 cancel 语义。

## Proposed Changes

### 1. `src/agentcore/runner.go` — 重构 executeToolCalls，per-tool cancel 检查

**What**:
- 删除 `RunStreamWithEvents` 中 `r.emitToolCallEvents(toolCalls, events)` 调用（[runner.go:124](file:///Users/nolangrant/Awaken/src/agentcore/runner.go#L124)），改为在 `executeToolCalls` 内 per-tool emit。
- 重写 `executeToolCalls`（[runner.go:297-308](file:///Users/nolangrant/Awaken/src/agentcore/runner.go#L297-L308)）为 per-tool 循环：
  - 每个 tool 执行前检查 `ctx.Err()`。
  - 若 ctx 已 cancel：append `syntheticCancelledToolResult(call)`，**不 emit 任何事件**（无 START、无 RESULT），`continue`。与 between-turns cancel 路径（`injectSyntheticToolResultsForCancel` 不 emit）保持一致。
  - 否则：`events.emitToolCall(call)`（当 events != nil）→ `r.toolRuntime.Execute(ctx, call)` → callback → `events.emitToolResult(result)`（当 events != nil）。
- 新增 helper：
  ```go
  func syntheticCancelledToolResult(call convcontext.ToolCall) ToolExecutionResult {
      return ToolExecutionResult{
          ToolCallID: call.ID,
          Name:       call.Function.Name,
          Content:    "工具执行已被取消",
          IsError:    true,
      }
  }
  ```
- `emitToolCallEvents` 函数（[runner.go:288-295](file:///Users/nolangrant/Awaken/src/agentcore/runner.go#L288-L295)）若不再有调用方则删除；若 `handleXMLFallback` 仍需要可保留，但检查后该路径也走 `executeToolCalls`，故可删。

**Why**: cancel 后同批后续工具不应执行、不应 emit `TOOL_CALL_START`、不应发生 mutation。synthetic result 保证 `ToolExchange` 后 conversation pairing 完整。

**How**: `Run`/`RunStream`（events=nil）同样受益：per-tool ctx 检查使其在 cancel 后停止执行后续工具并补 synthetic result，行为更正确且不破坏现有测试。

**Decision**: `src/agentcore/tool_runtime.go` **不改动**。cancel-aware synthetic result 生成是 runner 层语义（"工具执行已被取消" 文案、IsError 标记），ToolRuntime 作为纯执行器不应承担 cancel 语义。用户列出该文件但实际修复属于 runner 层。

### 2. `src/server/handlers.go` — cancel 结构化 JSON 错误

**What**（[handlers.go:98-111](file:///Users/nolangrant/Awaken/src/server/handlers.go#L98-L111)）:
- `errors.Is(err, awruntime.ErrRunNotFound)` 分支：`http.NotFound(w, r)` → `writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())`。
- 默认分支：`http.Error(w, "failed to cancel run", http.StatusInternalServerError)` → `writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel run")`。

**Why**: spec 要求 "cancel 不存在的 run 返回结构化错误"；与 `handleResolveApproval` 错误处理风格一致。

### 3. `src/server/server_test.go` — 断言 JSON body

**What**（[server_test.go:277-282](file:///Users/nolangrant/Awaken/src/server/server_test.go#L277-L282)）:
- missing cancel 用例：除断言 `rec.Code == 404` 外，decode body 为 `errorResponse`，断言 `errResp.Error == "RUN_NOT_FOUND"` 且 `errResp.Message != ""`。

**Why**: 防止回归到纯文本 404。

### 4. `apps/desktop/src/renderer/runtime/reducer.ts` — 修 status、重复步骤、冗余表达式

**What**:
- `run/cancelRequested` case（[reducer.ts:295-301](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L295-L301)）：增加 `status: "cancelling"`。
- `RUN_STATUS_CHANGED` handler（[reducer.ts:345-361](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L345-L361)）：`cancelled` 分支的 step key 从 `` `status:${status}` `` 改为 `"run-cancelled"`，与 `RUN_ERROR/RUN_CANCELLED`（[reducer.ts:535](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L535)）统一。后续 `RUN_ERROR` 事件 upsert 同一 key，不再产生重复步骤。
- 冗余表达式（[reducer.ts:359](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts#L359)）：`status: status === "cancelled" ? "done" : "done"` → `status: "done"`。

**Why**: 修正 cancel 标签；消除 timeline 重复；清理死代码。

### 5. `src/agentcore/runner_test.go` — 多工具 cancel 测试

**What**: 新增 `TestRunnerCancelStopsSubsequentToolsInBatch`：
- 注册两个工具：`cancelOnExecuteTool`（复用 [runner_test.go:912-934](file:///Users/nolangrant/Awaken/src/agentcore/runner_test.go#L912-L934) 模式，Execute 时调 `cancel()`）和 `failIfExecutedTool`（Execute 时 `t.Fatal("tool 2 should not execute")`）。
- LLM server 第一轮返回同一 response 中两个 tool_use（id `toolu_cancel_1`、`toolu_cancel_2`）；第二轮若被调用则 `t.Fatalf`。
- 断言：
  - `calls == 1`（无第二轮 LLM 调用）。
  - 事件中仅 1 个 `ToolCallStartedEventType`（`toolu_cancel_1`），无 `toolu_cancel_2` 的 START。
  - `convcontext.ValidateToolResultPairing(messages)` 通过。
  - `toolu_cancel_2` 对应的 tool result message 存在，`IsError == true`，`Content == "工具执行已被取消"`。
  - `failIfExecutedTool` 未被执行（由其自身 `t.Fatal` 保证）。

**Why**: 确定性覆盖 cancel 在 batch 中途时的不变量。

### 6. `apps/desktop` — 引入 vitest + reducer 测试

**What**:
- `apps/desktop/package.json`：
  - `devDependencies` 增加 `"vitest": "latest"`（executor 安装时确认与 vite ^8.0.16 兼容；若 peer 冲突则升 vitest 主版本）。
  - `scripts` 增加 `"test": "vitest run"`、`"test:watch": "vitest"`。
- 新增 `apps/desktop/vitest.config.ts`（minimal）：
  ```ts
  import { defineConfig } from "vitest/config";
  export default defineConfig({
    test: { environment: "node", include: ["src/**/*.test.ts"] },
  });
  ```
- 新增 `apps/desktop/src/renderer/runtime/reducer.test.ts`，覆盖：
  - `run/cancelRequested`：dispatch 后 `state.status === "cancelling"`、`state.phase === "cancelling"`。
  - `RUN_STATUS_CHANGED` status=`cancelled`：`state.phase === "cancelled"`、timeline 中 `run-cancelled` 步骤恰好 1 个。
  - 先 `RUN_STATUS_CHANGED(cancelled)` 再 `RUN_ERROR` code=`RUN_CANCELLED`：`run-cancelled` 步骤仍为 1 个（不重复）、`state.phase === "cancelled"`、`state.errorMessage` 为空。
  - `RUN_ERROR` code=`RUN_CANCELLED` 单独到达：`state.phase === "cancelled"`。
- 测试用纯 reducer 函数，无 DOM 依赖；`import type` 在运行时被擦除，无 `@awaken/ui` 运行时依赖。

**Why**: 用户选择引入 vitest 补单测；reducer 是纯函数，适合单测。

### 7. `.trae/specs/implement-slice8-cancel-run/checklist.md` — 校正状态

**What**:
- 修复完成后，确认 `[x]` 项属实。
- `[x] UI reducer 验证 cancelled 终态` 在 #6 完成后保留 `[x]`，可补注 "via vitest reducer.test.ts"。
- 其余项保持。

## Assumptions & Decisions

- **tool_runtime.go 不改动**：cancel 语义属 runner 层，ToolRuntime 保持纯执行器。
- **synthetic cancel result 文案**：`"工具执行已被取消"`，与现有 `injectSyntheticToolResultsForCancel`（[runner.go:188](file:///Users/nolangrant/Awaken/src/agentcore/runner.go#L188)）一致。
- **skipped tool 不 emit 事件**：与 between-turns cancel 路径一致；conversation 通过 `ToolExchange` 获得 synthetic result，pairing 完整。
- **错误码**：`RUN_NOT_FOUND`，与 `APPROVAL_NOT_FOUND` 平行。
- **vitest 版本**：`latest`，executor 安装时验证与 vite 8 兼容性。
- **`emitToolCallEvents` 函数**：重构后若无调用方则删除，避免死代码。

## Verification

```bash
# Go
go test ./src/runtime/... ./src/agentcore/... ./src/server/...
go test ./...

# Desktop
npm run build:desktop
npm run typecheck:desktop
npm --workspace @awaken/desktop run test

# Lint
git diff --check
```

预期：
- Go 测试全绿，含新增 `TestRunnerCancelStopsSubsequentToolsInBatch` 与增强的 missing cancel JSON 断言。
- Desktop typecheck/build 通过。
- vitest reducer 测试全绿。
- `git diff --check` 无空白错误。
