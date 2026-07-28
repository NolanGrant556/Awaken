# 执行模式与 MCP 来源边界

## 1. Inline

inline 激活把调用时读取的 Skill 正文和包根加入当前 run 的上下文块。它继承父 run 已固定的模型、工具可用性快照、环境快照、TaskScope 与取消边界；Skill 元数据不能扩大或收窄这些边界。环境快照中的 source/root 只用于记录本地发现来源，不代替调用时的锚定资源读取。

bundled `code-search` 当前固定使用 inline 主上下文。原生 Subagent 已有独立的只读任务运行时和 Desktop 入口，但尚未把 Skill fork 迁移到同一 manager；该 Skill 在统一底座和对应真实 Desktop 验收完成前不迁移为 fork，后续迁移必须同时复核父子上下文隔离、工具继承、来源回传和取消行为。

正式原生 Subagent 的任务注册、并发、控制、持久事件和 Desktop UI 合同由 [原生 Subagent 产品与架构契约](../02_主智能体闭环/原生Subagent产品与架构契约.md) 拥有。Skill fork 当前仍保持同步专用链路，不能把两个生命周期或事件 family 混写为同一能力。

## 2. Fork

`context: fork` 当前创建真实 child agent，不再静默降级为 inline。run-scoped `ChildAgentStarter` 优先于 Manager fallback；没有 starter 时返回 `skill_fork_unavailable`。

handoff 包含正文、根目录、锚定路径、参数、agent/model/effort 和工具提示。child 拥有独立上下文，父 run 只接收明确的 child 结果；父 run 的后续 SkillContextBlocks 不混入 child 正文。父子工具能力仍由创建 child 时的显式继承/收窄契约决定，Skill 文本本身不是权限。child 获取的 pinned environment 元数据不授权它预加载根内其他 Skill 或绕过 TaskScope/resource reader。

## 3. MCP 动态 Skill

MCP bridge 把 server 的 prompt/resource 动态注册为 `mcp__<server>__...` Skill：

- 注册时只持有目录元数据和 body fetcher；
- 调用时才请求 prompt messages 或读取 resource 正文；
- server disconnect/disable/reconcile 时注销动态条目；
- re-register 保留进程内最近调用时间；
- MCP readiness 不作为本地 Skill 的加载门禁；
- MCP 正文中的 shell 文本只是说明，宿主不会在 bridge 内直接执行。

MCP Skill 依旧通过统一 `skill` 工具调用；它不会绕过 MCP transport、认证或 ToolRuntime。

## 4. 当前不支持或已知缺口

- MCP WebSocket transport 仍不支持；in-process 只用于测试。
- 外部 MCP server 的 prompt/resource 能力、认证和内容可靠性取决于 provider；bridge 不能为 provider 错报内容兜底。
- fork 已有 Agent Core 集成测试，但本轮尚未形成 Windows 实机和真实 Desktop 跨进程证据。

## 5. 不变量

- fork 缺少 child starter 时必须显式失败，不能回落 inline。
- MCP server 离线时必须移除其 tools 和动态 Skills，不能继续调用旧 fetcher。
- MCP Skill 不改变父 run 的工具池，也不反向控制 server lifecycle。
- MCP 动态 Skill 没有本地 Skill root；不得为了环境快照 identity 伪造本地路径。

## 6. 代码 owner

- `src/skillruntime/manager.go`：inline/fork 调度与 receipt。
- `src/agentcore/agent.go`、`runner.go`：真实 child-agent handoff。
- `src/skills/mcp_bridge.go`：MCP prompt/resource 动态注册和注销。
- `src/mcpclient`：MCP transport、认证、tool/provider 生命周期。
