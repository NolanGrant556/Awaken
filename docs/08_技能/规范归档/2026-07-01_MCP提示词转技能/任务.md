## 1. 订阅接口

- [ ] 1.1 在 `src/mcpclient/manager.go` 增加 `Subscribe(ctx) <-chan ServerEvent`（若 add-mcp-runtime 未实现）
- [ ] 1.2 定义 `ServerEvent`：`Connected | Disconnected | Disabled | PromptsListChanged | ResourcesListChanged`
- [ ] 1.3 单测：状态切换触发对应事件

## 2. Bridge 包

- [ ] 2.1 创建 `src/skills/mcp_bridge.go`
- [ ] 2.2 `Bridge.Start(ctx, mcpManager, skillRegistry)`：消费事件，调用 `syncForServer`
- [ ] 2.3 `syncForServer(name)`：调 `prompts/list`，幂等地把 prompts 集合刷到 Skill registry
- [ ] 2.4 反注册：server `Disconnected | Disabled` → 移除该 server 的所有桥接 Skill

## 3. Skill 数据结构扩展

- [ ] 3.1 `SkillDefinition` 增加 `mcpBinding`（serverName + promptName + protocolVersion 可选）
- [ ] 3.2 `SkillDefinition` 增加 `requiredMcpServers`
- [ ] 3.3 SkillRegistry 可用性判定考虑 `requiredMcpServers`
- [ ] 3.4 本地 SKILL.md loader 校验：本地 Skill 不允许 `mcp__` 前缀
- [ ] 3.5 单测：本地 Skill 也能用 `requiredMcpServers`；非法 `mcp__` name 被拒

## 4. Prompt → Skill 转换

- [ ] 4.1 MCP `Prompt.name/description/arguments` → SkillDefinition 各字段
- [ ] 4.2 Skill name = `mcp__{serverName}__{promptName}`
- [ ] 4.3 `whenToUse` fallback 为 `description`
- [ ] 4.4 `arguments` schema 映射（type / required / default）
- [ ] 4.5 单测：转换器在常规 / 缺字段 / 大量 prompts 情况下的行为

## 5. Prompt 执行

- [ ] 5.1 SkillExecutor 识别 `source: mcp` 的 Skill，执行时调 `prompts/get` 获取正文
- [ ] 5.2 把正文（substituted with args）作为 user message append 到当前 run（inline）；fork 模式同 `add-skill-runtime`
- [ ] 5.3 `prompts/get` 超时 60s，超时返回结构化错误
- [ ] 5.4 单测：mock MCP server 提供 prompt，bridge 桥接，执行能拿到正文

## 6. Debounce 与防抖

- [ ] 6.1 `syncForServer` 在 300ms 内合并多次触发
- [ ] 6.2 单 server prompts 上限 100，超出截断 + warning
- [ ] 6.3 单测：连续 5 个 list_changed 在 200ms 内只触发一次实际 sync

## 7. Resource-as-prompt（最小集）

- [ ] 7.1 `resources/list` 中 `mimeType: text/x-prompt` 的资源也桥接为 Skill
- [ ] 7.2 其他 mimeType 暂不映射（留 TODO）
- [ ] 7.3 单测：混合 prompt + 文档 resource，仅 prompt 被桥接

## 8. Observability

- [ ] 8.1 Skill 调用事件透出 `mcpBinding`，让审计能看到来源
- [ ] 8.2 `GET /v1/skills` 响应包含 `mcpBinding`、`requiredMcpServers`
- [ ] 8.3 真 LLM smoke：起一个内置 echo MCP server（in-process transport）暴露一个 prompt，桥接为 Skill，agent 通过 tool call 触发执行

## 9. 文档

- [ ] 9.1 `docs/mcp-prompts-as-skills.md`：说明桥接规则、命名空间、可用性派生
- [ ] 9.2 更新 `docs/skill-md-format.md`，加入 `mcpBinding` / `requiredMcpServers` 字段说明
