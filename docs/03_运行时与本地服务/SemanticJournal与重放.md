# SemanticJournal 与重放

## 1. 权威与稳定 kind

SemanticJournal v2 是 run 内容与恢复的耐久事实源；09 域拥有 wire event/字段，01 域拥有 Desktop narrative。当前稳定 kind：

```text
thread, run_started, user_message, reasoning_summary, assistant_message,
tool_call, tool_result, artifact, file_change, turn_diff,
permission_decision, skill_activated, resource_receipt,
runtime_notice, recovery_decision, context_compacted, context_compacted_v2, turn_boundary,
run_terminal, persistence_warning, memory_recalled, memory_mutated,
context_snapshot, model_request, model_usage, context_epoch, instruction_snapshot,
provider_opaque_state, hook_snapshot, hook_invocation_started,
hook_invocation_completed, thread_rewound,
computer_session_opened, computer_state_changed,
computer_observation_captured, computer_action_prepared,
computer_review_requested, computer_review_resolved,
computer_action_started, computer_action_completed,
computer_control_transfer_started, computer_control_transfer_completed,
computer_session_closed, computer_media_accessed
```

`permission_decision` 是工具自动策略或 transform 的记录，不代表通用 Approval 能力。

`thread_rewound` 是普通会话 active lineage 改写的 append-only authority，本身不投影 AG-UI frame。读取、conversation 恢复与 transcript 重放必须先按该 marker 过滤目标 run 及其因果后缀；物理 journal 不截断。交互、CAS、副作用与恢复规则由 [01 用户消息撤销与编辑](../01_桌面工作台/中间交互区域/用户消息撤销与编辑设计契约.md) 唯一拥有。

`hook_snapshot` 是 run 配置审计事实，紧随 `run_started` 耐久化；它只含 snapshot ID、schema version、enabled/diagnostic 数量和各来源的 kind/status/digest/hook count，不含路径、matcher、command 或环境。它不投影 AG-UI frame，也不证明命令已经执行。每次真实执行在 spawn 前 append+flush `hook_invocation_started`，结束后 append+flush `hook_invocation_completed`；completion 只保存 identity、event/source、spawn/process/status/action/reason、duration、输入摘要、输出摘要与有界 preview，不保存 command、完整路径、输入正文、环境或完整 stdout/stderr。

`memory_recalled` 与 `memory_mutated` 只保存 Memory identity、scope/type、action、revision、status、count、source identity 和错误码，不保存主题正文。Dream maintenance task/report 的耐久 authority 是 `~/.awaken/memory/dream-tasks/`，其 `model_requests` / `model_usage` 也只写该 task authority，不写 thread SemanticJournal；其恢复和投影由 11 域定义。连接测试模型调用写 profile-scoped `~/.awaken/usage/model-profiles/*.jsonl`，同样不写 thread journal。

`instruction_snapshot` 在 run 执行前保存本次冻结指令的安全摘要：snapshot ID/time/partial、每个来源的 scope/display path/scope root/digest/original/applied bytes/truncated/status 和 diagnostics。它不保存 global/project 正文。旧 run 没有该 kind 时保持“无历史摘要”，不得读取当前文件反向补造。

## 2. Journal-first 写入顺序

- composer 将文本、有序 attachments 和 tokens 写入结构化 `user_message`；含 Skill token 的路径必须在预激活前完成该写入；
- run 主循环写 `run_started`，已准备的 composer turn 被 Agent 消费一次，不再写第二条 user message；
- run 带 Hook snapshot 时，在任何模型或工具执行前写并 flush 脱敏 `hook_snapshot`；每个 command Hook 在 spawn 前写并 flush started，completion 在其决策生效前写并 flush；
- 有副作用工具必须在 transport 前耐久 `tool_call` 和 decision；
- Skill 成功激活在正文进入执行上下文或 child 启动前写 `skill_activated`/`resource_receipt`；失败不写伪 activation；
- observed changes、artifacts 与 canonical `tool_result` 成批写入；
- Computer session/observation/action/review/control/media-access 事实由 run-bound recorder 写入同一 Journal；review requested/resolved 与 action started 在 UI 展示或 Driver perform 前 append 并 flush，写入失败必须阻止对应展示或输入；
- 上下文压缩结果在替换任何内存 authority 前写并 flush `context_compacted_v2`；feature flag 回退与尚未迁移的 v1 transcript thread 仍写并读取 v1 `context_compacted`，不能把 V2 section 写进不支持它的 recorder 后静默丢弃；
- 每次 thread-bound 模型调用在 transport 前写并 flush 脱敏 `model_request`，结束后以同一 request ID 写 `model_usage`；对话状态变化后写当前下一请求的 `context_snapshot`；
- 统一 LLM observer 发出的可展示 reasoning 在该段结束时 append `reasoning_summary`，再发布其 canonical live record；该记录只含展示正文、reasoning role 与稳定 message ID，并随 run terminal 的 flush 进入稳定存储；
- Context Epoch 的 exact baseline/source/tool 内容写 sidecar；journal 的 `context_epoch` 只写 epoch ID、digest、状态和边界原因；
- AGENTS synthetic prefix 正文只写 Context Epoch sidecar；`instruction_snapshot` append/flush 后通过 canonical projector 进入 live/replay，正文不得进入 payload；
- 最后写唯一 `run_terminal`。

append 或 flush 失败必须 fail-stop；不能继续副作用、模型 transport 或先切换内存上下文再补记。取消/崩溃恢复将未闭合 tool call 标为 interrupted/unknown；有 request 无 usage 的模型调用补 `interrupted + usage_unknown`；有 Hook started 无 completion 时补 `interrupted + spawn_state/process_state=unknown`，不重跑命令，也不猜副作用。`RunEnd` completion 可以位于 terminal 之后，恢复和投影必须保留该因果顺序。

## 3. 结构化用户输入与附件

`user_message` 保存文本、有序 attachments 和 composer tokens。附件保存 identity、kind、MIME、路径/显示名、尺寸、字节数与 runtime-owned `storage_key`；image base64 不写入 JSONL。重放以结构化字段为源，不从 UI 文案恢复 Slash token，也不扫描附件目录发明历史。

历史附件在重放时继续保留，不因当前所选模型缺少媒体能力而删改。Journal 对 tool-result media 只恢复 `storage_key` 与元数据，不在重放时全量灌回原图。进入下一次模型请求前，Agent Core 以当前 pinned `ModelCapabilities` 和**当前窗口**深拷贝并投影消息：不支持 vision/native PDF 的 profile 只收到稳定文本/metadata；支持能力的 profile 只水合近尾 1 个安全组内、且剩余窗口装得下的媒体。更早 Read/图/PDF 页以 stub 进入请求，blob 文件仍留在 `tool-results/`。`context_snapshot` 统计该次实际准入的请求副本，而不是未下发的本地媒体。重启后的 prepared request 必须与 live 投影一致。

Skill 预激活失败仍保留已写入的 `user_message`；成功的多 Skill records 成批写入，批次失败不保留部分 activation。

## 4. `context_compacted_v2` 与 v1 兼容契约

新压缩默认写 `context_compacted_v2`，payload 只以 `bundle_v2` 承载稳定 `PostCompactContextBundleV2` envelope。envelope schema version 固定为 2，包含 boundary/parent/generation/source sequence、`v1_messages|checkpoint_v2` conversation projection、TaskCheckpoint/SemanticSummary/RecentTurns、Instruction/Memory/RunPlan/Skill/Tool/Hook/gate/Subagent/Artifact refs、TranscriptLocator、ProviderOpaqueState、Budget/Coverage receipt 和完整 `BundleDigest`。未启用 section 必须显式为 `not_materialized + null + EMPTY_SECTION_DIGEST`，不可用 section 必须带稳定 reason；字段缺失、未知 state、value/digest 不一致均 fail closed。

当前 V2 已物化 TaskCheckpoint、SemanticSummary、最多三个 recent user-task turns、可用时的 TranscriptLocator、Coverage，以及 profile 有明确输入上限时的硬 10% BudgetReceipt。ProviderOpaqueState 默认仍为 `not_materialized`；只有 exact official OpenAI Responses native compact 成功、opaque output 结构/digest 合法且完整 prepared-body 通过同一硬 10% receipt 时才物化。该 section 保存 canonical encrypted output 与 provider/profile/credential/model/protocol/endpoint-contract identity；重启后只在 identity 全匹配时重新附加到 Responses input，变化时忽略并继续 provider-neutral checkpoint。Memory 只保存 identity/scope/type/revision/bytes/content digest，不复制正文；Skill ref 保存压后有界投影、正文 digest 与可重读路径。bundle digest 覆盖完整 canonical envelope。

旧 `context_compacted` payload 继续兼容：

```text
compact_boundary_id
mode
messages[]
pre_estimate_tokens
post_estimate_tokens
agent_id               # 仅 child runner
```

每条 compacted message 保存 role、content、tool calls/result pairing、name、error 标志、boundary ID、无 blob 的 attachment identity/storage key，以及 tool result 的 `model_content` metadata/storage key。第一条消息必须带与 payload 一致的 boundary；role 只允许 user/assistant/tool；完整消息集必须通过 tool-result pairing 校验。

重放按 journal seq 处理：合法 V2 先校验完整 bundle digest/section/ref/pairing，再用 conversation projection 替换之前累计的模型上下文，并从同一 bundle 恢复压后 Skill、TaskCheckpoint 与 Memory refs；合法 v1 仍按 `messages` 替换。authority 以最后一个合法 boundary 为准：V2→v1 会撤销旧 V2-only refs 并恢复 legacy Skill activation，v1→V2 则安装最新完整 bundle。坏 V2/v1 均保留此前最后一份 known-good context。V2 的 journal 提交使用可回滚 append+fsync，失败字节不会在后续 flush/restart 变成有效 boundary。完整 Journal 记录不删除，`thread_history_read` 只从当前 thread 返回带总字符预算和 secret/store-path 遮蔽的安全投影，provider opaque 与原始工具参数不进入结果。

父 runner 与 child runner 各自持久化自己的压缩边界；child payload 带 `agent_id`，不能把 child body 注入父上下文。

## 5. 上下文、模型调用与 provider 事实

`context_snapshot` 保存版本、basis、模型/协议/window/threshold、request-shape digest 和 typed contribution 计数；`model_request` 保存 request identity、purpose、模型/协议、shape digest、估算和 retry，不保存 HTTP body；`model_usage` 保存 provider/profile/credential/model/endpoint、cache metadata、nullable normalized/raw usage、pricing/cost、request ID、latency和终态。缺失 provider 字段保持缺失，不折 0。live 与 replay 使用同一 semantic projector，最近 provider usage 不反向改写当前 context snapshot。Renderer 回放同一 thread 的多个 run 时保留最后一个合法 `context_snapshot`；`compact_summary` / `native_compaction` usage 只作为压缩冻结的耐久时序标记，不作为面板用量，后续完整 snapshot 只有已经满足自身 hard target 才能解除没有 live lifecycle frame 的回放冻结。失败压缩后的高位 snapshot 与后续 run 没有快照都不能覆盖已恢复值；新的 `RUN_STARTED` 才结束上一 run 的失败冻结。Core 启动新 run 时则可独立从最后兼容硬边界之后恢复最近成功完整 `agent_turn` 的 provider total/input 与 `sent_estimated_input_tokens`，只作为 governor 的预算锚点；同一 context epoch 内该锚点是非递减下界，pre-epoch admission 与 post-epoch sent request 的估算阶段差异不能降低它，identity 不兼容、字段不完整或成功 compact 后才清除或拒绝恢复。`instruction_snapshot` 则是 run-scoped：重放按所属 run 恢复，不能像 thread-scoped context snapshot 跨 run 沿用。

Desktop 打开 thread 时先调用 `POST /v1/threads/{thread_id}/context-snapshot`，body 只复用当前 run profile 合同。Core 从 durable conversation 恢复 compact、附件、tool result 和已激活 Skill，结合当前 tool/MCP availability、Skill catalog、Memory 与当前 profile 生成下一请求的 v4 snapshot，flush 后 Desktop 再 GET transcript。该动作不发 provider 请求、不写 `model_request/model_usage`、不保存 API key；相同 projection 不重复追加。旧 run 的终态 session 仍驻留内存时，transcript 合并必须把此类 replay supplement 投影出来。没有可运行 profile、active run 正在拥有 live projection，或 active lineage 为空（例如撤销首轮）时，历史 transcript 仍可读，不伪造 snapshot，refresh 返回成功且不追加 record。

`reasoning_summary` 是 provider-neutral 的可展示事实：其来源只能是 LLM transport 已归一化的 `OnContent(text, isThinking=true)` 流；OpenAI-compatible、Anthropic 或后续适配器越过 `src/llm` 后都走同一写入与重放路径。模型没有发出可展示 reasoning 时不创建该记录。它不进入下一次模型上下文，也不替代 provider 私有续接状态。

`provider_opaque_state` 的 reasoning 正文不投影到 AG-UI、Memory、日志或 telemetry。恢复只能在 provider/profile、credential revision、model、protocol 和 endpoint identity 全部匹配时把它重新附加到 provider history；不匹配时忽略，不能降成 `reasoning_summary` 或普通 assistant/user 文本。修复前没有 `reasoning_summary` 的旧 run 不能从 opaque state 反向补造界面历史。

## 6. 工具、环境与大结果事实

tool call/result payload 持久保存 tool/environment snapshot ID、provider ID、capabilities、readiness、dependencies、transport/process/semantic 状态、side-effect state 和 `model_content` metadata。工具媒体实际文件位于 `sessions/<thread>/tool-results/<tool-call>/`，Journal 与 compact record 只保存 runtime-owned `storage_key`；Base64 不进入 JSONL、AG-UI、audit 或日志。

超过 32 KiB 的文本结果在首次写 journal/model context 前把完整正文原子写入同一 tool-results 目录的 `content.txt`（`0600`），再以 digest、字节数、稳定 preview 和 storage key 组成唯一 admission projection。重放和压缩识别该 projection，不重新截断或事后替换。

重放恢复 storage key 与元数据；hydrate 只发生在近尾媒体预算内。blob 缺失或 key 非法时保留原历史 tool card，并在实际 hydrate 失败时向模型结果追加 `tool_media_blob_missing` warning；不得把路径字符串或空 content 伪装成视觉成功。删除 thread 时删除其 tool-results，workspace/profile/线程状态重建后不会沿用内存文件快照或 cwd。

重放不查询当前 registry 或当前机器环境，也不因 provider 后来 ready 而改写旧 run。旧工具名按通用 legacy tool card 投影，不重写历史记录。

模型历史使用 canonical `ModelPayload`，UI 使用 display/projection。失败工具不能在重放时凭卡片文案生成不存在的 change/artifact。

媒体 Artifact 可额外耐久 `group_id` 与正整数 `group_index`。它们只表达同一次组图及其稳定位置：重试失败项时复用原组与原 index，replay/终态投影据此原位排序。继续 thread 前，Runtime 从 ready deliverable Artifact 重建当前媒体可见序号与 `artifact_id` 清单；这个 host-owned 派生清单不新增 journal kind、不包含真实路径，也不回写历史 `user_message`。

## 7. Skill 与任务输入恢复

`skill_activated` 保存锚定路径、正文、来源、调用模式和 receipt；服务重建先恢复 activation，遇到合法 V2 bundle 后以其压后 Skill refs 替换当前投影，边界之后的新 activation 再追加。压后不再使用独立 5K/25K 配额或按最近顺序丢弃 active Skill；每个 ref 必须保留 Skill ID、正文 digest 和精确 `Read` 源路径。

消息发送不写 requested/resolved 审核记录，也不在重启恢复时重建 pending review；它只保留普通 tool call/result 与 provider outcome 事实。

## 8. Live 与 replay

`src/protocol/agui/semantic_projector.go` 是 live/replay 共用 projector。`SemanticRecord.seq` 只定义 journal 内单调顺序；SSE cursor、wire seq/semanticSeq 由 09 域定义。旧 v1 adapter 只读既有测试/兼容输入，不得成为新写入格式。

## 9. Owner 与验证

- kind/payload 与 Runner：`src/agentcore/semantic_record.go`、`semantic_journal.go`、`runner.go`。
- 耐久、Context Epoch、工具媒体/大结果与模型历史恢复：`src/runtime/journal_store.go`、`context_epoch_store.go`、`tool_result_store.go`、`transcript_store.go`、`session.go`。
- wire 投影：`src/protocol/agui/semantic_projector.go`。

自动化必须覆盖压缩 append/flush 先于内存/epoch 替换、model request flush 先于 transport、OpenAI-compatible 与 Anthropic observer 的 `reasoning_summary` 同合同写入、active ephemeral reasoning 被 durable snapshot 取代、orphan usage 封口、重建 `RunService` 后 reasoning/snapshot/usage/summary+tail 精确恢复、坏 compaction fail closed、tool pairing、attachment/tool-media storage key 重放、文本大结果只 offload 一次、blob 缺失 warning、Base64 不落盘，以及 Memory receipt 无正文且 live/replay 同投影。真实 Desktop 还需执行一次含可展示 reasoning 的重启回放，并确认过程历史与最终回答仍在；含 `Read` 图片/PDF/大文本结果的旧新工具卡片和上下文面板也不得崩溃。Memory/Dream 的额外重启旅程按 11 域验收执行。
