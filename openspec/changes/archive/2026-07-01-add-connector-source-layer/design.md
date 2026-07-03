## Context

`add-mcp-runtime` 把 MCP server 抽象统一了，但配置入口仍是 JSON 文件。对 work 用户而言，最自然的心智是"我连了 Notion 这个应用"。Connector 这一层把"应用"翻译成 MCP server config。

Claude Code 的对应实现在 `sourcecode/src/services/mcp/claudeai.ts` + `officialRegistry.ts`，模式是：每个 connector 知道自己怎么生成 MCP config、需要哪些 OAuth scope、属于哪个 category。把这层 port 到 Go，去掉 Claude.ai 组织相关的特化。

`AGENTS.md` 要求 runtime 路径有 real-LLM smoke；本 change 自身不直接走 LLM，但下游 agent 调用 connector 提供的 MCP tool 时仍要在 smoke 里覆盖到。

## Goals / Non-Goals

**Goals:**

- Connector 是"轻产品壳 + MCP config factory"，不是新的 runtime 协议。
- 三种 connector 来源：`builtin`（first-party Go struct）、`local`（用户 JSON）、`team`（远端目录，留接口）。
- 同名 server 被多个来源声明时按优先级 builtin > team > local，并保留冲突信息给 UI。
- 手工 `awaken.mcp.json` server 和 connector 提供的 server 同名时，优先 connector，但记录"被 connector 覆盖"日志。
- Connector 状态对外暴露：`disconnected`、`pending`、`connected`、`needs-auth`、`failed`。

**Non-Goals:**

- 不做 Desktop 端的 Connector Center UI；本 change 只交付 API + 内部数据结构。
- 不引入 Claude.ai 特化的组织级 connector 目录。
- 不实现 XAA 或企业 SSO。
- 不实现 connector 内置工具（connector 只产 MCP config，不直接当工具）。
- 不实现 team connector 的远端拉取传输；本 change 只定 `ConnectorSource` interface。

## Decisions

**Decision 1：connector 与 MCP server 的关系。**

一个 connector 产生一个或多个 MCP server config（绝大多数 1:1）。连接 connector = 走完授权 → 把 config 注入 mcp-runtime `dynamic` scope → mcp-runtime 自然 connect。断开 connector = `RemoveDynamic` → mcp-runtime 自然反注册工具。

理由：把"连接生命周期"完全委托给 mcp-runtime，避免双状态机不一致。Connector 自己只跟踪 `授权状态`，server 连接状态从 mcp-runtime 读。

**Decision 2：内置 connector 的范围。**

首批选 2 个，按 M1 验收（"整理本周客户跟进并生成下一步行动"）所需：

- `notion`：HTTP transport，OAuth 2.0。负责文档型读写。
- `slack`：HTTP transport，OAuth 2.0。负责 IM 通知。

理由：这两个都已经有公开的 MCP server 实现，OAuth flow 都标准。GitHub / Linear 等留 P1。

**Decision 3：local connector 的形态。**

`~/.awaken/connectors.json` 里每个条目可以直接写一段 MCP server config，加上一个 `displayName` 和 `category`。本质上是"给一段 MCP config 套个壳"。理由：让早期用户在没有官方 connector 的情况下也能享受"连接器"的产品体验，而不是被迫去改 `awaken.mcp.json`。

**Decision 4：同名 server 去重。**

按来源优先级 `builtin > team > local > manual(awaken.mcp.json)`。冲突时高优先级生效，低优先级在 server snapshot 里以 `shadowedBy: "<source>"` 标记，UI 可显示告警。理由：不悄悄忽略，但也不让用户陷入"为什么我的配置不生效"的迷雾。

**Decision 5：connector 状态存储。**

- 授权 token 存到 `CredentialStore`（接口由 mcp-runtime 引入，本 change 复用，不重复造）。
- Connector 当前是否"已连接"的事实从 mcp-runtime server snapshot 推导，避免双状态机。
- "用户最后一次点过 connect 但还没授权完"这种中间态存在 connector registry 内存里。

## Risks / Trade-offs

- [token 仍然只在内存] → 进程重启需要重新授权。本 change 不上持久化（依赖未来 credential-store change）。先文档化告知。
- [手工 server 被 connector 覆盖时用户困惑] → server snapshot 显式标记 `shadowedBy`；CLI/API 输出可读 warning。
- [first-party connector 元数据漂移] → connector struct 里 hard-code OAuth client ID 等会有维护成本。短期可接受；长期由 team connector 远端拉取替代。
- [connector 多 server 输出] → 一个 connector 可能既要 doc server 又要 search server。`mcpConfigFactory` 返回 `map[string]McpServerConfig`，注入时按 connectorId+serverKey 命名，避免冲突。

## Migration Plan

首次落地。manual `awaken.mcp.json` 仍然支持；用户可以平滑迁移到 connector。

## Open Questions

- Team connector 远端目录是否走 HTTP JSON、还是直接复用 MCP 协议拉取 `resources/list`？等 team mode 真启动时再定。
- 内置 connector 的 OAuth client ID 是否走 Awaken 自己的 client 注册？还是要求用户自带？M1 用 Awaken 注册的 dev client，M2 决定是否对外公开。
