## Why

外部系统不只提供 tools，也会通过 MCP 协议提供 `prompts` 和 `resources` —— 这些其实就是该系统沉淀下来的 SOP（"客户跟进 SOP"、"表格异常巡检 SOP"）。如果只把 MCP 当工具桥，我们就丢掉了 server vendor 自带的方法论。

把 MCP prompts/resources 注册成 Awaken 的 Skill，意味着：CRM 厂商上线一个 prompt，用户的 Skill Library 自动多出一条"客户跟进 SOP"，缺 server 时 Skill 不消失而是显示"需要连接 X"。这是产品差异化里最有杠杆的一段——别人写流程，我们直接用。

依赖关系：本 change 在 `add-mcp-runtime` 和 `add-skill-runtime` 完成之后才能落地，因为它把两者粘起来。

## What Changes

- 新增 `src/skills/mcp_bridge.go`：监听 mcp-runtime 的 server 状态变化，对每个 `connected` server 拉取 `prompts/list` 和 `resources/list`。
- 把 MCP prompts 映射为 Skill（`source: mcp`）：name = `mcp__{serverName}__{promptName}`，description / whenToUse 直接来自 MCP prompt 的 `description` 字段，arguments 来自 MCP prompt 的 `arguments`。
- 把 MCP resources 中标记为可执行的（resource type = `prompt-template` 或类似惯例）也包入。其他普通资源不映射为 Skill（属于上下文层，留 follow-up）。
- 新增 SkillDefinition 字段：`mcpBinding`，记录该 Skill 来自哪个 server 的哪个 prompt，便于 observability。
- MCP Skill 的 `requiredConnectors` 自动派生为 `[server 对应的 sourceConnectorId]`（如果是 connector 注入的），或 `requiredMcpServers: [serverName]`（如果是手工配置的）。Server 断开 → Skill 自动 `unavailable`。
- 订阅 MCP server 的 `notifications/prompts/list_changed`，server 端 prompt 变化时刷新对应 Skill。
- 不在范围：MCP resources 的非 prompt 类型映射（普通文档资源应该走 connector context 而不是 Skill）；远端 Skill marketplace；Skill 排序优化。

## Capabilities

### New Capabilities

- `mcp-skill-bridge`: 把 MCP server 暴露的 prompts/resources 桥接为 Awaken Skill，并跟随 server 状态自动 enable/disable。

### Modified Capabilities

- `skills`: 扩展 `SkillDefinition` 增加 `mcpBinding` 字段；扩展可用性判定逻辑接受 `requiredMcpServers`（独立于 `requiredConnectors`，用于覆盖手工配置的 MCP server）。

## Impact

- 修改 `src/skills/types.go`：增加 `mcpBinding`、`requiredMcpServers` 字段。
- 修改 `src/skills/registry.go`：可用性计算考虑 `requiredMcpServers`。
- 新增 `src/skills/mcp_bridge.go`：subscriber + 转换器 + 反注册逻辑。
- 不修改 mcp-runtime 内部协议实现（消费已有 `Manager.Snapshot` + 新增订阅 hook，可在 mcp-runtime 中加 `Subscribe(ch)` 接口）。
- 修改 `src/mcpclient/manager.go`：增加 `Subscribe(ctx) <-chan ServerEvent` 用于状态变更通知（如果 mcp-runtime change 还没加，本 change 顺带加上）。
- 测试：扩展 skill smoke 用例，让 MCP server 暴露一个 prompt，验证它出现在 `GET /v1/skills` 中。
