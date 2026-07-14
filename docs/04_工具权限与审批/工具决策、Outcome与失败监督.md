# 工具决策、Outcome 与失败监督

## 1. 当前决策模型

普通工具默认自动执行。typed provider 仍依次通过固定快照查找、provider readiness、input schema 正规化、工具自身合同和对应 executor。默认 `shell` 是当前登录用户的直接本机 provider：命令分类和 TaskScope 可用于观测，但不把 workspace、网络、包管理器或未建模 CLI 变成默认拒绝条件。

统一决策动作是 allow、deny、needs_auth、needs_runtime、unsupported、transform，以及 typed 消息 provider 的专用 `needs_mass_message_review`。取消由 runtime 独立处理；legacy `abort` 输入只能转为 `deny` + `policy_error`，不是当前决策枚举。

当前没有通用逐工具 Approval，也没有 `awaiting_approval` run 状态。仅有两类交互 gate：

- `ask_user`：补充任务所需用户输入；
- mass-message review：发送给多人、群、频道、广播目标或会扩展收件人的消息。

`transform` 只允许一层；目标工具必须仍在同一 run 快照中，用目标 schema 重新正规化，并经目标 executor 硬门禁。已授权的转换不再进入第二次 AutoToolPolicy，但嵌套 transform 必须 deny。`TransformShellToFileChain` 是 helper/测试路径，当前默认策略不会把所有 Shell 写命令普遍自动转为 typed file chain。

typed 删除工具仍可通过 `trash_file` 提供可恢复语义；默认 raw Shell 不再拦截或转换 `rm` 等命令。因此只有实际选用 typed 路径时才能声称 Trash recovery，不能将该保证扩大到任意 Shell 或 Skill 子脚本。

provider `needs_runtime` 的 host-generated plan 仍可投影为 `dependency_install` next action，该 typed installer 继续验证 plan/environment/manager 和它自身的 staging 合同。但这是一条可选的 typed provider 恢复路径，不是通用 Skill 安装依赖的前置要求。Agent 可以按 `SKILL.md` 用默认 Shell 直接执行 `npm install`、`pip install` 或其他初始化命令，以真实命令结果继续诊断。

## 2. 群发消息审核

被声明为 `message.send` 的 provider 必须先形成 typed `MessageEgressEnvelope`，至少包含 provider、channel/account、可观察 recipients、message count、body/template、attachments 和是否扩展收件人。外部 MCP 中只有明确自洽的 read-only tool 可不带 Awaken per-tool metadata；所有非 read-only 或 annotations 自相矛盾的 tool 缺失/无效 metadata 时，在 transport 前以 `unverified_mcp_provider_metadata` fail closed。因此普通别名不再能绕过“非读调用必须有有效 provider contract”的门禁；具体 metadata 字段由 [06 工具接入](../06_MCP运行时/工具接入与技能桥接.md) 拥有。

- 一个明确 individual、单条消息且不扩展：自动执行；
- 多人、group、channel、broadcast、message count > 1 或 expands recipients：写耐久 review request，等待用户批准/拒绝；
- 收件人 identity/kind 不可观察：在 transport 前 deny。

review request 原样持久化 adapter 从实际调用参数生成的 `envelope digest`、`body digest` 和规范化 `recipient identity digest`，Session 不从 preview/count 重算简化摘要。resolution 必须回传并精确匹配这三个 digest；字段缺失 fail closed，digest mismatch、已解析或重放均拒绝。ToolRuntime 只在 resolution 已耐久且为 approved 后签发进程内随机、一次性的 execution grant，绑定 run、tool call、tool、provider 和三个 digest；adapter 必须在 external transport 前原子消费。直接调用 adapter、修改参数、跨调用复用或重放 grant 都只能得到 `review_required` / `transport=not_started`，不能触发外部调用。重启不能自动发送尚未批准的消息，也不能用修改后的 envelope 消费旧批准。

上述保证只属于按 `message.send` 接入的 typed provider。默认 raw Shell 不再对协作 CLI 做字符串拦截或 mass-message review，因此通过任意 CLI/脚本实施的消息发送不在 typed envelope 保证内。文档、UI 和 receipt 不得把 typed provider 的审核合同扩大成“Agent 任意外发均受控”。

## 3. ExecutionOutcome v2

工具结果必须区分：

- outcome status：succeeded、failed、denied、cancelled、interrupted、needs_auth、needs_runtime、unsupported、review_required；
- transport status：not_started、completed、failed、cancelled、unknown；
- process status/exit/signal；
- semantic status；
- operation 与 `side_effect_state`：not_started、none、applied、partial、unknown；
- errors/warnings、retryability、recovery options、next actions、observed changes 和 artifacts；
- tool/environment snapshot、provider、readiness、dependency observations。

UI 文案、模型 payload 和 canonical outcome 是不同投影，不能相互反推。工具自报成功但输出 schema 不匹配、transport 未完成或 side effect 未验证时，normalizer 必须降为结构化失败/unknown；不得生成伪 artifact 或伪 change。

`succeeded` 必须与 `transport=completed` 一致，但 semantic 可以是 success 或 neutral；例如无匹配查询或 diff 发现差异可以是“进程完成、语义中性”，不得只因 non-zero exit 把整个 run 猜成失败。`denied`/auth/runtime/unsupported/review 在 executor 前终止时必须是 `transport=not_started`。

side-effect 事实与工具成败分离：

- 已观测 changes/artifacts 的成功可标 `applied`；声明可变更但无证据时保守标 `unknown`；
- 失败/取消/中断且已有变更时为 `partial`，不能借“返回格式错”擦除外部变化；
- dependency manager exit 0 但复探未 ready 时，transport/process 可为 completed/exited，Outcome 必须是 `needs_runtime`；若 broker 已提交 workspace delta，changes 与 applied side-effect 仍必须保留；
- `ResourceChange.certainty` 的 observed/inferred/unknown 与 reversibility 的 reversible/conditional/irreversible/unknown 分开；permission 预测路径不能冒充 executor-observed change。

`finalizeToolResult` 是 direct 和 transformed 路径的唯一 canonical 终点：填 timing/长度上限，合并 executor-observed facts，规范化状态，验证 strict output schema，再发 terminal progress；只有已验证成功才发布 Artifact。新工具应实现 `OutcomeTool`，Go error 留给 transport/implementation failure，业务/进程失败进 Outcome。

### 3.1 模型投影与持久边界

`ExecutionOutcome.ModelPayload()` 深拷贝 canonical v2，对 diagnostic/details/change details/next-action input 的敏感 key 做递归脱敏，并对每个结构字段实施 8 KiB encoded budget；content、error、summary、patch/operation 也有独立长度上限。脱敏依赖 key name，不是自由文本秘密扫描器。

SemanticJournal 持久的是 canonical Outcome，不是缩减后的 ModelPayload。因此工具不得把凭据写入 content/summary/error/diagnostic 等将被持久的自由文本；完整 field-level retention/access policy 仍是当前缺口。

`dependency_install` 不实时转发原始 package-manager chunks；它先整体有界收集，再清洗 token marker、URL userinfo、常见 query token 与 auth assignment，最后才写 observer/Outcome。该专用边界不等于所有 legacy tool 的自由文本都已具备同等脱敏。

## 4. 失败监督

Agent 根据稳定 error code、retryability、transport 和 side-effect state 决定下一步：

- `needs_auth`、`needs_runtime`、`unsupported` 提供可执行 next action，不盲重试；
- `not_started` 可按修正后的输入重试；
- `partial` 或 `unknown` 先检查外部状态，禁止原样重放有副作用调用；
- 输出契约不匹配、provider readiness 改变或外部 provider 违约都保留诊断证据。

被声明为 remote `file.delete` 的 MCP tool 在 transport 前要求 per-tool recoverable-delete contract；调用后仍必须返回声明的 recovery evidence。未声明 metadata 的任意非 read-only 别名已由通用 MCP metadata 门禁在 transport 前阻断；有效 metadata 的 provider 若调用后撒谎，仍只能检测 contract violation，Awaken 无法撤销已发生的远程副作用。

当前硬预算是：默认 128 turns/512 个 supervisor-admitted tool calls；同一 retryable call 最多允许 2 次失败尝试，non-retryable 最多 1 次；同 tool + resource revision + error class 累计 3 次后阻断下一变体；同 fingerprint 连续 2 次后注入 recovery directive。调用方可配置 turn/call 上限，其余阈值当前固定。

单个业务工具的累计失败不再直接终止 run：单 turn 6 次或单 run 12 次失败会升级为强制 `strategy_change_required` directive——要求更换工具、参数或实现路径，或停止该策略并说明；run 只被 run 级预算（max turns / max tool executions）终止。被阻断的完全相同重复调用返回可恢复 tool result（`status=strategy_change_required`、`retryable=true`、`same_call_retryable=false`、`next_actions`），run 继续；改变工具、参数或策略视为新的尝试，不沿用旧 fingerprint 阻断。

signature 由 tool name + canonical arguments hash 组成，fingerprint 用稳定 error code/主诊断并清除 workspace root、timestamp、temp path、随机 ID 等易漂移信息。对 partial/unknown 或外部副作用不得盲目重放；非相关工具的成功也不能清除未完成的 recovery directive。

Shell 的 resource revision 使用 `command + workdir` 生成不暴露命令文本的稳定摘要；不同命令不再共用 `unknown` class budget。当已观测 mutation 成功，或默认 direct Shell 成功且由宿主基于 canonical call arguments 与 Shell command classifier 判定为明确恢复型 mutation（例如 `npm install`、`python -m pip install`、创建/写入文件）时，才清除旧 Shell 失败历史，使同一 validator/build 可以针对新状态重跑。仅有 `side_effect_state=unknown` 不足以解锁：`node --version`、无输出文件的 `curl` 和无关本机 CLI 即使 direct broker 无法观察副作用，也不能清除失败历史或 pending recovery；副作用已证明为 `none` 的只读成功同样不能绕过失败监督。

同 capability 存在多个 provider 也不允许 runtime 在失败后暗中换实现。若 provider A 已 `transport=completed` 且返回 partial/observed side effect，当次 call 必须以 A 的失败与变更事实终止，不自动调用 provider B。只有模型在检查外部状态后明确发出新的 B tool call，B 才可执行。

## 5. 耐久顺序

有副作用工具必须先写 tool call/decision 语义记录，再开始 transport；结果和 observed changes 完成后再写 terminal。mass review request/resolution 也先耐久再解锁。live 与 replay 使用同一 semantic projector。

ToolRuntime 内部 permission event 先进 buffering sink，再与 terminal result 批量耐久。因此可以声明“副作用前已完成决策且 tool call 已耐久”，不能声明每个 permission event 都已在副作用前独立落盘。

## 6. 审计现状

`ToolAuditRecord` 可保存 run/call/tool/source、intent/profile、决策来源、transform、Trash/subagent/guardian 和 Outcome 摘要。`InMemorySink`/`FileSink` 可供显式注入和自动化，但生产 Runner 当前未从 `AgentConfig` 注入 audit sink，默认为 `NoopSink`；`GET /v1/runs/{id}/audit` 也仍返回空数组。

`FileSink` 是异步测试/可选组件，不是 SemanticJournal 替代：队列溢出会丢最旧记录，`Close` 不排空队列，文件错误也未回传。因此当前不能宣称 Desktop 生产 audit query 已闭环。

## 7. 代码 owner

- `src/tool/outcome_v2.go`、`egress.go`：结果与消息 envelope 决策。
- `src/agentcore/tool_runtime.go`、`auto_tool_policy.go`、`failure_supervisor.go`：执行、审核与恢复。
- `src/tools/dependency_install`、`src/tool/execution_broker*.go`：typed dependency plan 执行、sandbox/staging、commit/discard。
- `src/runtime` SemanticJournal：耐久顺序。
- `src/tool/audit.go`、`src/server/handlers.go`：审计组件与尚未接通的查询面。
