# 工具决策、Outcome 与失败监督

## 1. 权威范围

本文唯一定义工具执行主链、决策枚举、专用人工 gate、canonical `ExecutionOutcome` v2、宿主工具活动、tool-call/tool-result 配对、FailureSupervisor 和审计现状。Shell 的本机执行边界、TaskScope、typed 文件变更和 Artifact producer 分别由本域对应专项文档拥有；Desktop 如何展示这些事实由 01 域拥有。

## 2. 执行链与决策

[`src/agentcore/tool_runtime.go`](../../src/agentcore/tool_runtime.go) 与 [`src/agentcore/tool_runtime_execution.go`](../../src/agentcore/tool_runtime_execution.go) 的当前主链是：

```text
subagent boundary backstop
  -> resolved-name-pool gate
  -> registry lookup
  -> input normalization + registered input-schema validation
  -> ExecutionIntent
  -> guardian gate
  -> Tool.CheckPermission / annotation fallback
  -> optional patch preview
  -> AutoToolPolicy
  -> permission event
  -> ask_user / dedicated decision gate / transform / executor router
  -> canonical outcome finalizer
  -> terminal tool result
```

SemanticJournal 不在 ToolRuntime 内部偷写。Runner 在副作用前先耐久化 tool call，工具返回后再将 terminal result 和 semantic facts 批量入账。permission decision 在 executor 前已完成，但执行路径的 permission event 先进入 buffering sink，随 terminal result batch 才耐久化；不得将“副作用前已完成决策”误写为“副作用前已耐久记录决策”。审计调用在前置 deny/auth/runtime/review 分支和执行后分支的具体时机不完全相同；统一不变量是“不先宣称执行成功”。

当前统一 decision action：

```text
allow, deny, transform, needs_auth, needs_runtime,
unsupported, needs_mass_message_review
```

旧 `abort` 只作兼容；统一 decision adapter 将它映射为 `deny` + `policy_error`，不是独立终止 decision。不存在普通工具的 `ask`、`approve` 或通用 review decision。`transform` 只允许一层，转换后的工具仍必须经过 schema 正规化和 executor-specific enforcement；嵌套 transform 被拒绝。

群发消息是唯一当前工具专用人工审核 gate：runtime 发出 review request，decision 枚举只有 `approved/rejected`；context 取消或 await 失败通过 error 路径表达，不是 `cancelled` decision。批准后 executor 仍要执行硬门禁。`ask_user` 属于 02 的任务澄清，不是工具审批。Shell executor 当前无条件拒绝已识别的 mass-message CLI，所以 Shell 群发即使 review 批准也不能真正执行；专用 connector/消息工具按各自 executor 合同继续。

`TransformShellToFileChain` helper 和测试存在，但默认 AutoToolPolicy 当前不会普遍将 Shell 写命令自动转换为 typed file chain。

## 3. Canonical ExecutionOutcome v2

Go authority 是 [`src/tool/outcome.go`](../../src/tool/outcome.go)、[`src/tool/outcome_v2.go`](../../src/tool/outcome_v2.go)、[`src/tool/outcome_collections.go`](../../src/tool/outcome_collections.go) 和 [`src/tool/outcome_projection.go`](../../src/tool/outcome_projection.go)。schema version 为 2，且四条状态轴不得压成一个 boolean：

| 维度 | 当前值 |
| --- | --- |
| `status` | `succeeded/failed/denied/cancelled/interrupted/needs_auth/needs_runtime/unsupported/review_required` |
| `transport_status` | `not_started/completed/failed/cancelled/unknown` |
| `semantic_status` | `success/failure/neutral/unknown` |
| `process_status` | `not_applicable/exited/timed_out/signaled/spawn_failed/unknown` |

`succeeded` 必须与 `transport=completed` 一致；语义可为 `success` 或 `neutral`。例如 `rg` 无匹配、`diff` 有差异或 shell `test` 为 false 可是已完成的 neutral 诊断，不能只因 non-zero exit 就将 run 猜成失败。`denied/needs_auth/needs_runtime/unsupported/review_required` 在规范化后为 `transport=not_started`。无效或自相矛盾的组合保守地转为 typed failure，不乐观推断成功。

canonical 顶层字段完整集合为：

```text
schema_version, content,
status, transport_status, semantic_status, process_status,
exit_code, signal, timed_out,
error_code, error_category, error_message, retryable,
recovery_options, diagnostic,
affected_paths, patch_summary, artifacts,
summary, operation, changes, environment,
warnings, errors, next_actions, details,
started_at, finished_at, duration_ms, truncated
```

`error_category` 为 `validation/policy/authorization/runtime/process/tool/persistence/unsupported/cancelled/domain`；`EnvironmentObservation.observation` 为 `none/partial/complete`。

### 3.1 结构化字段

| 对象 | 字段和当前语义 |
| --- | --- |
| `OperationDescriptor` | `type/target/side_effect_state`，概括本次操作和首要目标 |
| `ResourceChange` | `kind/action/resource/summary/details/certainty/reversibility`，表示已观测或已推断的资源变化 |
| `EnvironmentObservation` | `observation/observed_at/scope/source/workspace_root/working_directory/files/git/processes/environment_variables/unobserved`，显式区分看到了什么和未观测什么 |
| `OutcomeNotice` | `code/message`，用于 warning，不把警告伪装成成功文案 |
| `OutcomeError` | `type/category/stage/cause/context/possible_fixes/retry_strategy` |
| `NextAction` | `id/description/tool/preconditions/input_hint`，表示有边界的恢复选项，不是已执行副作用 |

兼容字段 `content/result/is_error/affected_paths/patch_summary` 可作旧链路与显示桥接，但 canonical 事实是 Outcome；`IsError` 只能从 Outcome 派生，不能反过来重建四条状态轴。`review_required` 不在 `IsError` 的 status 硬枚举内；当前 mass-message review 使用 semantic=`neutral`，因而 compatibility `IsError=false`，只有该 status 同时带 semantic failure 时才会派生 error。

`OutcomeTool` 必须把业务/进程失败写入 Outcome；Go error 保留给 transport 或 implementation failure。若实现同时返回 Go error 和 succeeded Outcome，runtime 会强制终态为 failed。legacy `(content,error)` adapter 仍存在，但不是新工具的权威合同。

### 3.2 副作用、变化可信度与可恢复性

`side_effect_state` 为：

```text
not_started, none, applied, partial, unknown
```

`ResourceChange.certainty` 为 `observed/inferred/unknown`，`reversibility` 为 `reversible/conditional/irreversible/unknown`。这三组概念不可混用：工具失败不等于“副作用没发生”，可恢复也不等于已经观测到变化。

规范化器的保守规则：

- policy/auth/runtime/unsupported/review 在 executor 前终止时为 `not_started`；
- succeeded 且有 observed changes/artifacts 时为 `applied`；已声明可变更但 executor 没提供可观测证据时不得猜 `none`，保守为 `unknown`；
- cancelled/interrupted/failed 且已有变化时为 `partial`；无法确认是否已写入时为 `unknown`；
- executor 真实观测的 path 才能升格为 observed change；permission 预测的 affected path 不能被误写为已完成变化。
- `ArtifactActionPublished` 只表示把既有文件暴露为 deliverable，不生成“创建/修改了源文件”的 `ResourceChange`。
- environment 默认只投影 executor 已知的 workspace/cwd/file/process 事实，未完整观测时显式列出 `unobserved`；finalizer 不会为每次工具自动扫描完整 git/process/environment snapshot。

### 3.3 错误阶段和恢复策略

Outcome 内部的执行错误阶段固定为：

```text
input_validation, policy, authorization,
precondition, execution, verification
```

Journal 持久化、SSE 投影或 Desktop 渲染失败不是同一次工具执行的 stage；它们由 03/09/01 的交付链分类，不回写工具已发生的 Outcome。

`retry_strategy` 为：

```text
do_not_retry, retry_same_input, retry_after_backoff,
refresh_state_then_retry, change_input_then_retry,
request_authorization, prepare_runtime, switch_tool,
manual_recovery_required
```

`retryable` 只回答相同 call 是否可能重试成功。validation failure 通常不能重放原输入，但仍应给出 `change_input_then_retry`；needs-auth/runtime 分别要求先授权/准备 runtime；review 要求完成对应窄 gate，不能自动绕过。

## 4. 唯一 finalizer 与可观测顺序

[`src/agentcore/tool_runtime_outcome.go`](../../src/agentcore/tool_runtime_outcome.go) 的 `finalizeToolResult` 是 direct 与 transformed 执行的唯一 canonical 终点。顺序为：

1. 填充 timing，将旧 `(content,error/status)` 结果适配为 Outcome，对 canonical content 先应用 20,000 characters 统一上限。
2. 分开 referenced path 与 executor-observed actual path，合并 typed trash/artifact/change 事实。
3. `Normalize` clone 原始结构、固定 schema v2，校验状态组合。非法 status 变为 `failed/transport=unknown/process=unknown/non-retryable/invalid_outcome_status`；自相矛盾的 succeeded 变为 failed + unknown transport；控制面阻断强制 not-started。
4. 对 succeeded result 验证 registered `output_schema`。strict definition 不匹配时转为 `tool_output_schema_mismatch`，stage 为 `verification`，不可 blind retry；adapted 定义只加 warning。
5. 即使 output schema 验证失败，也保留 executor 已观测的 path/artifact/change 和 `partial/unknown` 副作用；“结果格式错”不能篡改外部变更真相。
6. 补齐 summary/operation/target/side-effect state/environment/errors/next-actions，合并 warning，去重 changes 和 paths，检查集合不变量。succeeded 会清空 error/recovery/errors，不让旧失败残留污染成功结果。
7. 已失败结果发出 terminal `failed` progress 并调用 audit sink；已验证成功结果先发 `completed` progress，再发布 Artifact event，将 Artifact 回并 Outcome 后再次规范化并调用 audit sink。Desktop 生产 sink 当前为 Noop，该调用不等于已耐久写入审计记录。

因此对外顺序是：

```text
execute
  -> normalize + output verification
  -> terminal progress
  -> success-only artifact publication
  -> canonical audit / terminal tool result
```

不得在 schema 验证或 Outcome 规范化前先对外发布“成功”或 Artifact。transform target 会按新名重新 lookup，使用新 input/output schema 并经过 executor enforcement；该 transform decision 已授权转写，因此不再跑第二次 AutoToolPolicy，而嵌套 transform 被拒绝。

## 5. ModelPayload 与脱敏边界

`ExecutionOutcome.ModelPayload()` 为下一轮模型生成稳定 JSON 投影，而不修改 canonical Outcome。实现只以 `SchemaVersion` 分支：不等于 2 时用空 execution context 做兼容 Normalize；等于 2 时无条件 deep-clone 并只补齐 environment bookkeeping，避免丢失已确立的 `side_effect_state=unknown` 来源。它不能识别“schema v2 但未 finalise/状态矛盾”的 Outcome；这类值也不会在 ModelPayload 阶段重跑 Normalize，调用方必须先通过 runtime finalizer 建立 canonical 事实。

投影合同：

- 先深拷贝 map/slice/pointer，并把嵌套值转为 JSON-compatible 容器；投影可并发读取，不与原结构共享可变 backing storage；
- `diagnostic/details/change.details/next_action.input_hint` 等结构递归扫描 key；当前正则为 `authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|secret|cookie|credential`，匹配键的值替换为 `[REDACTED]`；一般 `token`、`auth_token`、`id_token` 不匹配；
- 每个 structured 投影字段各有 8 KiB encoded budget；超限时仍返回合法 JSON，包含 `truncated/original_bytes/preview`；结构深度上限为 64，循环引用与不支持类型使用显式占位；
- 文本预算分别是 content 12K runes、error message 4K、summary 512、patch summary 2K、operation type 256 和 target 2K；Artifact `Content` 不进入模型投影。
- 极端 marshal 失败时只 fallback 到 content；这是兼容降级，不改写 canonical Outcome。

脱敏是基于键名的 defense-in-depth，不是自由文本秘密检测器。位于非敏感 key 下的字符串不会按内容扫描；旧工具将 secret 写入 `content/summary/error_message` 或任意普通 key 仍可泄露。调用方和工具 author 必须在源头避免这一做法。

SemanticJournal 持久化的是 canonical Outcome，不是已缩减/脱敏的 ModelPayload，详见 [SemanticJournal 与重放](../03_运行时与本地服务/SemanticJournal与重放.md)。因此模型投影脱敏不等于 journal retention policy；canonical Outcome 的 field-level sensitivity、retention 和 access policy 当前尚未完整实现，工具不得将秘密放入将被持久化的自由文本或 diagnostic。

## 6. MCP Outcome 映射

[`src/mcpclient/tool_adapter.go`](../../src/mcpclient/tool_adapter.go) 将 MCP transport 和工具业务结果分层：

- manager/transport/protocol 调用失败保留为 Go error，同时构造 transport/runtime Outcome；
- server 已返回 `CallToolResult{IsError:true}` 表示 transport 已完成但语义失败，code 为 `mcp_tool_reported_error`；不得将它丢成“HTTP/MCP 调用没回来”；
- 成功结果的 `StructuredContent` 只要可转为 JSON-compatible 值，且符合 object/已选 wrapper 形状，就会先进入 `details`；adapter 不会在这一步先证明它符合 output schema，后续 finalizer 才验证，adapted definition mismatch 只加 warning 且保留 details；无 wrapper 的非 object 结构保留在 diagnostic；
- 外部 schema 整体反序列化失败、非 object root 或 canonical 字段冲突时有已知 warning/包装路径；`pattern`、`format`、`$ref` 等 `ToolParam` 不支持的 keyword 可被静默忽略，不保证降级、拒绝或告警。

external output schema 只能落在 `details`内：普通 object root 原样保留；非 object root 包在 `mcp_value`；object root 若含 canonical Outcome 同名字段，整包放入 `mcp_payload`，不单独丢弃或改名冲突字段。外部 object 没声明 `additionalProperties` 时按 JSON Schema 默认语义显式设为 `true`。

成功 MCP result 的 StructuredContent 进入 details 后会从 diagnostic 去重。`IsError=true` 分支当前早返回 typed failure，因而 StructuredContent 保留在 diagnostic，不进 schema-backed details。needs-auth 映射 `needs_auth/not_started`，tool-not-found 映射 `unsupported/not_started`，其他 manager/protocol failure 为 transport failure。外部定义是 adapted/incomplete 时，output mismatch 只记 warning；不伪装成 strict 验证。

`McpToolAdapter.OutputSchemaWarnings()` 当前没有被 registry/工具列表消费，所以包装或降级 warning 可能不出现在常规诊断面；这是当前可观测性缺口。

## 7. 宿主工具活动合同

工具执行前的活动提示由宿主生成，不要求 assistant 先输出一段“我正在…”叙事。[`src/tool/activity.go`](../../src/tool/activity.go) 定义 `ToolActivity{stage,title,detail}` 和可选 `ActivityDescribingTool`；[`src/agentcore/tool_runtime_events.go`](../../src/agentcore/tool_runtime_events.go) 在输入已正规化、已获准且进入 executor 前发出 stage=`started`、percentage=0 的 `ToolProgress`。没有专用描述器的工具显示 `正在执行 <tool name>`。

当前只有两个生产工具实现专用描述：

| 工具 | 当前活动 |
| --- | --- |
| `text_edit` | `正在修改文件`，detail 为已解析 path；参数无效时只显示标题 |
| `shell` | 普通命令为 `正在执行命令`，detail 截至 160 runes；识别 pip/pip3/npm/pnpm/yarn/brew/winget 安装命令时改为 `正在安装依赖` |

Shell detail 的当前脱敏只识别长 `sk-`、`ak-` 和 `Bearer ` token，不是任意 secret scrubber；安装命令分支当前也没有再应用普通分支的 160-rune 截断。因此工具 author 不得把明文凭据放入命令，且安装 activity 存在过长 detail 的当前限制。

ToolProgress wire 字段由 09 定义，Workbench narrative/live-replay 投影由 01 定义；两者必须使用同一个 host activity，不在 reducer 中再猜一套文案。原方案中 Artifact 工具的专用进度标题当前没有实现；未实现的标题不得写成当前 Desktop 行为。

## 8. Tool-call / tool-result 配对

每个 assistant tool call 在下一条非工具消息前必须恰有一个 tool result，call ID 必须对应。result name 非空时还必须与 call name 相等；空 name 的 legacy result 当前仍可通过，因此 name 完整性不是当前无例外不变量。schema 验证失败、deny、needs-auth/runtime、unsupported、群发拒绝、取消、supervisor block、executor error 和重启封口都要合成 terminal result。

Runner 在每次 LLM request 前验证配对；tool call 必须在副作用前耐久写入，result 和 artifact/change 一起入账。不得通过删除失败调用、仅修改 UI 卡片或伪造 assistant 文本来修复坏历史。

## 9. FailureSupervisor

[`src/agentcore/failure_supervisor.go`](../../src/agentcore/failure_supervisor.go) 的当前默认硬预算如下。`AgentConfig` 传入正数时可为特定 run 设置更小或更大的 turn/tool-execution 上限，`<=0` 才使用 128/512 默认值；其余重复失败阈值当前不可配置。

| 规则 | 当前值 |
| --- | --- |
| run 默认最大 turns | 128 |
| run 默认最大 supervisor-admitted call | 512 |
| 同一 retryable call 的 admitted attempt | 最多 2 次；第 3 次在 ToolRuntime 前阻断 |
| 同一 non-retryable call 的 admitted attempt | 最多 1 次；第 2 次阻断 |
| 同工具 + 可从 arguments 重建的 resource revision + error class 的 `IsError` outcome | 最多 3 次；下一变体阻断 |
| 同 failure fingerprint 连续出现 | 2 次后要求 recovery |
| 单 turn 失败 | 6 次终止 turn/run |
| 单 run 失败 | 12 次终止 run |

`BeforeCall` 先检查总执行预算、exact call budget，再检查 tool + resource revision + error class 预算。Runner 在 tool call 已耐久化、supervisor 未阻断后，但在 `ToolRuntime.Execute` 之前调用名为 `NoteRealExecution` 的计数器。因此它统计的是 supervisor-admitted calls：pool gate、schema validation、permission deny、needs-auth/runtime/review 等后续未进入 executor 的调用也会消耗 attempt/512 预算；只有 supervisor 自身前置阻断的 call 不增加该计数。`AfterOutcome` 以 canonical `Outcome.IsError()` 判定失败，所有 error outcome 都进入 class/failure 预算，不限于 executor 失败。

call signature 是 `tool name + canonical arguments hash`：JSON key 排序，`patch/old_string/new_string/content` 以及普通字符串统一 CRLF/CR 和行尾空白。failure fingerprint 是 `tool + error code + primary diagnostic`，并从 diagnostic 移除 workspace root、timestamp、temp path、随机 ID 和行号。`AfterOutcome` 的 resource revision 优先取 arguments 的 `expected_sha256/sha256/resource_revision`，再取 path，最后取 Outcome diagnostic 中的相关 digest；resource identity 以 path 为主。invalid-patch 与 patch-context 错误 code 各自折叠为稳定 error class，防止只换一个 code 就绕过预算。

同 fingerprint 连续两次后，supervisor 生成 `RecoveryDirective`，Runner 用 `FormatRecoveryContext` 把 tool/error/attempt/options/message 作为结构化系统上下文注入下一轮。`ApplyRecoveryToOutcome` 把 recovery options 合并回 Outcome，需要改策略时在 diagnostic 写 `recovery_required=true`。同 tool + revision + class 已产生三次 `IsError` outcome 后，只有当下一次 `BeforeCall` 能从 arguments 重建相同 revision 时，JSON 参数不同的变体才会在 ToolRuntime 前被 class gate 阻断。若上次 revision 只存在 Outcome diagnostic，`BeforeCall` 传入空 Outcome 而得到 `unknown`，无法匹配已记录 class key；这是当前 pre-call class gate 盲区。

当前清理语义比“确定恢复成功”宽：任意 `!Outcome.IsError()` 的 outcome，只要工具名相同或非 unknown 的 resource identity 相同，就会清除 pending directive；`review_required + semantic=neutral` 也可能命中，不要将其误写为 `status=succeeded` 才清除。不相关工具/资源的 non-error outcome 不清除。而且 non-error outcome 只重置连续失败/fingerprint，不清除 `callAttempts`、`callFailureMeta`、`classFailures`或 class metadata；同签名后续仍可因历史 attempt，同 revision/class 仍可因历史失败预算被阻断。

预算耗尽形成 `denied/not_started/semantic=failure` 的结构化阻断 result，diagnostic 包含 `call_signature/attempt_count/recovery_required`。exact-call 或 class pre-block 本身不设 `TerminateRun/TerminateTurn`，Runner 可继续让模型改变策略；只有总 512 admitted-call 预算，或 `AfterOutcome` 累计的单 turn 6 / 单 run 12 失败预算直接设终止。最终 run 仍可因后续状态成为 `repeated_tool_failure` 或 `max_turns`，但不能将每个 exact/class synthetic block 写成自动终止 run。

### 9.1 当前恢复映射

| 错误类 | 当前 recovery options 与禁止行为 |
| --- | --- |
| invalid patch format | 改用 `text_edit` / `text_file_create`、重读文件；不重放相同 patch |
| patch context/stale | `read_file`、用当前内容改为 `text_edit`；不重放旧 patch |
| `text_not_unique` | 扩大 `old_string` 上下文，或显式 `replace_all=true` |
| `text_not_found` | 重读并用当前内容重建 edit |
| `stale_file_revision` | 重读，取新 digest 后重建 edit |
| `process_exit_nonzero` | 检查 exit/stderr，改命令或停止，不盲目循环 |
| `needs_runtime` | 使用 host runtime manager；Artifact Skill 不得 shell install 绕过 |
| `skill_tool_not_allowed` | 重编译 activation plan；不用 Shell 绕过 denied tool |
| `skill_platform_contract_conflict` | 退出当前 Skill；只能在 Skill scope 外另建 typed-platform recovery |
| `persistence_error/persistence_unavailable` | 停止新副作用，等待 journal 恢复或终止 |
| `tool_retry_blocked` | 更换策略；禁止相同参数再试 |

## 10. 审计现状

[`src/tool/audit.go`](../../src/tool/audit.go) 的 `ToolAuditRecord` 保存 call/run/tool/source/annotations、decision/reason/origin、时间、outcome/result/error、profile/intent/action/transforms、trashed/subagent/guardian 等。`FileSink` 是可显式注入的按日 JSONL 组件和测试设施；当前 Runner 的 `toolRuntimeConfigFromAgentConfig` 不传 `AuditSink`，ToolRuntime 因而默认使用 `NoopSink`，Desktop 生产执行链没有接入 FileSink。

即使显式注入，FileSink 也不是 SemanticJournal 的耐久替代：队列溢出静默丢弃最旧记录；`Close` 关闭 goroutine 而不排空 queue；mkdir/open/marshal/write 错误均被吞掉，`Write` 仍返回 nil。

09 定义的 run audit 查询动作当前没有接入 run 的真实 sink，handler 返回空数组。审计写入能力和按 run 查询 API 不能被描述成已完整闭环。

## 11. 不变量与当前不支持

- Definition 声明不授权；decision 必须在 executor 前完成。副作用前耐久不变量是 tool call；permission event 当前与 terminal batch 一起耐久化，不得宣称已在副作用前耐久记录。
- executor/finalizer 必须输出 canonical Outcome；UI 文案不能改写状态或副作用真相。
- 没有普通工具的通用审批等待态或 `ApprovalCard`。
- 不支持无限重试、自动猜测 partial side effects 或默认 Shell-to-file transform。
- Shell mass-message CLI 的批准后执行路径未接通；不得把 review UI 出现等同于消息可发送。
- Desktop 生产 ToolRuntime 当前是 Noop audit sink，audit query 也不返回真实 run 记录；验收只能核对 SemanticJournal 的已接线事件，或在自动化中显式注入 sink，不能假设生产 FileSink 存在。
- 宿主 activity 脱敏不是通用 secret scanner；未实现的专用工具标题不是当前产品契约。

## 12. 测试与 Desktop 验收

自动化 owner：[`src/agentcore/tool_runtime_test.go`](../../src/agentcore/tool_runtime_test.go)、[`src/agentcore/tool_runtime_policy_test.go`](../../src/agentcore/tool_runtime_policy_test.go)、[`src/agentcore/tool_runtime_events_test.go`](../../src/agentcore/tool_runtime_events_test.go)、[`src/agentcore/tool_runtime_audit_test.go`](../../src/agentcore/tool_runtime_audit_test.go)、[`src/agentcore/shell_outcome_integration_test.go`](../../src/agentcore/shell_outcome_integration_test.go)、[`src/agentcore/failure_supervisor_test.go`](../../src/agentcore/failure_supervisor_test.go)、[`src/agentcore/semantic_journal_test.go`](../../src/agentcore/semantic_journal_test.go)、[`src/tool/outcome_test.go`](../../src/tool/outcome_test.go)、[`src/tool/outcome_projection_test.go`](../../src/tool/outcome_projection_test.go)、[`src/tool/external_output_schema_test.go`](../../src/tool/external_output_schema_test.go)、[`src/tool/audit_test.go`](../../src/tool/audit_test.go)、[`src/mcpclient/tool_adapter_test.go`](../../src/mcpclient/tool_adapter_test.go)。

最小自动化要覆盖：四状态轴的合法/矛盾组合、strict output mismatch、failed + partial/unknown side effect、ModelPayload 深拷贝/脱敏/每字段预算、MCP `IsError` 和 StructuredContent、allow/deny/needs-runtime/auth/review、terminal pairing、ToolActivity 默认/安装命令脱敏、重复失败阻断和 audit sink。

真实 Desktop 至少观察 allow、deny、needs-runtime/auth、一次工具失败和重复失败阻断。群发审核只能在隔离测试中观察，禁止真实发送。ToolActivity、卡片、terminal result 与最终叙事必须保持同一 status；失败不得生成虚假 Artifact，neutral 诊断退出不得误标 run 失败。audit endpoint 的空数组不能作为“没有工具活动”的验收证据。
