## Context

Awaken 当前 `src/tool/permission.go` 已经为 `apply_patch`、`shell` 做了硬编码决策。MCP / connector / Skill 加入后，工具数量从 ~7 涨到几十，硬编码不可持续。

Claude Code 把这块收在 `sourcecode/src/hooks/useCanUseTool.tsx` + `services/internalLogging.ts` + `services/toolUseSummary/toolUseSummaryGenerator.ts`：决策只看工具的 annotations（不看名字），审批 UI 按 annotations 类型分发，审计统一格式。同一抽象 port 到 Go。

ApprovalRequest schema 是"共同契约"——本 change 给出 wire-level schema 草案，Nolan 那边的 Desktop Approval Center 在产品侧渲染。两边都 freeze 这套 schema。

`AGENTS.md` 要求 tool / approval / SSE 路径都有 real-LLM smoke——本 change 改动覆盖所有这些，smoke 要扩展。

## Goals / Non-Goals

**Goals:**

- `ToolDefinition` 是工具的唯一身份证：注册到工具池的所有工具都必须填齐元数据。
- 权限决策只看 annotations，不看名字。
- 决策结果三态：`allow`（直接执行）、`ask`（弹审批，等用户决定）、`deny`（直接拒绝，原因写审计）。
- 工具调用生命周期事件统一：`started → progress* → (completed | failed | needs_auth)`。
- 审计记录可在 run 事件流取回，也可写到可选 sink（v1 默认内存/磁盘，远端 sink 留接口）。
- `needs_auth` 是头等 outcome，不是错误的一种——上游 UI 用它走重新授权交互。

**Non-Goals:**

- 不做 Desktop Approval Center 的 UI 实现（产品侧）。
- 不做远端 / channel 审批（M3）。
- 不做 XAA / 组织级 policy。
- 不做实时风险评分模型。
- 不引入新的 LLM-based policy 模型（v1 全部规则式）。

## Decisions

**Decision 1：`ToolDefinition` 字段最小集。**

字段：
- `name`：唯一名（含命名空间前缀如 `mcp__`、`skill__`）。
- `displayName`：UI 友好名。
- `source`：`builtin | mcp | connector | skill | plugin`（plugin 留扩展位）。
- `description`：自然语言描述。
- `inputSchema`：JSON Schema。
- `annotations.readOnly`：bool，是否只读。
- `annotations.destructive`：bool，是否会写入/删除外部状态。
- `annotations.openWorld`：bool，是否会访问外部网络（区别于本地）。
- `annotations.requiresAuth`：bool，缺凭据时是否会 fail。
- `annotations.serverName`：optional，MCP 工具的 server 名。
- `annotations.toolName`：optional，原始名（去前缀后的）。
- `permissionPolicy`：决策提示（`always_allow | always_ask | per_session | one_time`），可在用户配置中覆盖。

理由：与 Claude Code 的 `useCanUseTool` 输入字段对齐，又满足 work 场景把 connector / skill 当 first-class 的需求。

**Decision 2：决策规则（v1 默认）。**

按 annotations 决策：
- `readOnly == true` AND `openWorld == false` → `allow`（本地只读，无外联）。
- `readOnly == true` AND `openWorld == true` → `allow`（远端只读，例如 web_search）。
- `destructive == true` → `ask`。
- `requiresAuth == true` 且当前 server 状态非 `connected` → 直接 `deny` 且 outcome 是 `needs_auth`（让上游处理授权）。
- Skill 工具 → 继承所引用工具集中最严格的策略；如全部 readOnly 则 allow，含 destructive 则 ask。
- 用户配置可对单个工具或单个 server 全局 override，最高优先级。

理由：默认对 destructive 保守，对 readOnly 宽松；用户可在体验后手动放宽。

**Decision 3：事件 schema。**

新增 `AWAKEN_TOOL_PROGRESS` 事件 payload：
```
{
  toolCallId, toolName, source, stage,
  message?, percentage?, error?
}
```
`stage` ∈ `{ started, progress, completed, failed, needs_auth }`。

`needs_auth` 也作为 stage 而不是单独事件——上游统一处理。`AWAKEN_APPROVAL_REQUESTED` 仍由 `ask` 决策触发（现有事件），本 change 不重复定义；但确保 payload 里带 `ToolDefinition` 完整 metadata 让 UI 渲染。

**Decision 4：审计记录形态。**

`ToolAuditRecord`：
```
{
  toolCallId, runId, toolName, source,
  annotations, decision, decisionReason,
  approvalOrigin: "local_ui" | "system_policy" | "auto_allow" | "skill_preapproved" | null,
  startedAt, completedAt, outcome: "success" | "failed" | "needs_auth" | "denied",
  resultSummary?, errorReason?
}
```

存储：默认写入 run 事件历史（已有 event bus），同时通过 `AuditSink` interface 留远端 / 文件 sink 扩展位。v1 实现一个 NoopSink + InMemorySink + FileSink（写到 `~/.awaken/audit/{date}.jsonl`）。

**Decision 5：兼容 Nolan 的 ApprovalRequest。**

`ApprovalRequest`（已有）payload 扩展：
```
{
  toolDefinition: <full ToolDefinition>,
  input: <arguments>,
  riskHints: {
    destructive, openWorld, requiresAuth,
    affectedSystems: [serverName | connectorId]
  },
  allowedActions: ["approve", "reject", "edit_plan", "request_more_info"]
}
```

理由：UI 渲染审批界面不需要回查工具池；审批 payload 自包含。`riskHints.affectedSystems` 让用户秒看"这个动作会影响哪些外部系统"。

## Risks / Trade-offs

- [所有现有工具都要补 annotations] → 本 change 列入 task，逐个 patch；MCP/Skill 的 annotations 已经在前面 change 里要求填齐，重点是 builtin 工具。
- [硬编码移除后部分边缘 case 行为漂移] → 加 regression test，比对决策结果与旧实现。
- [`AuditSink` 文件 sink 写盘 latency] → 异步队列写盘；丢失最近 N 条容忍（v1 不上 fsync），文档化。
- [`needs_auth` 上游误处理为普通错误] → AG-UI mapper 强约定，单测覆盖。
- [`permissionPolicy` 用户配置 schema 需要新文件] → 用 `~/.awaken/permissions.json` 承载，schema 在 docs。

## Migration Plan

1. 引入 `ToolDefinition` 与新 `Tool` interface。
2. 逐个迁移现有 builtin 工具（同 PR 一并改）。
3. MCP / Skill 注册路径已经在 add-mcp-runtime / add-skill-runtime 中以 ToolDefinition 形态注入，本 change 顺势收口。
4. 旧权限函数保留 deprecated shim，仅供过渡使用，发版后下个 release 删除。

## Open Questions

- `permissionPolicy` 是否区分"per-session"和"per-run"？v1 只做 per-session，per-run 留后续。
- `AuditSink` 远端写入是否需要批量？v1 默认不批量；如果落 OpenTelemetry 等，再优化。
