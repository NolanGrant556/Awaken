## Why

伙伴负责的四块（MCP、connector、Skill、bridge）最终都汇入同一个工具池。如果工具池每条入口各自处理权限、各自上报进度、各自命名结果字段，agent 端就会出现"同一个动作来自不同源时审批 UI / 日志 / 审计语义不一致"的混乱。

Awaken 当前 `src/tool/permission.go` 已经为内置工具做了一版决策；MCP / connector / Skill 进入后需要：
1. 统一 `ToolDefinition` metadata schema（`source / readOnly / destructive / openWorld / requiresAuth / serverName / toolName`）。
2. 统一权限决策结果（`allow / ask / deny`）。
3. 统一工具进度事件（`started / progress / completed / failed / needs_auth`）。
4. 统一审计输出（来源、是否 destructive、是否 openWorld、审批者来源）。

这层是 plan 中明确列出的"共同契约"——伙伴主导 `ToolDefinition`，Nolan 消费。本 change 把它收口。

## What Changes

- 定义统一 `ToolDefinition` schema 并迁移所有现有工具（builtin、MCP、connector、skill）使用同一个结构。
- 修改 `src/tool/permission.go`：决策函数接受 `ToolDefinition.Annotations`（不再只看工具名硬编码），输出 `PermissionDecision`（`allow | ask | deny` + 原因 + 风险等级）。
- 新增工具进度事件：`AWAKEN_TOOL_PROGRESS`（含 stage、message、percentage 可选、error 可选），并在 agentcore 调用工具时按统一接口发射。
- 扩展 `AwakenEvent` 集合：`needs_auth` 阶段是首次引入，对应 MCP 401 等场景；要在 `src/protocol/agui/mapper.go` 中映射成 AG-UI 兼容事件。
- 新增 `ToolAuditRecord`：每次工具调用结束写入审计流（落到 run 事件历史 + 可选 sink），包含 source、tool、approval origin（local UI / system policy / pre-approved skill / 等）、duration、outcome。
- 新增 `GET /v1/tools` API：列出当前工具池中所有工具及 metadata（合并 builtin + MCP + skill），供 UI / 调试用。
- 新增 `GET /v1/runs/{id}/audit`：返回该 run 的审计记录序列。
- 不在范围：UI Approval Center 的渲染（Nolan 那边的产品改造）；远端 / channel 审批（M3）；XAA 等组织级 policy（v1 不做）。

## Capabilities

### New Capabilities

- `tool-platform`: 工具元数据 schema、权限决策模型、工具进度事件、调用审计——这些跨越 builtin / MCP / connector / Skill 的横切关注点收口在这里。

### Modified Capabilities

- `mcp-runtime`: MCP 工具注册时 MUST 提供完整 `ToolDefinition.Annotations`（之前 `add-mcp-runtime` 只承诺透传 MCP server 给的 annotations；本 change 把字段对齐到 ToolDefinition 的统一 schema）。
- `skills`: SkillTool 注册时 MUST 提供 `ToolDefinition.Annotations`（推导规则在本 change 给）。

## Impact

- 修改 `src/tool/tool.go`：定义 `ToolDefinition`、`ToolAnnotations`，重构 `Tool` interface 让所有 tool 通过 `Definition()` 暴露 metadata。
- 修改 `src/tool/permission.go`：决策函数签名改为接受 `ToolDefinition`，移除硬编码工具名分支。
- 修改 `src/tool/registry.go`：注册时校验 metadata 完整性，拒绝 metadata 不全的工具。
- 修改 `src/agentcore/tool_runtime.go`：调用工具前先做决策，调用过程发射统一进度事件，调用后写审计。
- 修改 `src/runtime/event_bus.go` 与 `src/protocol/agui/events.go` / `mapper.go`：新增 `AWAKEN_TOOL_PROGRESS` 事件。
- 修改 `src/server/handlers.go` 与 `router.go`：新增 `/v1/tools` 和 `/v1/runs/{id}/audit` 路由。
- 影响所有现有工具实现（`src/tools/*`）：补齐 `Definition()`；之前未声明 annotations 的需要逐个判定（默认 `readOnly: false`、需要审批；具体 default 在 design.md）。
- 影响 Nolan 那边的 Desktop Approval Center：本 change 约定 approval payload 的 schema，前端按这套 schema 渲染。
- 测试：扩展 smoke 用例，确保 destructive 工具触发 ask、readOnly 工具直接 allow、needs_auth 触发对应分支。
