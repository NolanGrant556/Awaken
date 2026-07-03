## Context

MCP 协议把 prompts 当成一等公民：server 可以在 `prompts/list` 里返回带 `name / description / arguments` 的可参数化 prompt 模板，client 用 `prompts/get` 拿到具体内容。这种语义和 Awaken Skill 一对一吻合：name / description / arguments 都齐。

Claude Code 在 `sourcecode/src/skills/mcpSkillBuilders.ts` 里做的就是这种桥接，把 MCP server 的 prompt 当作 Skill 暴露给主 agent。Awaken 直接复用同一抽象。

依赖：本 change 要在 `add-mcp-runtime` 和 `add-skill-runtime` 都落地后开工。前者提供 server snapshot + prompts/list 调用，后者提供 Skill Registry 与可用性模型。

`AGENTS.md` 要求 agent / tool 路径有 real-LLM smoke——本 change 触发的是 Skill 路径，覆盖 smoke 即可（不必额外 LLM 矩阵）。

## Goals / Non-Goals

**Goals:**

- 一个 connected MCP server 暴露的每个 prompt → 一条 `source: mcp` 的 Skill。
- 命名空间：`mcp__{serverName}__{promptName}`，与 MCP tool 命名同源，避免与本地 Skill 冲突。
- Server 断开 / disable → 桥接产生的 Skill 自动 `unavailable`；server 重连 → 自动 `available`。
- 订阅 `notifications/prompts/list_changed`，server 端 prompt 变化时刷新桥接 Skill 集合。
- 透出 `mcpBinding` 元数据，让 observability / 审计能看到 Skill 是 MCP 来源。

**Non-Goals:**

- 不映射普通 MCP resources（文档型）。这些应该走"上下文资源"通道而不是变成 Skill；留 follow-up change。
- 不做 prompt → Skill 的语义增强（自动生成 whenToUse 等）。直接用 server 给的 description。
- 不做远端 Skill 仓库 / marketplace。
- 不做 Skill ranker。

## Decisions

**Decision 1：何时触发同步。**

订阅 mcp-runtime 的 `ServerEvent` channel（`add-mcp-runtime` 已留 `Subscribe` 接口；如未实现，本 change 顺带补上）。事件包括 `Connected`、`Disconnected`、`Disabled`、`PromptsListChanged`。每个事件触发 `syncForServer(name)`，幂等地把当前 prompts 集合刷到 SkillRegistry。

理由：单向数据流（mcp-runtime → bridge → SkillRegistry），不需要 SkillRegistry 反查 mcp-runtime。

**Decision 2：Skill name 冲突处理。**

桥接 Skill 走 `mcp__` 前缀，本地 Skill 不带这个前缀，物理上不可能冲突。如果用户在本地 SKILL.md 里手写一个 `name: mcp__foo__bar` —— 视为非法名称，加载时拒绝（loader 加校验：本地 Skill name 不得以 `mcp__` 开头）。

**Decision 3：requiredMcpServers vs requiredConnectors。**

桥接 Skill 派生 `requiredMcpServers: [serverName]`。如果 server 同时有 `sourceConnectorId`，也派生 `requiredConnectors: [connectorId]`。两个字段都被纳入可用性判定（任一不满足即 `unavailable`）。本地 SKILL.md 也可以手写 `requiredMcpServers`（不只限于桥接 Skill）。

理由：connector 是产品壳，server 是协议事实，两者解耦。`requiredConnectors` 适合用户视角（"我没连 notion"），`requiredMcpServers` 适合技术视角（"那个手工配置的私有 server 还没起来"）。

**Decision 4：prompts/get 的拉取时机。**

`prompts/list` 通常只返回 name/description/arguments，不带正文。正文要等 Skill 真的被执行时调 `prompts/get` 拉。这避免启动时一次性把所有 prompt 内容拽下来。缓存策略：单次执行内不缓存（避免 prompt 变化时拿到陈旧版本）；后续可以加 ETag/TTL 优化。

**Decision 5：resource-as-prompt 的范围。**

MCP `resources/list` 返回的 resource 有 `mimeType`。约定 `mimeType: "text/x-prompt"` 或带 `annotations.prompt: true` 的 resource 视为可执行 prompt 模板，桥接为 Skill；其它不映射。如果某些 server 用了惯例外的标记，先不处理（避免误识别普通文档为 Skill）。本 change 先只做 `prompts/list`，resource-as-prompt 留 follow-up tick。

## Risks / Trade-offs

- [server 漂移导致 Skill 频繁出现/消失] → 在 Skill 注册/反注册事件上加 debounce（300ms 内合并），避免抖动；UI 看到的 `state` 是稳态。
- [prompts/get 长延时阻塞 agent loop] → 与 mcp-runtime 工具调用同样的 60s 超时；超时返回结构化错误。
- [恶意 server 返回大量 prompt 撑爆 tool list] → 单 server prompts 数量上限（默认 100），超过截断 + warning；可配置。
- [server prompt description 不规范导致 LLM 选错] → 暂时无能为力，靠产品反馈和未来 ranker；observability 会记录哪个 Skill 被错选。

## Migration Plan

依赖 `add-mcp-runtime` 和 `add-skill-runtime` 完成。落地后用户无感知，启动时自动桥接当前 connected server 的 prompts。

## Open Questions

- `prompts/get` 缓存策略：v1 不缓存；M2 是否加 ETag？等真观察到性能问题再说。
- `mcpBinding` 是否要带 `protocolVersion`？v1 不需要，本字段更多是 audit 用。
