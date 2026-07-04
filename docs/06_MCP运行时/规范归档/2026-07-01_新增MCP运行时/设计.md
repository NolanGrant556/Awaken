## Context

Awaken 当前在 `src/tool/registry.go` 维护一个静态工具注册表，所有内置工具（`apply_patch`、`shell`、`web_search`、`read_file` 等）在启动时注册一次。要让外部系统进入工具池，需要：

1. 一个 transport-agnostic 的 MCP client，可以并发持有多条连接。
2. 一种动态注册/反注册工具的机制（连接断开/server 禁用时立刻从工具池移除）。
3. 一种把"未授权 server"也显式表达的方式，否则用户只能看到"工具没了"。

Claude Code 在 `sourcecode/src/services/mcp/` 里把这些都落地了——TypeScript 实现且和 `@modelcontextprotocol/sdk` 深度耦合。Awaken 是 Go，需要重新实现，但抽象层级照搬。

`AGENTS.md` 要求 runtime/agent/tool 路径必须有真 LLM smoke 测试，所以新工具池路径也得纳入既有 smoke 用例覆盖。

## Goals / Non-Goals

**Goals:**

- 单进程内并发管理多个 MCP server 连接。
- 支持 stdio / streamable HTTP / SSE / WebSocket / in-process 五种 transport。
- 连接状态对 server 端 API 可见，UI（在后续 change 里）可以渲染 `connected/failed/needs-auth/pending/disabled`。
- MCP tool 注册时携带 metadata（`readOnly`、`destructive`、`openWorld`、`requiresAuth`、`serverName`、`toolName`），为后续 `tool-platform` change 的权限决策铺路。
- 401 / session expired 能触发 `needs-auth` 状态而不是直接 fail。
- 重连策略：指数退避，最多 5 次，超过后转 `failed` 留 UI 显示。

**Non-Goals:**

- 不做 connector 来源层（独立 change `add-connector-source-layer`）。
- 不做 MCP prompts/resources → Skill 桥接（独立 change `add-mcp-prompts-as-skills`）；本 change 只保证 server.list_prompts / server.list_resources 调用通了、能拿到原始数据。
- 不实现 IDE 专用 transport（`sse-ide` / `ws-ide`）。
- 不做 XAA、Claude.ai proxy 等 vendor-specific 认证集成。
- 不实现 prompt cache、context management 等模型协议层细节。

## Decisions

**Decision 1：用官方 Go SDK 还是自实现？**

候选：
- (A) `github.com/modelcontextprotocol/go-sdk`（如已 stable）
- (B) 基于 `net/http` + 自实现 JSON-RPC 2.0 client

选 (A) 为优先，理由：MCP 协议在演进（list_changed、elicitation、progress），跟着官方 SDK 走能减少协议漂移的维护成本。Fallback：如果 SDK 不稳定或缺关键 transport（如 in-process），用 (B) 实现，但保持 Conn 抽象与 SDK 接口对齐，便于后续替换。

**Decision 2：tool 命名空间。**

MCP 工具统一加前缀 `mcp__{server}__{tool}`，与 Claude Code 一致。理由：
- 避免和 builtin 工具冲突（builtin 名字不带 `__`）。
- agentcore 在工具调用日志里一眼看出来源。
- 取消注册时只需按前缀过滤。

**Decision 3：连接状态机存储位置。**

放在 `src/mcpclient/manager.go` 内的内存 `map[serverName]*ServerState`，server API handler 通过 manager 暴露的只读 snapshot 访问。不持久化——重启后重新连接即可。OAuth token 持久化由 `add-tool-permission-observability` 之外的凭据存储模块负责（后续 change，本次留接口）。

**Decision 4：重连退避。**

指数退避 1s → 2s → 4s → 8s → 16s，最多 5 次。超过转 `failed` 状态，需用户手动触发 reconnect。不无限重连，避免远端 server down 时把日志刷爆。

**Decision 5：HTTP/SSE auth header 计算。**

支持配置里的 `headersHelper` 字段（shell 命令）输出 header JSON。这是 Claude Code 现成的模式，能兼容企业内部签名网关。第一版只支持环境变量替换 `${VAR}`，shell helper 留接口但默认 disable，等 `add-tool-permission-observability` 把凭据存储建好后再启用。

## Risks / Trade-offs

- [stdio server 子进程泄漏] → manager 在 server 切到 `failed`/`disabled` 时 kill 进程，并在 process exit 监听里同步状态。
- [HTTP MCP server 自签证书] → 默认走系统 trust store，配置里允许 `insecureSkipVerify: true` 但加 logger warning。
- [in-process transport 把 MCP server 嵌入 Go 进程] → 必须跑在独立 goroutine，panic 时不能拖垮主进程；用 `recover` + `failed` 状态上报。
- [501 / OAuth flow 阻塞 agent loop] → tool 调用走超时（默认 60s），超时返回结构化错误，agent 可以决定 retry 或换路径。

## Migration Plan

首次落地，无遗留迁移问题。后续 change（connector、skill）作为消费方，依赖本 change 提供的 `mcpclient.Manager` 接口和 `/v1/mcp/*` API。

## Open Questions

- in-process transport 的 SDK 接口形态：是 `interface{ ListTools() }` 还是 `chan Message`？SDK 还在演进，等真做的时候再敲定。
- 用户级 `~/.awaken/mcp.json` 是否需要 schema 校验工具（独立 CLI 命令）？v1 默认在启动时校验+warning，不阻塞启动；schema 命令留 P2。
