## 1. 包结构

- [ ] 1.1 创建 `src/connectors/` 包：`types.go`、`registry.go`、`source_builtin.go`、`source_local.go`、`source_team.go`
- [ ] 1.2 定义 `ConnectorDefinition`（id、displayName、category、provider、authType、requiredScopes、trustLevel、mcpConfigFactory）
- [ ] 1.3 定义 `ConnectorState`（disconnected/pending/connected/needs-auth/failed + lastError + lastSync + sourceOrigin）
- [ ] 1.4 定义 `ConnectorSource` interface：列举 connector、增量更新订阅、来源标识

## 2. 内置 connector

- [ ] 2.1 写 `notion` connector：HTTP transport 配置工厂、OAuth scope、category=`docs`
- [ ] 2.2 写 `slack` connector：HTTP transport 配置工厂、OAuth scope、category=`chat`
- [ ] 2.3 单测：内置 connector `mcpConfigFactory` 在不同 scope 输入下产出符合 schema 的 config

## 3. Local connector 加载

- [ ] 3.1 `~/.awaken/connectors.json` schema 与加载器
- [ ] 3.2 验证：每个 local connector 至少有 displayName + category + 一段合法 MCP server config
- [ ] 3.3 与 builtin 同 id 时按优先级覆盖，记录 warning

## 4. Team connector 接口（留口不实现拉取）

- [ ] 4.1 定义 `TeamConnectorSource` interface（`List(ctx) ([]ConnectorDefinition, error)`、`Subscribe(ctx) <-chan Event`）
- [ ] 4.2 提供 noop 实现作为默认；真正的远端拉取留 follow-up

## 5. mcp-runtime 集成

- [ ] 5.1 在 `src/mcpclient/manager.go` 加 `InjectDynamic(connectorId, serverName, config)` 和 `RemoveDynamic(connectorId, serverName)`
- [ ] 5.2 server snapshot 上加 `sourceConnectorId?` 字段
- [ ] 5.3 同名 server 冲突时按 builtin > team > local > manual 优先级仲裁；被覆盖者上 `shadowedBy` 字段
- [ ] 5.4 单测：手工 `awaken.mcp.json` 和 connector 注入同名 server 时，仅 connector 生效

## 6. Connector 生命周期

- [ ] 6.1 `Registry.Connect(ctx, id)`：取 connector → 走 OAuth（复用 mcp-runtime auth flow）→ token 存到 CredentialStore → `InjectDynamic`
- [ ] 6.2 `Registry.Disconnect(ctx, id)`：`RemoveDynamic` → revoke token（best-effort）→ 状态 `disconnected`
- [ ] 6.3 `Registry.State(id)` 推导：connector 自身 status + 对应 server 在 mcp-runtime 的 state 合并

## 7. Server HTTP API

- [ ] 7.1 `GET /v1/connectors`：列出 connector 及 state
- [ ] 7.2 `POST /v1/connectors/{id}/connect`：返回 authorize URL（复用 mcp `auth` flow）
- [ ] 7.3 `POST /v1/connectors/{id}/disconnect`
- [ ] 7.4 路由注册到 `src/server/router.go`

## 8. 测试

- [ ] 8.1 单测：注册 builtin connector → connect → 注入 dynamic → mcp-runtime 看到新 server
- [ ] 8.2 单测：disconnect → 工具池中无相关工具
- [ ] 8.3 单测：local connector 同 id 覆盖 builtin（不允许覆盖）/ local connector 同 id 覆盖 local（允许）行为
- [ ] 8.4 集成测试：用 mock OAuth server 跑完一次完整 connect 流程

## 9. 文档

- [ ] 9.1 `docs/` 增加 `~/.awaken/connectors.json` 示例（一段 local connector）
- [ ] 9.2 README 增加"连接 Notion / Slack"快速上手
- [ ] 9.3 更新 `CLAUDE.md` 工具层段落，加入 connectors 层级
