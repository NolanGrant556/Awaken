# Slice 4 评审问题修复计划

## Summary

本次计划修复 Slice 4（只读文件任务）代码评审中提出的 5 类问题：

1. RunTimeline 中 `TOOL_CALL_RESULT` 把已完成的 tool step 改回 `active` 的用户可见 bug。
2. 只读工具 workspace 越界拒绝未验证其 error 会作为 `tool_result` 回填给模型的测试缺口。
3. `list_files` / `grep_search` 缺少相对路径穿越（`../`）的 Execute 路径拒绝测试。
4. 真实 LLM smoke 测试断言过于脆弱（仅判非空）。
5. 规格文档中 `src/agentcore/conversation.go` 路径错误。

## Current State Analysis

### 1. RunTimeline tool step 状态

- 相关文件：
  - [apps/desktop/src/renderer/runtime/reducer.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts) L365-L412
  - [packages/ui/src/components/feedback/RunTimeline.tsx](file:///Users/nolangrant/Awaken/packages/ui/src/components/feedback/RunTimeline.tsx)
  - [packages/ui/src/components/feedback/RunStep.tsx](file:///Users/nolangrant/Awaken/packages/ui/src/components/feedback/RunStep.tsx)

- 问题：事件顺序为 `TOOL_CALL_START` → `TOOL_CALL_ARGS` → `TOOL_CALL_END` → `TOOL_CALL_RESULT`。
  - `TOOL_CALL_END` 把 step key `tool:${toolCallId}` 标记为 `status: "done"`。
  - `TOOL_CALL_RESULT` 再次使用 key `tool:${id}`（当 `toolCallId` 存在时与前者相同）覆盖为 `status: "active"`。
  - 最终 UI 上工具调用完成后仍显示为运行中。

### 2. 只读工具越界拒绝的 tool result 回填

- 相关文件：
  - [src/agentcore/runner.go](file:///Users/nolangrant/Awaken/src/agentcore/runner.go) L245-L256
  - [src/agentcore/tool_runtime.go](file:///Users/nolangrant/Awaken/src/agentcore/tool_runtime.go) L44-L105
  - [src/context/context.go](file:///Users/nolangrant/Awaken/src/context/context.go) L270-L276
  - [src/tools/file_read/file_read_test.go](file:///Users/nolangrant/Awaken/src/tools/file_read/file_read_test.go)
  - [src/tools/list_files/list_files_test.go](file:///Users/nolangrant/Awaken/src/tools/list_files/list_files_test.go)
  - [src/tools/grep_search/grep_search_test.go](file:///Users/nolangrant/Awaken/src/tools/grep_search/grep_search_test.go)

- 问题：现有测试只断言 Execute 返回 error 包含 "outside workspace"，未在 runner/agentcore 层验证该 error 会被包装成 `ToolExecutionResult` 并通过 `ToolExchange` 回填为 `tool_result`，也未验证不触发 approval。

### 3. list_files / grep_search 相对路径穿越测试

- 相关文件：
  - [src/tools/list_files/list_files_test.go](file:///Users/nolangrant/Awaken/src/tools/list_files/list_files_test.go)
  - [src/tools/grep_search/grep_search_test.go](file:///Users/nolangrant/Awaken/src/tools/grep_search/grep_search_test.go)
  - [src/tools/file_read/file_read_test.go](file:///Users/nolangrant/Awaken/src/tools/file_read/file_read_test.go) L102-L119（已有 `TestReadFileBlocksTraversal`，可作为模板）

- 问题：`file_read` 有 `../outside.txt` 的 Execute 拒绝测试，但 `list_files` 和 `grep_search` 目前只有绝对路径越界测试（`/etc`、`~/Desktop`），缺少相对路径归一化/穿越的 Execute 失败路径。

### 4. 真实 LLM smoke 断言

- 相关文件：
  - [src/server/server_test.go](file:///Users/nolangrant/Awaken/src/server/server_test.go) L606-L747

- 问题：`TestReadOnlyFileSmoke` 仅检查最终文本非空，未检查文本是否与 README/项目相关；且对“必须调用只读工具”的断言可能因模型差异而脆弱。

### 5. 规格文档路径错误

- 相关文件：
  - [.trae/specs/implement-slice4-readonly-file-tasks/spec.md](file:///Users/nolangrant/Awaken/.trae/specs/implement-slice4-readonly-file-tasks/spec.md) L22

- 问题：受影响代码列出 `src/agentcore/conversation.go`，实际不存在，对话管理在 `src/context/context.go`。

## Proposed Changes

### Change 1: 修复 RunTimeline tool step 最终状态

**文件**: [apps/desktop/src/renderer/runtime/reducer.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts) L381-L411

**What**: 调整 `TOOL_CALL_RESULT` 的 step 处理逻辑：
- 当 `toolCallId` 存在时，复用同一 step key 并更新 description 为结果摘要，但把 `status` 设为 `"done"` 而不是 `"active"`。
- 当 `toolCallId` 缺失时，再fallback 到生成独立 step key（保持现状）。
- 同时把 title 改为 `"Tool result"`（若 event 提供 toolCallName 则保留）以准确表达这是一个结果节点。

**Why**: 避免 `TOOL_CALL_RESULT` 覆盖 `TOOL_CALL_END` 的完成状态，使用户看到工具调用已结束并带有结果摘要。

**How**: 修改 `case "TOOL_CALL_RESULT":` 分支：
```ts
const id = action.event.toolCallId || `tool-result-${state.toolResults.length + 1}`;
const stepKey = action.event.toolCallId ? `tool:${action.event.toolCallId}` : `tool-result:${id}`;
records = upsertStep(records, stepKey, {
  title: action.event.toolCallName || "Tool result",
  description: excerpt(content),
  status: "done",
  tag: "tool",
});
```

### Change 2: 补齐只读工具越界拒绝的 tool result 回填测试

**文件**: [src/agentcore/runner_test.go](file:///Users/nolangrant/Awaken/src/agentcore/runner_test.go)

**What**: 新增一个单元测试（例如 `TestRunnerReadOnlyToolOutsideWorkspaceFeedsErrorBack`）：
- 构造 workspace root 为临时目录的 runner。
- 让模型 LLM mock 返回一个 `read_file` tool call，参数为 `/etc/hosts`。
- 调用 `RunStreamWithEvents`（或 `Run`）。
- 断言：
  - 工具返回 error 且内容包含 "outside workspace" / "permission"。
  - 事件流中出现 `TOOL_CALL_RESULT` 且内容包含拒绝信息。
  - 不出现 `AWAKEN_APPROVAL_REQUESTED`。
  - conversation messages 中 assistant 的 `tool_use` 与下一个 `tool` result 配对正确，且 tool result 的内容就是拒绝信息。

**Why**: 验证 workspace guard 的拒绝路径会作为 tool result 回填，模型可据此继续，而不是触发 approval 或静默失败。

**How**: 复用现有 runner test 的 mock server 与 helper（`newRunnerAnthropicStreamServer`、`testAgentConfig` 等），注册真实 `read_file` 工具，通过 `toolRuntime` 的权限上下文限制 workspace。

### Change 3: 补齐 list_files / grep_search 相对路径穿越 Execute 测试

**文件**:
- [src/tools/list_files/list_files_test.go](file:///Users/nolangrant/Awaken/src/tools/list_files/list_files_test.go)
- [src/tools/grep_search/grep_search_test.go](file:///Users/nolangrant/Awaken/src/tools/grep_search/grep_search_test.go)

**What**: 分别新增：
- `TestListFilesBlocksTraversal`：workspace 为 `parent/workspace`，调用 `{"root":"../outside"}`，断言返回 error 包含 "outside workspace"。
- `TestGrepSearchBlocksTraversal`：workspace 为 `parent/workspace`，在 `parent/outside.txt` 写入内容，调用 `{"query":"secret","root":"../outside"}`，断言返回 error 包含 "outside workspace"。

**Why**: 与 `file_read` 的 `TestReadFileBlocksTraversal` 对齐，确保三种只读工具都覆盖相对路径归一化/穿越的 Execute 失败路径。

**How**: 参考 [src/tools/file_read/file_read_test.go](file:///Users/nolangrant/Awaken/src/tools/file_read/file_read_test.go) L102-L119 的模式，构造父目录、workspace 子目录、外部目录，通过 `tool.ContextWithPermissionContext` 注入权限上下文后调用 `Execute`。

### Change 4: 增强真实 LLM smoke 的低脆弱断言

**文件**: [src/server/server_test.go](file:///Users/nolangrant/Awaken/src/server/server_test.go) L606-L747

**What**: 在 `TestReadOnlyFileSmoke` 中保留现有强断言（必须调用只读工具、无 approval/artifact、有 RUN_FINISHED）的前提下，增强最终回复可读性检查：
- 收集所有 `EventToolCallResult` 的内容，检查是否至少有一个结果包含 README 相关路径（如 `README`）或项目相关文件内容。
- 或者检查 `assistantText` / `finalResult` 中包含 `README`、`项目`、`project`、`Awaken` 等任一关键词（大小写不敏感）。
- 不强制要求固定中文表述。

**Why**: 降低 smoke 测试因模型措辞差异而失败的风险，同时确保最终总结确实基于读取到的 README/项目内容。

**How**: 在事件收集循环中记录 tool result 内容；循环结束后用 `strings.Contains(strings.ToLower(...), ...)` 做宽松匹配。

### Change 5: 修正规格文档中的错误文件路径

**文件**: [.trae/specs/implement-slice4-readonly-file-tasks/spec.md](file:///Users/nolangrant/Awaken/.trae/specs/implement-slice4-readonly-file-tasks/spec.md) L22

**What**: 将受影响代码列表中的 `src/agentcore/conversation.go` 替换为 `src/context/context.go`。

**Why**: 文档一致性。

**How**: 直接编辑 spec.md 的 Impact 小节。

## Assumptions & Decisions

1. **UI 状态修复采用“同一 step 完成”方案**：不新增独立 result step，而是在同一 `tool:${toolCallId}` step 上以 `done` 状态展示结果摘要。这样与现有 `upsertStep` 语义一致，且不会导致 timeline 中同一工具调用出现两个条目。
2. **越界回填测试放在 runner 层**：比在单个 tool 测试里更能验证“拒绝 → tool result → conversation”完整链路，符合 slice4 验收意图。
3. **相对路径穿越测试使用 `../outside` 而非符号链接**：与现有 `file_read` 测试风格保持一致，覆盖最常见的目录穿越场景。
4. **smoke 断言使用宽松关键词匹配**：避免对模型输出做严格断言，同时确保总结确实与 README/项目相关。
5. **不修改工具实现本身**：现有 workspace guard 实现已通过测试，本次只补测试和 UI/文档修复。

## Verification Steps

1. 运行 Go 单元测试：
   ```bash
   go test ./src/agentcore/... ./src/tools/... ./src/runtime/...
   ```
   预期：全部通过。

2. 运行 server 层 smoke（若环境有 `LLM_API_KEY`）：
   ```bash
   go test ./src/server/... -run TestReadOnlyFileSmoke -v
   ```
   预期：通过或 `t.Skip`（未设置 key 时）。

3. 运行 Desktop 构建与类型检查：
   ```bash
   cd apps/desktop && npm run build
   ```
   预期：构建成功。

4. （可选）运行 Desktop 后输入“找出 README 并总结项目”，确认 timeline 中工具调用步骤最终显示为已完成并带有结果摘要。
