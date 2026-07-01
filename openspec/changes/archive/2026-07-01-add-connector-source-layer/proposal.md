## Why

裸用 MCP 配置文件门槛太高：用户需要知道 server command、env、OAuth client ID、scope。Work 场景的用户想看到的是"连一下 Notion"、"连一下 Slack"，不是"配 stdio command"。

Connector 是 MCP 之上的一层产品化封装：它是受信的、可发现的"工具源"（official / team / local），最终产出一份 MCP server config 注入到 mcp-runtime 的 `dynamic` scope。Connector 不引入新的 runtime 协议，只是 MCP config 的工厂 + 元数据 + 授权流程的承载。

这一层不做出来，UI 就只能让用户编辑 JSON；这一层做出来，UI 可以做"应用市场"式的连接器中心。Plan 第二里程碑（Connector Platform）依赖它。

## What Changes

- 新增 `src/connectors` 包：定义 `ConnectorDefinition`、`ConnectorState`、`ConnectorRegistry`、`ConnectorSource`。
- 内置 connector 目录（first-party）：以 Go struct 注册，首批含 1–2 个 work 场景常用 connector（具体在 design.md 定）。
- 自定义 connector（local）：用户可在 `~/.awaken/connectors.json` 添加自己的 connector 描述。
- Team connector（可选 P1）：从远端目录拉取 connector 描述，留接口。
- Connector 授权完成后，通过 `mcpclient.Manager.InjectDynamic(name, config)` 注入到 mcp-runtime 的 `dynamic` scope。
- 去重策略：同一个 server name 同时被手工 `awaken.mcp.json` 和 connector 声明时，优先 connector（明确来源）并 UI 标记冲突；不同 connector 声明同名 server 时按优先级 official > team > local。
- 新增 server API：
  - `GET /v1/connectors` — 列出可用 connector 及当前状态。
  - `POST /v1/connectors/{id}/connect` — 触发授权 + config 注入。
  - `POST /v1/connectors/{id}/disconnect` — 撤销 + 反注入。
- 不在范围：MCP runtime 本身（依赖 `add-mcp-runtime`）、Skill 与 connector 联动（属于 `add-skill-runtime`）、Desktop Connector Center UI（产品侧 follow-up）。

## Capabilities

### New Capabilities

- `connectors`: 受信工具源的注册、状态、授权流程，以及把 connector 转换为 mcp-runtime `dynamic` scope server config 的去重合并逻辑。

### Modified Capabilities

- `mcp-runtime`: 暴露 `InjectDynamic(name, config)` / `RemoveDynamic(name)` 接口供 connectors 调用；并在 `GET /v1/mcp/servers` 的响应里附带 `sourceConnectorId?` 字段，便于 UI 显示"由 Slack connector 提供"。

## Impact

- 新增 Go 包：`src/connectors`、`src/server/handlers_connectors.go`。
- 修改 `src/mcpclient/manager.go`：新增 `InjectDynamic` / `RemoveDynamic` 接口，并在 server snapshot 上附带 `sourceConnectorId`。
- 修改 `src/mcpclient/config.go`：合并 `dynamic` scope 时根据 connector 元数据做去重。
- 新增配置：`~/.awaken/connectors.json` 用户级；`docs/` 提供示例。
- 不修改 Desktop 代码（UI 在后续产品侧 change 落地）。
- 首批内置 connector 引入的外部依赖在 design.md 列明（避免在 proposal 里把范围撑大）。
