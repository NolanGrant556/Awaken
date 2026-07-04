## 1. 统一 ToolDefinition

- [ ] 1.1 在 `src/tool/tool.go` 定义 `ToolDefinition`、`ToolAnnotations`、`PermissionPolicy`
- [ ] 1.2 修改 `Tool` interface 增加 `Definition() ToolDefinition`（保留旧 `Schema()` 作为 deprecated shim）
- [ ] 1.3 `Registry` 注册时校验 metadata 完整性（缺字段拒绝注册并打 log）
- [ ] 1.4 单测：合法/缺字段/重复名字三种 case

## 2. 迁移现有内置工具

- [ ] 2.1 `apply_patch` → ToolDefinition（destructive=true, openWorld=false, readOnly=false）
- [ ] 2.2 `shell` → ToolDefinition（destructive=true, openWorld 由命令决定，默认 true）
- [ ] 2.3 `read_file` / `list_files` / `grep_search` → readOnly=true, openWorld=false
- [ ] 2.4 `web_search` → readOnly=true, openWorld=true
- [ ] 2.5 `calculator` → readOnly=true, openWorld=false
- [ ] 2.6 单测：每个工具调 `Definition()` 返回预期 annotations

## 3. 决策引擎

- [ ] 3.1 重构 `src/tool/permission.go`：`Decide(ctx, def ToolDefinition, input) PermissionDecision`
- [ ] 3.2 实现默认决策规则（readOnly→allow、destructive→ask、requiresAuth+disconnected→deny+needs_auth）
- [ ] 3.3 用户配置 `~/.awaken/permissions.json` 加载与 override
- [ ] 3.4 移除原硬编码工具名分支
- [ ] 3.5 单测：默认规则矩阵 + 用户 override 覆盖

## 4. 工具进度事件

- [ ] 4.1 定义 `AWAKEN_TOOL_PROGRESS` 事件 payload schema
- [ ] 4.2 修改 `src/agentcore/tool_runtime.go`：调用工具前发 `started`，调用过程允许工具发 `progress`，结束发 `completed | failed | needs_auth`
- [ ] 4.3 `src/protocol/agui/events.go` + `mapper.go`：把 `AWAKEN_TOOL_PROGRESS` 映射为 AG-UI 兼容事件
- [ ] 4.4 单测：每种 stage 都能被序列化并通过 SSE 透出

## 5. 审计

- [ ] 5.1 定义 `ToolAuditRecord` schema
- [ ] 5.2 `AuditSink` interface + 三个实现：`NoopSink`、`InMemorySink`、`FileSink`（写 `~/.awaken/audit/{date}.jsonl`）
- [ ] 5.3 工具结束时同时写入 run 事件历史 + AuditSink
- [ ] 5.4 异步队列 + 退化策略（队列满时丢弃旧记录，打 warning）
- [ ] 5.5 单测：三种 sink 各自路径

## 6. ApprovalRequest 扩展

- [ ] 6.1 现有 `AWAKEN_APPROVAL_REQUESTED` payload 扩展 `toolDefinition`、`riskHints`、`allowedActions`
- [ ] 6.2 `src/protocol/agui/mapper.go` 适配新字段
- [ ] 6.3 单测：destructive 工具触发 ask 时，payload 字段齐全

## 7. needs_auth 流程

- [ ] 7.1 决策返回 `deny + outcome=needs_auth` 时 agentcore 不调用工具，直接发 `AWAKEN_TOOL_PROGRESS{stage:needs_auth}`
- [ ] 7.2 mapper 把 `needs_auth` 透出到前端
- [ ] 7.3 单测：MCP server 在 `needs-auth` 时 agent 调用其工具触发 needs_auth 流程

## 8. HTTP API

- [ ] 8.1 `GET /v1/tools`：列出当前工具池及完整 metadata
- [ ] 8.2 `GET /v1/runs/{id}/audit`：返回该 run 的 ToolAuditRecord 列表
- [ ] 8.3 路由注册到 `src/server/router.go`
- [ ] 8.4 单测：empty run / 含多种工具调用 run 两种 case

## 9. 文档与契约对齐

- [ ] 9.1 `docs/tool-definition.md`：ToolDefinition 字段说明（伙伴契约文档）
- [ ] 9.2 `docs/permissions.md`：默认决策矩阵 + 用户配置 schema
- [ ] 9.3 与 Nolan 对齐 ApprovalRequest payload schema，更新到 `docs/`
- [ ] 9.4 更新 `CLAUDE.md` 的"Approval & permissions"段，反映新决策模型

## 10. Smoke 与回归

- [ ] 10.1 扩展 `src/server` smoke：destructive 工具触发 ask 决策 → 模拟批准 → 完成 → 审计可查
- [ ] 10.2 扩展 smoke：readOnly 工具直接 allow → 完成 → 审计可查
- [ ] 10.3 扩展 smoke：MCP `needs-auth` 时调用其工具 → 不执行 + needs_auth 事件
- [ ] 10.4 回归测试：旧 builtin 工具的决策结果与迁移前一致（用 fixture 比对）
