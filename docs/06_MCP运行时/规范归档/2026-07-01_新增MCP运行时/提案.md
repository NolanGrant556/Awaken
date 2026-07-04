## Why

Awaken 的 work 场景需要接入大量外部系统（文档、表格、IM、CRM、知识库），每种系统单独写一套工具接入会让权限、observability、approval、UI 都失控。MCP（Model Context Protocol）是把"工具/资源/Prompt"统一成一种 transport-agnostic 协议的现成抽象，Claude Code 已经验证了它在多 transport、OAuth、重连、能力上报上的可行性。

Awaken v1 需要一个最小可用的 MCP runtime：能并发连接多个 server、支持主流 transport、暴露统一的工具池和资源池给 agentcore，并把连接状态（包括需要授权、连接失败、被禁用）显式表达给上层 UI。没有它，连接器、Skill、外部 SOP 都没法落地。

## What Changes

- 新增 `src/mcpclient` 包：定义 MCP server 配置 schema、连接管理器、传输适配器、auth handler。
- 支持 transport：`stdio`、`http`（streamable HTTP）、`sse`、`ws`、`in-process`（SDK-like，本地 Go 包暴露为 server）。
- 连接状态机：`pending → connected | failed | needs-auth | disabled`，支持自动重连和 401 触发重新授权。
- MCP server 暴露的 tools 注册到 `src/tool/registry.go` 的统一工具池，命名带 `mcp__{server}__{tool}` 前缀避免冲突。
- 新增 server 端 API：
  - `GET /v1/mcp/servers`：列出所有已配置 server 及当前状态。
  - `POST /v1/mcp/servers/{name}/auth`：触发 OAuth 流程或注入凭据。
  - `POST /v1/mcp/servers/{name}/disable` / `/enable`：禁用或启用。
  - `GET /v1/mcp/tools`：列出当前可用工具，带 metadata。
- 配置来源支持：项目级 `awaken.mcp.json`、用户级 `~/.awaken/mcp.json`、运行时动态注入（为 connector 留口）。
- 不在本 change 范围：connector 来源层（独立 change）、MCP prompts/resources → Skill 映射（独立 change）、IDE 专用 transport（v1 不做）。

## Capabilities

### New Capabilities

- `mcp-runtime`: MCP server 连接管理、传输适配、auth、tools/resources 发现，以及对 Awaken 工具池的注册。

### Modified Capabilities

无（首次引入相关能力）。

## Impact

- 新增 Go 包：`src/mcpclient`、`src/server/handlers_mcp.go`。
- 修改 `src/tool/registry.go`：支持动态 tool 注册/注销。
- 修改 `src/agentcore/runner.go`：消费 MCP 工具时透传 server 上下文（用于权限决策和审计）。
- Desktop 侧需要在后续 change（connector）里消费 `/v1/mcp/*` API；本 change 不改前端。
- 新增依赖：MCP Go SDK（候选 `github.com/modelcontextprotocol/go-sdk`）或自实现 JSON-RPC 2.0 客户端，决定写在 design.md。
- 配置文件：新增 `awaken.mcp.json` 例样到 `docs/` 目录。
