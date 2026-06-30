# Awaken 主 Agent 完整闭环开发计划

更新时间：2026-06-30

## 目标

把 Awaken 现有 `Agent -> Runner -> RequestBuilder -> LLM stream -> ToolRuntime -> tool_result -> next turn -> Runtime/SSE/Desktop` 路径增强成完整主 Agent 闭环。

完整闭环定义：

```text
用户输入
-> 创建 run / turn
-> 写入 conversation message
-> 构造 ContextEnvelope
-> 组装 ToolPool
-> 计算上下文窗口
-> 必要时 compact
-> 构建 LLM request
-> 流式接收 assistant text / tool_use
-> AUTO ToolRuntime 执行工具
-> tool_result 回写 conversation
-> 校验 tool_use / tool_result pairing
-> 继续下一轮 LLM
-> completed / cancelled / max_turns / context_blocked / model_error
-> 写 transcript
-> Runtime/SSE/Desktop 同步
```

## 非目标

以下内容不进入本轮主 Agent 闭环：

- subagent / worker / team mode。
- 用户审批 UI 作为主路径。
- AgentDefinition registry 和 `subagent_type` 选择。
- 复杂 ToolSearch / marketplace / deferred tool discovery。
- Claude Code 的 coding-only prompt、git/PR/worktree 规则。
- API-side context management、cached microcompact、context collapse 全量照搬。
- remote session、channel approval、permission relay。

## 当前项目基线

### 已有能力

- `src/agentcore/agent.go`
  - `Agent.RunStreamWithEvents()` 已把用户输入写入 `Conversation`，再交给 `Runner`。
- `src/agentcore/runner.go`
  - `Runner.RunStreamWithEvents()` 已有多轮循环：LLM stream、工具调用、tool result 回写、继续下一轮。
  - 已有 `injectSyntheticToolResultsForCancel()`，取消时会补错误 tool result。
  - `buildLLMRequest()` 已在请求前调用 `context.ValidateToolResultPairing()`。
- `src/agentcore/request_builder.go`
  - 已有系统 prompt、runtime info、tool schema 构建。
- `src/context/context.go`
  - 已有 `Conversation`、`Message`、`ToolCall`、`ToolResult`、粗略 token 估算。
- `src/context/tool_pairing.go`
  - 已有严格 tool_use / tool_result pairing 校验。
- `src/agentcore/tool_runtime.go`
  - 已有工具查找、参数 normalize、权限判断、工具执行、artifact、command output、patch preview、tool result。
- `src/tool/permission.go`
  - 已有 `allow / ask / deny` 权限行为和 workspace / writable roots 边界。
- `src/runtime/run_service.go`
  - 已有 thread 复用、run 启动、单 thread active run 锁、cancel、in-memory transcript。
- `src/runtime/session.go`
  - 已有 run 状态、事件总线、approval awaiter、terminal status。
- `src/server/handlers.go`
  - 已有 run API、thread transcript API、SSE replay。
- `apps/desktop/src/renderer/runtime/reducer.ts`
  - 已有 run/thread 视图、tool result、patch、approval、artifact、narrative 投影。
- `src/llm/openai_chat.go` 和 `src/llm/anthropic_messages.go`
  - 已有 OpenAI Chat 与 Anthropic Messages stream/tool_use 解析。
- `src/agentcore/runner_test.go`
  - 已覆盖 native tool loop、XML fallback、tool error、Anthropic tool_use、pairing reject、cancel synthetic tool result。

### 主要缺口

- `Runner` 循环还没有显式 `TurnState` 和 `TerminalReason`。
- `RequestBuilder` 还没有 `ContextEnvelope` 分层结构。
- `Context` 只有粗略 token 估算，没有窗口状态、warning、blocking、compact。
- `ToolRuntime` 的 `ask` 会进入 `AwaitApproval`，和本轮 AUTO 工具主路径冲突。
- `ToolExecutionResult` 字段不足，不能表达 denied / cancelled / decision source / duration。
- Runtime transcript 当前主要是 session 内存事件历史，不是可跨进程恢复的 durable transcript。
- Desktop reducer 仍把 `awaiting_approval` 作为活跃主路径之一，AUTO 主路径需要新的 policy/tool 状态投影。

## 全局约束

### 主 Agent 模式

- 只有一个主 Agent loop。
- 一个 thread 同一时刻最多一个 active run。
- 一个 run 内可以有多轮 LLM turn。
- 工具执行不等待用户审批。
- 工具不允许悄悄失败：失败、拒绝、取消都必须生成合法 `tool_result`。

### AUTO 工具规则

AUTO 不是跳过权限层，而是把 `ask` 改写成自动决策。

```text
tool static permission
-> AutoToolPolicy.Decide
-> allow: execute tool
-> deny: synthesize error tool_result
-> abort: synthesize abort tool_result and terminal if needed
```

M1 默认策略：

- read-only 且 workspace 内：allow。
- 工具自身 deny：deny。
- 参数不合法：deny。
- workspace / writable roots 越界：deny。
- destructive / open-world / requires-user-interaction：默认 deny，除非显式 allowlist。
- policy 不可用：fail closed。

### Message pairing 不变量

- 每个 assistant tool call 必须有且只有一个后续 tool result。
- tool result 必须出现在下一条 user/assistant message 之前。
- tool result 的 `ToolCallID` 必须引用已存在的 tool call。
- 有 `Name` 时必须和 tool call name 一致。
- 取消、deny、validation error 也必须补 tool result。

### Context window 默认值

M1 先用本地估算，不接复杂 API-side context management。

```text
model_window: 从模型配置读取，缺省 128000
output_reserve: 16000
effective_window: model_window - output_reserve
warning_threshold: effective_window * 0.85
autocompact_threshold: effective_window * 0.90
compact_target: effective_window * 0.30
blocking_reserved: 3000 到 5000
```

### TerminalReason

必须统一定义，不允许散落字符串：

- `completed`
- `cancelled`
- `aborted_streaming`
- `aborted_tools`
- `max_turns`
- `context_warning`
- `context_blocked`
- `prompt_too_long`
- `compact_failed`
- `model_error`
- `tool_pairing_error`
- `empty_response`
- `tool_policy_abort`

## 开发清单

### Phase 0：基线锁定

- [ ] 0.1 记录当前主链路基线
  - 进入条件：当前 `main` 或工作分支能跑 Go 测试。
  - 执行项：
    - 跑 `go test ./src/agentcore ./src/context ./src/runtime ./src/server ./src/llm ./src/tool ./src/tools/...`。
    - 跑 Desktop reducer 相关测试。
    - 记录当前通过/失败的测试。
  - 完成条件：
    - 有一份基线测试结果。
    - 失败项被标记为既有失败或本轮必须修复。
  - 解锁下一步：可以开始改核心类型。

- [ ] 0.2 标记保留路径和废弃路径
  - 进入条件：0.1 完成。
  - 执行项：
    - 保留 `Runner.RunStreamWithEvents()` 作为主 loop 入口。
    - 保留 `RequestBuilder.Build()` 作为 LLM request 入口。
    - 保留 `ToolRuntime.Execute()` 作为工具执行入口。
    - 保留 Runtime/SSE/Desktop 现有事件链。
    - 将 approval path 标记为兼容能力，不作为 AUTO 主路径。
  - 完成条件：
    - 文档或代码注释明确这些边界。
  - 解锁下一步：核心类型可以围绕现有入口扩展。

#### Claude Code 参考设计

- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:175-183；设计要点：`QueryEngine` 明确拥有 query lifecycle 和 conversation/session state，一个 conversation 复用一份消息、文件缓存、usage 等状态；Awaken 对应启发：Phase 0 锁定 `Agent -> Runner -> RequestBuilder -> ToolRuntime` 时，应把 `Runner.RunStreamWithEvents()` 视为主闭环生命周期入口，而不是在 Runtime/SSE/Desktop 旁边再造一套状态入口。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:675-686；设计要点：SDK/headless 路径最终仍调用统一的 `query({ messages, systemPrompt, userContext, systemContext, canUseTool, toolUseContext, ... })`；Awaken 对应启发：Phase 0 的基线验证应覆盖从当前消息、request build、permission/tool context 到主 loop 的完整路径，避免只测某个单点 helper。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:181-199；设计要点：`QueryParams` 把 `messages`、`systemPrompt`、`canUseTool`、`toolUseContext`、`maxTurns`、`querySource` 等主 loop 输入集中声明；Awaken 对应启发：保留 `RequestBuilder.Build()` 和 `ToolRuntime.Execute()` 时，新增字段应优先挂到明确的 loop/request/tool context 边界，而不是散落在事件层。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1259-1301；设计要点：发起 API 前统一做 message normalization，并修复 tool_use/tool_result pairing；Awaken 对应启发：Phase 0 基线应记录现有 `RequestBuilder.Build()` 与 tool result 回写的配对行为，后续 Phase 变更时用测试保护“不破坏可继续对话”的契约。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/services/tools/toolExecution.ts:337-467；设计要点：`runToolUse()` 是单个工具调用的稳定执行入口，先找 tool、处理未知工具、检查 abort，再进入权限和调用流程；Awaken 对应启发：Phase 0 标记 `ToolRuntime.Execute()` 为唯一工具执行入口是合理的，后续 AUTO policy 应在该入口内扩展，而不是绕开现有 tool result 生成链。

### Phase 1：核心状态与事件

- [ ] 1.1 新增 `TurnState`
  - 进入条件：Phase 0 完成。
  - 执行项：
    - 在 `src/agentcore` 新增或扩展类型：
      - `SessionID`
      - `ThreadID`
      - `RunID`
      - `TurnID`
      - `QueryChainID`
      - `Depth`
      - `Messages`
      - `TurnCount`
      - `MaxTurns`
      - `MainLoopModel`
      - `TransitionReason`
      - `TerminalReason`
  - 完成条件：
    - `Runner` 每轮循环能生成稳定 turn id。
    - 测试能断言多轮工具调用时 `TurnCount` 增长。
  - 解锁下一步：事件可以带 turn/query 信息。

- [ ] 1.2 新增 terminal reason 事件字段
  - 进入条件：1.1 完成。
  - 执行项：
    - 扩展 `agentcore.AwakenEvent`：
      - `TurnID`
      - `QueryChainID`
      - `Depth`
      - `TerminalReason`
      - `TransitionReason`
    - `RunFinished`、`RunError`、`RunCancelled` 都带 terminal reason。
  - 完成条件：
    - Runtime/SSE 不丢这些字段。
    - Desktop reducer 可以忽略未知字段且不崩。
  - 解锁下一步：stream event 和 transcript 可共享同一 envelope。

- [ ] 1.3 明确事件清单
  - 进入条件：1.2 完成。
  - 执行项：
    - 保留现有事件：
      - `RunStarted`
      - `TextMessageStarted`
      - `TextDelta`
      - `TextMessageFinished`
      - `ReasoningStarted`
      - `ReasoningDelta`
      - `ReasoningFinished`
      - `ToolCallStarted`
      - `ToolCallArgsDelta`
      - `ToolCallArgs`
      - `ToolCallFinished`
      - `ToolProgress`
      - `ToolResult`
      - `PatchUpdated`
      - `TurnDiff`
      - `RunFinished`
      - `RunError`
    - 新增事件：
      - `TurnStarted`
      - `TurnFinished`
      - `ToolPermissionDecided`
      - `ContextEstimated`
      - `ContextWarning`
      - `ContextBlocked`
      - `CompactStarted`
      - `CompactFinished`
      - `CompactFailed`
      - `ToolPairingRepaired`
  - 完成条件：
    - 事件类型在 Go 和 TS shared 类型中都有定义。
    - 现有前端不因新增事件报错。
  - 解锁下一步：AUTO 工具和 context window 可以发事件。

#### Claude Code 参考设计

- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:203-217；设计要点：Claude Code 在 query loop 内维护显式 `State`，包含 `messages`、`toolUseContext`、`turnCount`、`transition` 等跨轮字段；Awaken 对应启发：`TurnState` 应承担跨轮状态载体，而不是只在事件里临时拼 `TurnID/TurnCount`。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:265-279；设计要点：loop 入口集中初始化 mutable state，`turnCount` 初始为 1，`transition` 初始为空；Awaken 对应启发：Phase 1 增加 `TurnStarted/TurnFinished` 时，可以先用最小状态初始化保证旧行为等价，再逐步把 terminal/transition reason 接进去。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:346-363；设计要点：每轮初始化或递增 `queryTracking`，包含 `chainId` 和 `depth`，并写回 `toolUseContext`；Awaken 对应启发：`QueryChainID` 与 `Depth` 应跟随 loop state 传播到工具执行和事件，而不是只存在于 LLM request。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:1207-1249；设计要点：重试/恢复路径通过设置新的 `State` 并写入 `transition.reason` 表达 `max_output_tokens_escalate`、`max_output_tokens_recovery`；Awaken 对应启发：`TransitionReason` 应覆盖“继续跑下一轮”的原因，便于测试断言恢复路径，而不是只在错误文案中体现。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:1484-1520；设计要点：工具阶段被 abort 或 hook 阻止时返回结构化 terminal reason，例如 `aborted_tools`、`hook_stopped`；Awaken 对应启发：`RunCancelled`、`RunError`、`RunFinished` 应携带 `TerminalReason`，前端可忽略但 transcript/runtime 可审计。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/query.ts:1704-1727；设计要点：达到 `maxTurns` 时返回 `{ reason: 'max_turns', turnCount }`，否则构造下一轮 `State` 并设置 `transition: { reason: 'next_turn' }`；Awaken 对应启发：Phase 1 的多轮工具调用测试应同时断言 `TurnCount` 增长和终止原因，避免无限 loop 与正常 next turn 混淆。

### Phase 2：AUTO 工具运行时

- [ ] 2.1 扩展 `ToolExecutionResult`
  - 进入条件：Phase 1 完成。
  - 执行项：
    - 在 `src/agentcore/tool_runtime.go` 扩展字段：
      - `Status`: `succeeded | denied | cancelled | failed`
      - `ErrorCode`
      - `DecisionSource`
      - `DecisionReason`
      - `StartedAt`
      - `FinishedAt`
      - `DurationMs`
      - `AffectedPaths`
      - `PatchSummary`
  - 完成条件：
    - 原有 `ToolResult` 事件仍能渲染。
    - tool error 测试能读取 `Status=failed`。
  - 解锁下一步：AUTO policy 可以留下可观测决策。

- [ ] 2.2 定义 `AutoToolPolicy`
  - 进入条件：2.1 完成。
  - 执行项：
    - 新增接口：
      - `Decide(ctx, ToolDecisionRequest) ToolDecision`
    - `ToolDecisionRequest` 包含：
      - `RunID`
      - `ThreadID`
      - `TurnID`
      - `ToolCallID`
      - `ToolName`
      - `RawInput`
      - `NormalizedInput`
      - `StaticBehavior`
      - `StaticReason`
      - `WorkspaceRoot`
      - `WritableRoots`
      - `IsReadOnly`
      - `IsDestructive`
      - `IsOpenWorld`
      - `RequiresUserInteraction`
    - `ToolDecision` 包含：
      - `Action`: `allow | deny | abort`
      - `Source`: `tool_rule | auto_allowlist | workspace_boundary | headless_default | policy_error`
      - `Reason`
      - `Retryable`
      - `UpdatedInput`
  - 完成条件：
    - 有默认 `AutoToolPolicy` 实现。
    - 单测覆盖 allow、deny、abort。
  - 解锁下一步：ToolRuntime 可以去掉主路径 approval。

- [ ] 2.3 改写 `PermissionAsk` 主路径
  - 进入条件：2.2 完成。
  - 执行项：
    - 在 `ToolRuntime.Execute()` 中：
      - 保留 `selectedTool.CheckPermission()`。
      - 当返回 `PermissionAsk` 时，不调用 `awaitApproval()`。
      - 改调用 `AutoToolPolicy.Decide()`。
      - 根据 `ToolDecision` 执行 allow / deny / abort。
    - `awaitApproval()` 只作为兼容 path 保留，不进入默认配置。
  - 完成条件：
    - 无 `ApprovalAwaiter` 时，`apply_patch` / shell 这类 ask 工具不会报 `approval required ... no approval awaiter`。
    - deny 会返回 error tool result。
    - Runtime 不进入 `awaiting_approval`。
  - 解锁下一步：主 Agent loop 可无审批执行。

- [ ] 2.4 发出 `ToolPermissionDecided`
  - 进入条件：2.3 完成。
  - 执行项：
    - 在 static permission 和 auto policy 后发事件。
    - 字段包含 tool call、static behavior、final action、source、reason、affected paths。
  - 完成条件：
    - Desktop 可以展示 policy 决策或忽略它。
    - 单测能捕获 `ToolPermissionDecided` 在 `ToolCallStarted` 和 `ToolResult` 之间。
  - 解锁下一步：工具执行链具备可观测性。

- [ ] 2.5 标准化 deny / abort tool result
  - 进入条件：2.4 完成。
  - 执行项：
    - 新增 helper：
      - `synthesizeDeniedToolResult`
      - `synthesizeAbortedToolResult`
      - `synthesizeValidationToolResult`
    - 文案统一：
      - `工具执行被 AUTO policy 拒绝: <reason>`
      - `工具执行已被取消`
      - `工具参数错误: <reason>`
  - 完成条件：
    - 所有错误 result 都 `IsError=true`。
    - 都可以通过 `ValidateToolResultPairing()`。
  - 解锁下一步：LLM 可以从失败工具结果继续收束。

#### Claude Code 参考设计

- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/Tool.ts:483-503；设计要点：每个 Tool 先暴露 `validateInput()`，再暴露 tool-specific `checkPermissions()`，通用权限逻辑放在 `permissions.ts`；Awaken 对应启发：`ToolRuntime.Execute()` 中应保留当前静态权限检查，再把 `PermissionAsk` 接到 `AutoToolPolicy.Decide()`，避免把工具私有校验和全局 AUTO policy 混在一起。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/types/permissions.ts:174-246；设计要点：权限结果显式分为 `allow | ask | deny`，`allow/ask` 可携带 `updatedInput`，`deny` 必须携带 `message` 和 `decisionReason`；Awaken 对应启发：`ToolDecision` 的 `Action/Source/Reason/UpdatedInput` 字段应保持结构化，以便 `ToolPermissionDecided` 和错误 tool result 共用同一决策事实。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/utils/permissions/permissions.ts:503-552；设计要点：未找到直接等价设计；Claude Code 的 `ask` 不是默认无审批执行，而是在 `dontAsk` 中转 deny，在 `auto/plan+auto` 中进入 classifier/fast-path，并保留需用户交互工具的手动路径；Awaken 对应启发：Phase 2 的 AUTO 主路径要比 Claude Code 更确定，必须明确 `RequiresUserInteraction`、headless、open-world 等场景的 fail-closed 行为。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/utils/permissions/permissions.ts:593-685；设计要点：auto mode 在运行 classifier 前先尝试 `acceptEdits` fast-path，并对安全 allowlist 工具直接 allow，同时记录 `tengu_auto_mode_decision`；Awaken 对应启发：默认 `AutoToolPolicy` 可先实现 deterministic allowlist/workspace-boundary 决策，再追加 LLM classifier 类能力，且每次决策都要可观测。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/services/tools/toolExecution.ts:916-1036；设计要点：工具执行前统一解析 hook/permission 结果，非 allow 时记录 `tool_decision`，然后合成 `tool_result` 且 `is_error: true`；Awaken 对应启发：`ToolPermissionDecided` 应位于 `ToolCallStarted` 和 `ToolResult` 之间，deny/abort 必须产出可配对的 error tool result，不能让 loop 悬空。
- Claude Code 文件：/Users/nolangrant/claudecode源代码/src/services/tools/toolExecution.ts:1128-1222；设计要点：权限返回的 `updatedInput` 会覆盖执行输入，然后才调用 `tool.call()`，并把 `toolUseID/userModified` 写入 tool context；Awaken 对应启发：`ToolDecision.UpdatedInput` 不应只是审计字段，allow 后要进入真实工具执行输入；同时 `ToolExecutionResult` 要记录 started/finished/duration，支撑 Phase 2 的可观测性。

### Phase 3：ContextEnvelope 分层上下文

- [ ] 3.1 定义 `ContextEnvelope`
  - 进入条件：Phase 2 完成。
  - 执行项：
    - 在 `src/agentcore` 或 `src/prompt` 定义：
      - `SystemBlocks`
      - `UserContextItems`
      - `SystemContextItems`
      - `Attachments`
      - `ToolSchemas`
      - `RuntimeSnapshot`
      - `CacheKeys`
  - 完成条件：
    - 类型能表达当前 `RequestBuilder` 已有 system prompt、runtime info、tools。
  - 解锁下一步：RequestBuilder 可逐步迁移。

- [ ] 3.2 将 `RequestBuilder` 改为先构建 envelope
  - 进入条件：3.1 完成。
  - 执行项：
    - `RequestBuilder.Build()` 内先调用 `BuildContextEnvelope()`。
    - 再由 `BuildLLMRequestFromEnvelope()` 输出现有 `LLMRequest`。
    - 保持现有外部签名尽量不变。
  - 完成条件：
    - 现有 request builder 测试通过。
    - LLM request 内容和迁移前等价。
  - 解锁下一步：可以加入 runtime delta 和 context window。

- [ ] 3.3 拆出 runtime snapshot
  - 进入条件：3.2 完成。
  - 执行项：
    - 从 `prompt.RuntimeInfo` 中明确拆出：
      - current date
      - working directory
      - model name
      - entry point
      - tool summary
      - workspace root
      - writable roots
    - 每轮请求前刷新。
  - 完成条件：
    - 同一 thread 下切 workspace 不允许复用。
    - tool summary 跟实际 ToolPool 一致。
  - 解锁下一步：上下文窗口估算可以看到完整请求体。

- [ ] 3.4 定义 M1 context delta
  - 进入条件：3.3 完成。
  - 执行项：
    - 只做 4 类 delta：
      - `runtime_snapshot_delta`
      - `tool_pool_delta`
      - `memory_hit`
      - `mcp_instruction_delta`
    - 当前项目没有 MCP runtime 时，先保留空 delta。
  - 完成条件：
    - delta 能被记录和注入，但不改变现有行为。
  - 解锁下一步：compact/resume 可以避免重复注入。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/query.ts:181-197`：未找到直接等价设计。Claude Code 没有单一 `ContextEnvelope` 类型，最接近的是 `QueryParams` 同时携带 `messages`、`systemPrompt`、`userContext`、`systemContext`、`toolUseContext`、`fallbackModel`、`taskBudget`。Awaken 对应启发：保留这些分层输入，但用显式 `ContextEnvelope` 把它们收束成可测试、可 debug 的中间结构。
- `/Users/nolangrant/claudecode源代码/src/context.ts:113-149`、`/Users/nolangrant/claudecode源代码/src/context.ts:152-188`：Claude Code 将 system context 和 user context 分开 memoize，system 侧放 git/cache breaker，user 侧放 CLAUDE.md/current date。Awaken 对应启发：`SystemContextItems`、`UserContextItems`、`CacheKeys` 应独立建模，避免 runtime snapshot、用户记忆和系统注入混在同一个 prompt 字符串里。
- `/Users/nolangrant/claudecode源代码/src/query.ts:449-467`：Claude Code 在请求前将 `systemContext` append 到 `systemPrompt`，并把 `systemPrompt/userContext/systemContext/toolUseContext/forkContextMessages` 作为 compact 的 cache-safe 参数。Awaken 对应启发：`BuildContextEnvelope()` 应先于 compact 和 request，且 compact 输入要拿到完整 envelope，而不是只看 messages。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1231-1267`：Claude Code 在 API 层动态构建工具 schema 并 normalize messages，说明 provider request builder 才是工具 schema 与消息格式落地的位置。Awaken 对应启发：`BuildLLMRequestFromEnvelope()` 应作为 provider-neutral envelope 到 provider request 的唯一边界。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1327-1379`：Claude Code 会按条件把 deferred tools、Chrome/MCP instruction、attribution/system prefix 注入最终 system/messages，同时尽量控制 prompt cache churn。Awaken 对应启发：M1 `tool_pool_delta`、`mcp_instruction_delta` 要进入 envelope 的 delta 区，而不是直接散落追加到 system prompt。

### Phase 4：上下文窗口与 compact

- [ ] 4.1 新增 `ContextWindowState`
  - 进入条件：Phase 3 完成。
  - 执行项：
    - 定义字段：
      - `ModelWindow`
      - `OutputReserve`
      - `EffectiveWindow`
      - `EstimatedTokens`
      - `WarningThreshold`
      - `AutoCompactThreshold`
      - `CompactTarget`
      - `LastAPIUsageTokens`
      - `EstimatedDeltaTokens`
      - `LastCompactBoundaryID`
      - `ConsecutiveCompactFailures`
      - `HasAttemptedReactiveCompact`
  - 完成条件：
    - 每次 `Build()` 能返回估算结果。
  - 解锁下一步：请求前可以发 warning/blocking。

- [ ] 4.2 扩展 token estimator
  - 进入条件：4.1 完成。
  - 执行项：
    - 基于 `context.EstimateTotalTokens()` 扩展：
      - messages content
      - tool call arguments
      - tool result content
      - system prompt
      - tool schema JSON
      - ContextEnvelope attachments
    - 不接 API count tokens。
  - 完成条件：
    - 单测覆盖纯文本、tool call、长 tool result、tool schema。
  - 解锁下一步：warning 和 compact 有数据依据。

- [ ] 4.3 发出 context window 事件
  - 进入条件：4.2 完成。
  - 执行项：
    - 每轮请求前发 `ContextEstimated`。
    - 超过 warning 发 `ContextWarning`。
    - 超过 blocking 且无法 compact 发 `ContextBlocked`。
  - 完成条件：
    - Runtime/SSE/Desktop 能接收事件。
    - Desktop 可先只记录，不必立即设计复杂 UI。
  - 解锁下一步：compact 可由阈值触发。

- [ ] 4.4 实现 durable compact
  - 进入条件：4.3 完成。
  - 执行项：
    - 新增 `CompactConversation()`：
      - 输入 messages、ContextEnvelope、target token。
      - 输出 summary message、tail messages、compact boundary。
    - M1 可先用 LLM summary 或 deterministic summary stub，测试用 stub。
    - boundary 写入 conversation。
  - 完成条件：
    - compact 后 `GetMessagesForModel()` 只取最近 boundary 后内容。
    - compact boundary 不破坏 tool_result pairing。
  - 解锁下一步：长会话可持续。

- [ ] 4.5 实现 reactive compact retry
  - 进入条件：4.4 完成。
  - 执行项：
    - 当 LLM 返回 prompt too long：
      - 如果本轮未尝试 reactive compact，则 compact 一次并重试。
      - 如果已尝试，则 terminal `prompt_too_long`。
    - 设置 `HasAttemptedReactiveCompact`，防止死循环。
  - 完成条件：
    - 单测覆盖一次 retry 成功和二次失败终止。
  - 解锁下一步：主 loop 可以处理估算失误。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/services/compact/autoCompact.ts:32-49`、`/Users/nolangrant/claudecode源代码/src/services/compact/autoCompact.ts:72-90`：Claude Code 先从模型窗口扣除输出保留 token，再计算 autocompact threshold。Awaken 对应启发：`ContextWindowState` 里的 `ModelWindow`、`OutputReserve`、`EffectiveWindow`、`AutoCompactThreshold` 应显式记录，避免只存一个粗略 token 数。
- `/Users/nolangrant/claudecode源代码/src/services/compact/autoCompact.ts:93-145`：`calculateTokenWarningState()` 同时返回剩余百分比、warning、error、autocompact、blocking 状态。Awaken 对应启发：`ContextEstimated`、`ContextWarning`、`ContextBlocked` 可以来自同一次窗口状态计算，减少 UI/runtime 口径不一致。
- `/Users/nolangrant/claudecode源代码/src/query.ts:365-467`：Claude Code 每轮先取 compact boundary 后的 messages，再依次应用 tool result budget、snip、microcompact、context collapse，最后才进入 autocompact。Awaken 对应启发：`GetMessagesForModel()` 应固定顺序处理 boundary、attachments、tool result 裁剪和 compact 视图，不能让各路径各自切历史。
- `/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:299-338`、`/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:596-624`：Claude Code 的 compact 结果包含 boundary marker、summary messages、保留消息、attachments、hook results，并用 `buildPostCompactMessages()` 统一 post-compact 顺序。Awaken 对应启发：`CompactConversation()` 的输出要直接表达 summary、tail messages、attachments 和 durable boundary，避免 compact 后的请求重组有第二套规则。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1062-1175`：Claude Code 对 prompt-too-long 先尝试 context-collapse drain，再 single-shot reactive compact；成功后用 post-compact messages 继续，失败才暴露 `prompt_too_long`。Awaken 对应启发：`HasAttemptedReactiveCompact` 必须进入 per-turn 状态，防止 prompt too long retry 死循环。
- `/Users/nolangrant/claudecode源代码/src/services/compact/grouping.ts:3-22`、`/Users/nolangrant/claudecode源代码/src/services/compact/grouping.ts:34-42`：Claude Code 按 API round 分组作为 compact/truncate 边界，并依赖 API 前 pairing repair 处理异常输入。Awaken 对应启发：durable compact boundary 要优先切在 tool_use/tool_result 安全边界，compact 前后都跑 `ValidateToolResultPairing()`。

### Phase 5：LLM request 与 stream hardening

- [ ] 5.1 Request 前统一 pairing check
  - 进入条件：Phase 4 完成。
  - 执行项：
    - 保留现有 `ValidateToolResultPairing()`。
    - 在 compact 后、request 前都跑一次。
    - 失败时 terminal `tool_pairing_error`。
  - 完成条件：
    - 现有 `TestRunnerRejectsInvalidToolResultPairingBeforeLLM` 仍通过。
  - 解锁下一步：stream tool_use 可以放心进入工具执行。

- [ ] 5.2 统一 `LLMRequest` 中的上下文来源
  - 进入条件：5.1 完成。
  - 执行项：
    - `LLMRequest` 保持：
      - `SystemPrompt`
      - `Messages`
      - `Tools`
      - `Model`
      - `Temperature`
      - `MaxTokens`
    - 增加内部 metadata：
      - `ContextWindowState`
      - `EnvelopeDebug`
    - 不暴露给 API provider。
  - 完成条件：
    - OpenAI 和 Anthropic request 输出保持兼容。
  - 解锁下一步：stream reducer 可记录 usage 和 context。

- [ ] 5.3 完善 stream tool_use 收集
  - 进入条件：5.2 完成。
  - 执行项：
    - OpenAI stream 和 Anthropic stream 都必须：
      - 发 tool args delta 事件。
      - 在 block 结束后产生完整 tool call。
      - 没有 tool call 时 `IsFinished=true`。
    - 对空 response 仍返回 `ErrEmptyResponse`。
  - 完成条件：
    - Anthropic tool_use 流测试通过。
    - OpenAI tool_call 流测试通过。
  - 解锁下一步：Runner loop 可稳定执行工具。

- [ ] 5.4 实现 max output recovery
  - 进入条件：5.3 完成。
  - 执行项：
    - Provider 暴露 stop reason 或截断信号后：
      - 最多追加 1 到 3 次 continuation message。
      - 超限 terminal `model_error` 或 `max_output_reached`。
    - 如果当前 provider 无 stop reason，先保留接口和测试 stub。
  - 完成条件：
    - 不会无限续写。
  - 解锁下一步：长输出不会半截结束。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1259-1301`：Claude Code 在 API request 前 normalize messages，并在模型能力差异下剥离不支持的 tool-search 字段，最后调用 `ensureToolResultPairing()` 修复 resume/teleport 造成的 pairing mismatch。Awaken 对应启发：`LLMRequest` 构建前要有统一 pairing check 和 provider-aware normalize，失败时走明确 terminal。
- `/Users/nolangrant/claudecode源代码/src/utils/messages.ts:5120-5135`、`/Users/nolangrant/claudecode源代码/src/utils/messages.ts:5301-5326`、`/Users/nolangrant/claudecode源代码/src/utils/messages.ts:5437-5460`：`ensureToolResultPairing()` 同时处理缺失 tool_result、孤儿 tool_result、重复 id，并支持 strict mode 下直接 throw。Awaken 对应启发：Phase 5 可以先采用严格校验作为安全闸，后续再决定是否引入 synthetic error tool result repair。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1538-1728`：Claude Code 用 `paramsFromContext()` 汇总 betas、max tokens、thinking、cache breakpoints、context_management、output_config，并只在最终 API 参数里展开。Awaken 对应启发：`ContextWindowState` 和 `EnvelopeDebug` 应保留为内部 metadata，provider request 只暴露 provider 真正需要的字段。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1818-1836`、`/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1933-2210`：Claude Code 使用 raw stream 避免 SDK 对 partial JSON 的重复解析，自己累积 `content_block_start/delta/stop`，在 block stop 时产出完整 assistant message。Awaken 对应启发：Anthropic/OpenAI stream reducer 都要记录 args delta，但只有 block 完成后才生成完整 tool call。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:1868-1927`、`/Users/nolangrant/claudecode源代码/src/services/api/claude.ts:2308-2364`：Claude Code 为 stream idle 设置 watchdog，超时释放资源并触发 fallback；同时检测无 `message_start` 或无完整 content block 的空流。Awaken 对应启发：stream hardening 要区分 timeout、empty response、正常 end_turn，并把空响应映射到 `ErrEmptyResponse` 或可观测 terminal。
- `/Users/nolangrant/claudecode源代码/src/query.ts:709-740`、`/Users/nolangrant/claudecode源代码/src/query.ts:893-929`、`/Users/nolangrant/claudecode源代码/src/query.ts:1011-1029`：Claude Code 在 streaming fallback 或 abort 时清理 partial assistant/tool state，丢弃 pending executor，必要时合成缺失 tool_result，避免 orphan tool_result 进入下一轮。Awaken 对应启发：stream fallback、用户取消、模型 fallback 都必须共享一套 partial tool_use 收尾逻辑。

### Phase 6：Runner 主 loop 显式化

- [ ] 6.1 将 for loop 改为 `TurnState` 驱动
  - 进入条件：Phase 5 完成。
  - 执行项：
    - 保留 `for i := 0; i < MaxThoughts; i++` 行为。
    - 每轮开始生成 `TurnStarted`。
    - 每轮结束生成 `TurnFinished`。
    - 每次 continue 都写 `TransitionReason`。
  - 完成条件：
    - 多轮工具 loop 测试能看到 turn 事件。
  - 解锁下一步：terminal reason 可以准确落点。

- [ ] 6.2 收敛工具执行后的继续条件
  - 进入条件：6.1 完成。
  - 执行项：
    - `len(response.ToolCalls) > 0` 后：
      - 执行工具。
      - 写 `Conversation.ToolExchange()`。
      - pairing check。
      - 如果 ctx cancelled，终止。
      - 否则 `TransitionReason=tool_result_continuation`。
  - 完成条件：
    - tool error 仍会进入下一轮 LLM。
    - cancel in batch 不再发第二次 LLM。
  - 解锁下一步：cancel 和 max turns 可统一。

- [ ] 6.3 统一 cancel 两阶段
  - 进入条件：6.2 完成。
  - 执行项：
    - 流式阶段 cancel：
      - 补 pending tool result。
      - terminal `aborted_streaming`。
    - 工具阶段 cancel：
      - 当前工具完成或 synthesize cancelled result。
      - 后续工具不执行。
      - terminal `aborted_tools`。
  - 完成条件：
    - 现有 cancel synthetic tool result 测试通过。
    - 事件 terminal reason 正确。
  - 解锁下一步：Runtime status 可准确表达 cancelled。

- [ ] 6.4 统一 max turns
  - 进入条件：6.3 完成。
  - 执行项：
    - 每次工具批次结束后检查下一轮是否超过 `MaxThoughts`。
    - 超过时：
      - 所有已产生 tool result 保留。
      - terminal `max_turns`。
      - 返回可读错误。
  - 完成条件：
    - 不丢最后一批 tool result。
  - 解锁下一步：主 loop 所有终止路径可枚举。

- [ ] 6.5 保留 XML fallback 但降级为兼容路径
  - 进入条件：6.4 完成。
  - 执行项：
    - `handleXMLFallback()` 保留。
    - 给 XML fallback 产生的 tool call 也补 `ToolPermissionDecided` 和 `TurnFinished`。
    - 文档标注它不是主 tool_use 路径。
  - 完成条件：
    - XML fallback 现有测试通过。
  - 解锁下一步：可以做 transcript/resume。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/query.ts:551-558`：设计要点：Claude Code 不信任 provider 的 `stop_reason === "tool_use"`，而是在 streaming 中看到 `tool_use` block 后设置 `needsFollowUp` 作为 loop 继续信号。Awaken 对应启发：Phase 6 的 `TurnState` / `TransitionReason` 应以已解析 tool call 为准，不能只依赖 provider stop reason。
- `/Users/nolangrant/claudecode源代码/src/query.ts:654-708`：设计要点：主 loop 内把一次模型调用包在 `while (attemptWithFallback)` 中，显式传入 messages、system prompt、tools、abort signal、MCP tools、agent definitions、task budget 等上下文。Awaken 对应启发：Runner 主 loop 应把每轮 LLM sampling 的输入上下文和可变状态集中在 TurnState，而不是散落在 for loop 局部变量里。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1011-1052`：设计要点：streaming 阶段 abort 优先处理，必要时补齐 missing tool result / consume remaining tool executor updates，最后返回 `aborted_streaming`。Awaken 对应启发：Phase 6.3 的 streaming cancel 必须先修复 pairing，再落 terminal reason。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1380-1409`：设计要点：模型产出的 tool_use 统一进入 tool executor，tool update 被 yield 后再 normalize 成下一轮可回传 API 的 user/tool_result。Awaken 对应启发：Phase 6.2 的工具批次结束后，应统一写 `Conversation.ToolExchange()` 并只从规范化 tool_result 进入下一轮。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1484-1515`：设计要点：工具阶段 abort 和 streaming abort 分开处理，工具阶段返回 `aborted_tools`，并在 abort 时仍可发 max-turns attachment。Awaken 对应启发：Phase 6.3 应把 `aborted_streaming` 与 `aborted_tools` 作为不同 terminal path 验收。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1704-1728`：设计要点：工具结果准备好后先检查 `maxTurns`，未超限才构造下一轮 `State`，并写入 `transition: { reason: "next_turn" }`。Awaken 对应启发：Phase 6.4 的 max turns 应在保留最后一批 tool result 后决定是否继续，并给每个 continue 写明确 transition reason。

### Phase 7：Transcript 与 resume

- [ ] 7.1 定义 durable transcript record
  - 进入条件：Phase 6 完成。
  - 执行项：
    - 新增 JSONL record：
      - `record_type`
      - `session_id`
      - `thread_id`
      - `run_id`
      - `turn_id`
      - `message_id`
      - `parent_message_id`
      - `role`
      - `content`
      - `tool_calls`
      - `tool_call_id`
      - `tool_name`
      - `is_error`
      - `event`
      - `terminal_reason`
      - `timestamp`
    - 不替代 Runtime in-memory history，先并行写。
  - 完成条件：
    - 每个 user/assistant/tool message 都能落盘。
  - 解锁下一步：跨进程 resume。

- [ ] 7.2 实现 transcript writer
  - 进入条件：7.1 完成。
  - 执行项：
    - 在 `Conversation` append message 时，或者 `Runner` turn 完成后写增量。
    - compaction 或修复 pairing 时允许全量 rewrite 或 append boundary。
  - 完成条件：
    - 单测覆盖增量追加和重复写入幂等。
  - 解锁下一步：resume loader 可读。

- [ ] 7.3 实现 resume loader
  - 进入条件：7.2 完成。
  - 执行项：
    - 从 transcript 还原：
      - messages
      - compact boundary
      - last context window state
      - pending terminal state
    - 修复或拒绝：
      - dangling tool_use
      - orphan tool_result
      - duplicate tool_result
  - 完成条件：
    - 还原后 `ValidateToolResultPairing()` 通过。
  - 解锁下一步：Runtime thread transcript 可跨重启。

- [ ] 7.4 接入 RunService thread reuse
  - 进入条件：7.3 完成。
  - 执行项：
    - 新 thread 创建时建立 transcript path。
    - 复用 thread 时从内存 Agent 优先。
    - 进程重启后从 transcript 恢复 Agent conversation。
  - 完成条件：
    - 重启后继续同一 thread，不丢上下文。
  - 解锁下一步：Desktop resume 行为可稳定。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:128-155`：设计要点：Claude Code 明确 transcript message 的单一判定标准，user/assistant/attachment/system 进入 transcript，progress 属于 UI ephemeral，不参与 parentUuid chain。Awaken 对应启发：Phase 7.1 的 durable transcript record 应区分可恢复对话记录和纯 UI/进度事件，避免 resume 时把 ephemeral event 当上下文。
- `/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:202-225`：设计要点：transcript path 由当前 sessionId 与 sessionProjectDir/originalCwd 共同决定，当前 session 和其他 session 的路径解析不同。Awaken 对应启发：Phase 7.4 新建/复用 thread 时，需要把 thread_id/run_id/session path 绑定成稳定契约，避免跨进程 resume 读错路径。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:436-463`：设计要点：用户消息在进入 query loop 前先写 transcript，防止进程在 API 返回前被杀导致 `/resume` 找不到会话。Awaken 对应启发：Phase 7.2 writer 应在 user prompt 被 RunService 接受后立即落盘，而不是等 assistant 首 token。
- `/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:1408-1448`：设计要点：`recordTranscript()` 会清洗消息、查重已写 UUID、维护 parentUuid hint，并返回最后实际记录的 chain participant。Awaken 对应启发：Phase 7.2 的增量追加要幂等，Phase 7.1 的 `message_id` / `parent_message_id` 需要支持 rewrite/compact 后仍能恢复链。
- `/Users/nolangrant/claudecode源代码/src/utils/conversationRecovery.ts:148-231`：设计要点：resume 反序列化会过滤 unresolved tool_use、孤立 thinking、空白 assistant，并把 mid-turn interruption 转成合成的继续 user message。Awaken 对应启发：Phase 7.3 loader 需要修复或拒绝 dangling tool_use / orphan tool_result，并给中断恢复定义明确策略。
- `/Users/nolangrant/claudecode源代码/src/utils/conversationRecovery.ts:405-440`：设计要点：Claude Code 从 JSONL transcript chain-walk，选择最新非 sidechain leaf，再通过 parent chain 构造 conversation。Awaken 对应启发：Phase 7.3/7.4 应从 durable transcript 还原 thread 的有序消息链，而不是只按文件顺序盲读。

### Phase 8：Runtime / Server / Desktop 集成

- [ ] 8.1 Runtime 状态去 approval 主路径
  - 进入条件：Phase 2 至 Phase 6 完成。
  - 执行项：
    - AUTO 模式下不得进入 `RunStatusAwaitingApproval`。
    - approval API 保留，但不是主路径。
    - 新增或复用 status 表达 policy/tool running。
  - 完成条件：
    - AUTO 工具 ask 不触发 `ApprovalRequested`。
  - 解锁下一步：Desktop 不出现审批卡片阻塞。

- [ ] 8.2 Server SSE 支持新增事件
  - 进入条件：8.1 完成。
  - 执行项：
    - `mapRuntimeEvent()` 映射新增事件到 AG-UI frame。
    - unknown event fallback 不丢原始信息。
    - replay `after_seq` 仍单调。
  - 完成条件：
    - SSE replay 测试覆盖新增事件。
  - 解锁下一步：Desktop reducer 可消费。

- [ ] 8.3 Desktop shared type 更新
  - 进入条件：8.2 完成。
  - 执行项：
    - 更新 `apps/desktop/src/shared/agui.ts` 和 runtime 类型：
      - context estimated/warning/blocked
      - compact events
      - tool permission decided
      - terminal reason
  - 完成条件：
    - TypeScript 编译通过。
  - 解锁下一步：reducer 可投影。

- [ ] 8.4 Desktop reducer 投影
  - 进入条件：8.3 完成。
  - 执行项：
    - context events 投影到 Inspector。
    - compact events 投影到 narrative。
    - tool permission decided 投影到 tool record。
    - terminal reason 投影到 run summary。
    - approval records 在 AUTO path 下不增长。
  - 完成条件：
    - reducer tests 覆盖 AUTO allow、AUTO deny、compact、terminal reason。
  - 解锁下一步：UI 可验收。

- [ ] 8.5 Inspector 展示
  - 进入条件：8.4 完成。
  - 执行项：
    - 展示：
      - current turn
      - tool calls
      - auto decision
      - context tokens
      - compact boundary
      - terminal reason
    - 不做复杂新视觉，只复用现有面板。
  - 完成条件：
    - 用户能看懂 run 为什么继续、为什么结束、工具为什么执行或拒绝。
  - 解锁下一步：端到端 smoke。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:175-183`：设计要点：`QueryEngine` 是 conversation lifecycle 和 session state 的拥有者，一个 conversation 一个 engine，messages/file cache/usage 跨 turn 持续。Awaken 对应启发：Phase 8 的 Runtime/RunService 应明确谁拥有 run state，Server/Desktop 只消费投影。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:675-731`：设计要点：`QueryEngine` 从 `query()` async generator 消费内部消息，同时负责 transcript 写入；assistant 消息 fire-and-forget，user/system boundary 则 await 保序。Awaken 对应启发：Phase 8.2 `mapRuntimeEvent()` 应把 runtime event 到 SSE frame 的映射放在边界层，同时保留 transcript 写入顺序约束。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:757-969`：设计要点：统一 switch 不同 message type，assistant/user/stream_event/attachment/system/tool_use_summary 分别投影成 SDK 可消费事件，并把 compact boundary、api retry、max turns 转成结构化 result/system。Awaken 对应启发：Phase 8.3/8.4 的 shared type 与 reducer 应以事件类型分流，unknown event 保留原始信息。
- `/Users/nolangrant/claudecode源代码/src/cli/structuredIO.ts:126-162`：设计要点：StructuredIO 定义 SDK stdio 协议，并用 outbound queue 保证 `control_request` 与 stream events 只有一个写出通道。Awaken 对应启发：Phase 8.1 保留 approval API 时，应避免 approval/control 事件绕过 Runtime 主事件序列造成 Desktop 阻塞态漂移。
- `/Users/nolangrant/claudecode源代码/src/cli/transports/SSETransport.ts:149-160`：设计要点：SSETransport 读使用 SSE，写使用 HTTP POST，并支持 reconnect 时通过 Last-Event-ID 恢复。Awaken 对应启发：Phase 8.2 的 SSE `after_seq` replay 应作为正式协议能力，不只是测试辅助。
- `/Users/nolangrant/claudecode源代码/src/cli/transports/SSETransport.ts:344-388`：设计要点：SSE reader 增量解析 frame、维护 `lastSequenceNum`、去重已见 sequence，并把 frame event/data 分发。Awaken 对应启发：Phase 8.2 Server SSE 新事件应保持 seq 单调、可去重、可诊断 unknown frame。

### Phase 9：验收 smoke

- [ ] 9.1 Smoke：纯聊天
  - 进入条件：Phase 8 完成。
  - 执行项：
    - 输入普通问题。
    - 模型无 tool_use。
  - 完成条件：
    - 有 stream 文本。
    - terminal `completed`。
    - transcript 有 user/assistant。

- [ ] 9.2 Smoke：读工具 AUTO allow
  - 进入条件：9.1 完成。
  - 执行项：
    - 模型调用 `file_read` 或 read-only 测试工具。
  - 完成条件：
    - 发出 `ToolPermissionDecided: allow`。
    - 工具执行。
    - tool_result 回写。
    - 第二轮 LLM 输出最终回答。

- [ ] 9.3 Smoke：写工具 AUTO deny
  - 进入条件：9.2 完成。
  - 执行项：
    - 构造越界 path 或 destructive 工具调用。
  - 完成条件：
    - 不进入 approval。
    - 发出 `ToolPermissionDecided: deny`。
    - 生成 error tool_result。
    - 模型能继续收束或解释失败。

- [ ] 9.4 Smoke：写工具 allowlist allow
  - 进入条件：9.3 完成。
  - 执行项：
    - 对 workspace 内安全写工具配置 allowlist。
    - 模型执行写入。
  - 完成条件：
    - patch preview 仍出现。
    - 工具成功。
    - artifact / turn diff 保留。

- [ ] 9.5 Smoke：长上下文 compact
  - 进入条件：9.4 完成。
  - 执行项：
    - 构造超过 autocompact 阈值的消息历史。
  - 完成条件：
    - 发出 `ContextWarning`。
    - 发出 `CompactStarted` / `CompactFinished`。
    - 产生 compact boundary。
    - 下一轮 request 只使用 boundary 后 summary + tail。

- [ ] 9.6 Smoke：prompt too long reactive compact
  - 进入条件：9.5 完成。
  - 执行项：
    - 模拟 provider 返回 prompt too long。
  - 完成条件：
    - 第一次触发 reactive compact retry。
    - 第二次失败 terminal `prompt_too_long`。
    - 不死循环。

- [ ] 9.7 Smoke：cancel during streaming
  - 进入条件：9.6 完成。
  - 执行项：
    - 流式输出中 cancel。
  - 完成条件：
    - terminal `aborted_streaming` 或 `cancelled`。
    - transcript 可恢复。

- [ ] 9.8 Smoke：cancel during tool batch
  - 进入条件：9.7 完成。
  - 执行项：
    - 第一个工具触发 cancel，后续工具不执行。
  - 完成条件：
    - 已开始工具有 result。
    - 未执行工具有 synthetic cancelled tool_result。
    - pairing 校验通过。

- [ ] 9.9 Smoke：100 轮工具 loop
  - 进入条件：9.8 完成。
  - 执行项：
    - 用测试 LLM 连续返回工具调用。
  - 完成条件：
    - 不出现 orphan tool_result。
    - 不进入 approval。
    - max turns 能按预期终止。
    - 内存和事件数量可控。

- [ ] 9.10 Smoke：restart resume
  - 进入条件：9.9 完成。
  - 执行项：
    - 跑一段工具会话。
    - 停进程。
    - 从 transcript 恢复 thread。
    - 继续提问。
  - 完成条件：
    - 不重复注入 memory/context delta。
    - pairing 校验通过。
    - 上下文延续。

#### Claude Code 参考设计

- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:1135-1155`：设计要点：未找到直接等价的 Phase 9 smoke 清单；最接近的是 SDK result envelope，成功结果包含 `result`、`stop_reason`、`session_id`、usage、permission denials、structured output。Awaken 对应启发：Phase 9.1/9.2 应验收最终 terminal/result frame，而不只看 UI 文本出现。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:841-873`：设计要点：max turns 被投影为 `error_max_turns` result，包含 duration、turn count、stop reason、session id、usage、permission denials 和错误文本。Awaken 对应启发：Phase 9.9 的 100 轮工具 loop 应验收结构化 `max_turns` terminal，而不是只断言 loop 停止。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1119-1175`：设计要点：prompt-too-long / media-size reactive compact 只尝试一次恢复，成功则产生 post-compact messages 并继续，失败则 yield withheld error 并返回 `prompt_too_long` 或 `image_error`。Awaken 对应启发：Phase 9.5/9.6 要同时验收 compact boundary、retry 次数和不死循环。
- `/Users/nolangrant/claudecode源代码/src/query.ts:1011-1052`：设计要点：streaming cancel 会补齐 missing tool result 或 consume executor 剩余结果，再返回 `aborted_streaming`。Awaken 对应启发：Phase 9.7 不应只看 cancel 状态，还要验收 transcript 可恢复和 pairing 不破。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts:1070-1080`：设计要点：发出最终 result 前先 flush transcript，因为 Desktop 收到 result 后可能立刻杀掉 CLI 进程。Awaken 对应启发：Phase 9.10 restart resume smoke 应把“停进程前 transcript 已刷盘”作为验收点。
- `/Users/nolangrant/claudecode源代码/src/utils/conversationRecovery.ts:456-592`：设计要点：resume loader 支持从最近会话、指定 session、LogOption 或 JSONL path 恢复，并返回 messages、interruption state、file history、context collapse、agent/worktree metadata。Awaken 对应启发：Phase 9.10 应覆盖进程重启后从 transcript 恢复 thread，并检查上下文、metadata 和 tool pairing 延续。

## 模块交付顺序

必须按以下顺序合并，避免大爆炸：

1. Phase 0：基线锁定。
2. Phase 1：状态和事件，只加字段，不改变行为。
3. Phase 2：AUTO ToolRuntime，先解决无审批工具闭环。
4. Phase 3：ContextEnvelope，先等价迁移 RequestBuilder。
5. Phase 4：ContextWindow 和 compact。
6. Phase 5：LLM request/stream hardening。
7. Phase 6：Runner loop 显式化。
8. Phase 7：durable transcript/resume。
9. Phase 8：Runtime/SSE/Desktop 投影。
10. Phase 9：端到端 smoke。

## 测试矩阵

### Go 单测

- [ ] `go test ./src/context`
  - pairing pass / missing / orphan / name mismatch。
- [ ] `go test ./src/agentcore`
  - native tool loop。
  - Anthropic tool_use loop。
  - XML fallback loop。
  - AUTO allow。
  - AUTO deny。
  - cancel synthetic result。
  - max turns。
  - context warning / compact。
- [ ] `go test ./src/runtime`
  - thread busy。
  - cancel。
  - SSE history。
  - AUTO path 不进入 awaiting approval。
- [ ] `go test ./src/server`
  - SSE frame seq。
  - new event mapping。
- [ ] `go test ./src/llm`
  - OpenAI stream tool call。
  - Anthropic stream tool_use。
  - provider error mapping。
- [ ] `go test ./src/tool ./src/tools/...`
  - permission context。
  - workspace boundary。
  - read-only allow。
  - dangerous write deny。

### Desktop 测试

- [ ] Runtime reducer：
  - tool permission decided。
  - AUTO deny tool result。
  - compact events。
  - terminal reason。
  - thread switch keeps view.
- [ ] Thread transcript replay：
  - multiple runs。
  - after_seq。
  - resumed terminal state。
- [ ] App smoke：
  - pure chat。
  - tool loop。
  - cancel。
  - restored thread。

### 真实 LLM 验收

按照项目 `AGENTS.md`，runtime / agent loop / tool execution / SSE 验收需要真实 LLM。

- [ ] Anthropic Messages 协议真实 smoke。
- [ ] OpenAI-compatible 协议真实 smoke。
- [ ] read-only tool AUTO allow。
- [ ] denied tool 生成 tool_result 后模型能继续回答。
- [ ] 长上下文触发 compact 后继续回答。

## 完成定义

主 Agent 完整闭环完成时，必须同时满足：

- [ ] 用户输入能创建 run 并启动主 Agent loop。
- [ ] 主 Agent 能多轮调用 LLM。
- [ ] 主 Agent 能收集原生 provider tool_use / tool_call。
- [ ] 工具执行完全 AUTO，不等待用户审批。
- [ ] allow / deny / abort 都有 `ToolPermissionDecided`。
- [ ] 所有工具结果都合法回写为 tool_result。
- [ ] 每次 LLM 请求前 pairing 校验通过。
- [ ] 上下文窗口可估算、可 warning、可 compact、可 blocked。
- [ ] compact 产生 durable boundary。
- [ ] prompt too long 最多 reactive retry 一次，不死循环。
- [ ] cancel 在 streaming 和 tool batch 阶段都能收束。
- [ ] terminal reason 完整落到 Runtime/SSE/Desktop。
- [ ] transcript 可以跨进程恢复。
- [ ] Desktop 能显示 stream、tool、AUTO decision、compact、terminal 状态。
- [ ] 所有 smoke 通过。

## 风险和处理

- 风险：把 `ask` 直接改成 `allow`，导致越界写入。
  - 处理：必须经过 `AutoToolPolicy`，默认 fail closed。
- 风险：compact 后丢 tool_result pairing。
  - 处理：compact 前后都跑 `ValidateToolResultPairing()`。
- 风险：prompt too long recovery 死循环。
  - 处理：`HasAttemptedReactiveCompact` 和 `ConsecutiveCompactFailures` 必须落状态。
- 风险：Desktop 仍显示 approval 卡片，用户以为需要审批。
  - 处理：AUTO path 不发 `ApprovalRequested`，只发 `ToolPermissionDecided`。
- 风险：resume 只恢复聊天文本，丢工具协议状态。
  - 处理：transcript record 必须保存 tool call、tool result、boundary 和 terminal metadata。
- 风险：一次性改太多导致现有工具 loop 回归。
  - 处理：每个 phase 结束都跑已有 runner/runtime/reducer 测试。
