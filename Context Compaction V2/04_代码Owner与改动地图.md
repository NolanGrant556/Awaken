# 代码 Owner 与改动地图

## 1. 当前主要 owner

| 责任 | 当前 owner |
| --- | --- |
| 窗口、阈值、估算、provider anchor | `src/agentcore/context_window.go`、`context_usage.go` |
| proactive/manual/reactive governor | `src/agentcore/context_engine.go`、`runner.go` |
| summary、tail、pairing、media stub | `src/agentcore/context_compactor.go`、`context_summary_prompt.go` |
| 完整请求组装与分类投影 | `src/agentcore/request_builder.go` |
| InstructionSnapshot | `src/instructions/`、`src/agentcore`、`src/runtime/context_epoch*` |
| RunPlan | `src/agentcore/run_plan.go`、对应 tool/runtime service |
| Memory recall | `src/memory/recall.go`、agent/run wiring |
| Skill context | `src/agentcore/skill_context.go`、`src/skillruntime` |
| Hook snapshot/receipt | `src/hookruntime`、`src/pluginhooks`、`src/agentcore` 的 hook projection |
| active permission/input gate | `src/agentcore` gate/state owner 与对应 runtime projection |
| Subagent handoff | `src/agentcore/runner.go`、`src/subagentruntime/manager.go` |
| Journal/replay | `src/runtime/journal_store.go`、SemanticJournal recorder |
| Context Epoch | `src/runtime/context_epoch_store.go`、`context_snapshot_refresh.go` |
| provider lowering/usage/opaque state | `src/llm/openai_chat.go`、`openai_responses.go`、`anthropic_messages.go` |
| HTTP/SSE 与工具注册 | `src/server`、`src/protocol/agui` |
| Desktop UI/诊断 | `apps/desktop`、`Awaken Design System` |

## 2. 建议新增模块

最终命名由实施 Agent 在核对 package 边界后决定，建议职责如下：

| 建议模块 | 职责 |
| --- | --- |
| `context_bundle.go` | Bundle V2 schema、digest、validate、兼容 reader，包括 Hook snapshot ref 与 active gate refs |
| `context_checkpoint.go` | typed facts、rolling merge、coverage receipt |
| `context_turns.go` | 完整 turn grouping、protected turn 选择 |
| `context_budget_allocator.go` | mandatory floor、全局预算、缩减策略 |
| `context_history_locator.go` | boundary/sequence locator 与安全投影 |
| `src/agentcore/context_native_compaction.go` | native compact 调用编排、bundle ref、identity/digest、native→local fallback |
| `src/llm/native_compaction.go` | exact OpenAI official Responses capability、`/responses/compact` wire 与 opaque replay |
| `src/runtime/context_compaction_store.go` | journal-first bundle persistence/replay/atomic install coordination |

不要把 Memory、Skill、Subagent 的业务状态复制进 agentcore。Bundle 只持有对应 authority 的冻结 snapshot/ref，并通过明确接口安装/恢复。

## 3. Phase 对应修改面

### Phase 1：Bundle

- `src/agentcore/runner.go`
- `src/agentcore/context_engine.go`
- `src/runtime/journal_store.go`
- `src/runtime/context_epoch_store.go`
- Memory/Skill/Hook/Subagent snapshot/install interfaces

### Phase 2：Checkpoint

- `src/agentcore/context_summary_prompt.go`
- `src/agentcore/context_compactor.go`
- RunPlan、tool outcome、Memory receipt、Skill activation、Subagent snapshot readers

### Phase 3：Turns/History

- `src/agentcore/context_window.go`
- `src/agentcore/context_compactor.go`
- `src/runtime` Journal query owner
- `src/tool`/`src/tools` root-only tool contract
- `src/server` tool wiring

### Phase 4：Hard 10%

- `src/agentcore/context_window.go`
- `src/agentcore/context_engine.go`
- `src/agentcore/request_builder.go`
- `src/agentcore/context_usage.go`
- Skill/Memory/tool progressive disclosure providers

### Phase 5：Native compaction

- capability/model profile declarations
- `src/llm/openai_responses.go`
- request policy/purpose wiring
- provider opaque state persistence identity
- custom endpoint negative fixtures

### Phase 6：Desktop

- shared runtime DTOs
- Main/preload bridge only if新增字段需要
- Renderer context panel/status reducer
- Design System 只负责展示 props，不拥有压缩状态机

## 4. 必须同步的正式文档

只更新发生事实变化的唯一权威：

- `docs/02_主智能体闭环/上下文窗口与压缩.md`
- `docs/02_主智能体闭环/提示词缓存与成本治理.md`
- `docs/02_主智能体闭环/AGENTS指令与运行快照.md`
- `docs/02_主智能体闭环/运行计划与任务进度设计契约.md`
- `docs/02_主智能体闭环/原生Subagent产品与架构契约.md`
- `docs/11_长期记忆/长期记忆与Dream设计契约.md`
- `docs/08_技能/` 对应激活/恢复权威
- `docs/04_工具权限与审批/` 当前线程历史工具与 progressive disclosure 权威
- `docs/05_模型与供应商配置/模型配置与调用.md`

本目录不得在实现后继续复制这些当前事实。

## 5. Dirty work 隔离

创建本计划时 Awaken 工作树已有 Plugin Console、MCP auth、plugin setup、server handlers 等用户修改。实施 Agent必须：

- 不 reset/restore/clean；
- 优先使用 `git diff -- <task path>` 核对重叠；
- 若 Phase 3/5 必须修改与当前 dirty work 重叠的 server/MCP/provider 文件，先报告精确冲突面；
- staging 仅包含本任务文件；
- 完成前报告 cwd、branch、HEAD、diff 和未触碰的用户修改。
