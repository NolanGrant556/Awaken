## 1. 基础结构与依赖

- [x] 1.1 在 `go.mod` 添加 MCP Go SDK（如可用）或定 fallback 自实现的最小 JSON-RPC 2.0 client 范围
- [x] 1.2 创建 `src/mcpclient/` 包骨架：`types.go`、`config.go`、`manager.go`、`transport.go`、`auth.go`
- [x] 1.3 定义 `McpServerConfig` schema（stdio/http/sse/ws/in-process 五种 variant）和 `ScopedMcpServerConfig`（带 scope: `local|user|project|dynamic`）
- [x] 1.4 定义 `ServerState` 类型（`connected|failed|needs-auth|pending|disabled`）和对应 payload 结构

## 2. 配置加载与合并

- [x] 2.1 实现 `awaken.mcp.json`（项目级）和 `~/.awaken/mcp.json`（用户级）加载器
- [x] 2.2 实现配置 scope 合并：`project` 覆盖 `user`，预留 `dynamic`（connector 注入用）和 `enterprise`（policy 用）
- [x] 2.3 实现 `${ENV_VAR}` 替换；shell `headersHelper` 留接口但默认 disable
- [x] 2.4 配置文件 schema 在启动时校验，错误打 warning 但不阻塞启动

## 3. 传输适配器

- [x] 3.1 实现 stdio transport：spawn child process、管道 wiring、退出监听
- [x] 3.2 实现 streamable HTTP transport（含 SSE 升级）
- [x] 3.3 实现 SSE transport（兼容老的非 streamable HTTP server）
- [x] 3.4 实现 WebSocket transport
- [x] 3.5 实现 in-process transport：通过 channel 与本地 Go 包实现的 MCP server 通信
- [x] 3.6 给每种 transport 写 readonly mock server 的单元测试

## 4. 连接管理器

- [x] 4.1 `Manager.Start(ctx)`：批量启动所有 enabled server，并发限制
- [x] 4.2 状态机：pending → 握手 → connected / failed / needs-auth
- [x] 4.3 重连策略：指数退避 1/2/4/8/16s，最多 5 次，超过转 `failed`
- [x] 4.4 401 / session expired → 转 `needs-auth` 并保留 config，不直接 fail
- [x] 4.5 `Manager.Disable(name)` / `Enable(name)` 接口
- [x] 4.6 `Manager.Snapshot()` 返回所有 server 状态的只读快照（供 server handler 用）

## 5. 工具池集成

- [x] 5.1 修改 `src/tool/registry.go` 支持动态注册/反注册
- [x] 5.2 MCP tool 注册时统一加前缀 `mcp__{server}__{tool}`
- [x] 5.3 透传 MCP tool annotations（`readOnly`、`destructive`、`openWorld`）到 `ToolDefinition` metadata
- [x] 5.4 server 转 `failed`/`disabled` 时反注册该 server 的所有工具
- [x] 5.5 `Manager` 监听 server `notifications/tools/list_changed` 时刷新工具

## 6. Auth（最小实现）

- [x] 6.1 OAuth 2.0 authorization code + PKCE flow 骨架（discovery、loopback callback、token exchange）
- [x] 6.2 token 临时放内存；持久化接口（`CredentialStore`）留 interface，先用内存实现
- [x] 6.3 触发 auth 的 HTTP 接口：`POST /v1/mcp/servers/{name}/auth` 返回 authorize URL 让 UI 打开
- [x] 6.4 401 自动转 `needs-auth`，下次 tool 调用时返回结构化错误而不是悄悄失败

## 7. Server HTTP API

- [x] 7.1 `GET /v1/mcp/servers`：列出 server 及状态
- [x] 7.2 `POST /v1/mcp/servers/{name}/auth`：触发 OAuth 或注入凭据
- [x] 7.3 `POST /v1/mcp/servers/{name}/disable` 与 `/enable`
- [x] 7.4 `GET /v1/mcp/tools`：列出当前工具池（含 builtin + MCP，过滤 disabled server）
- [x] 7.5 路由注册到 `src/server/router.go`

## 8. 测试

- [x] 8.1 单测：每种 transport 的握手、tool list、tool call、断连恢复
- [x] 8.2 单测：连接状态机所有跳转（含 401 → needs-auth）
- [x] 8.3 单测：工具池动态注册/反注册
- [x] 8.4 集成测试：起一个 stdio echo server + 一个 mock HTTP server，并发调用
- [x] 8.5 真 LLM smoke：扩展 `src/server` 现有 smoke 用例，让 agent 通过 MCP echo server 完成一次工具调用

## 9. 文档

- [x] 9.1 在 `docs/` 加 `awaken.mcp.json` 示例配置（stdio + http + in-process 各一份）
- [x] 9.2 README 补一段"接入第一个 MCP server"快速上手
- [x] 9.3 更新 `CLAUDE.md` 的工具层描述，加入 `mcp-runtime` 章节
