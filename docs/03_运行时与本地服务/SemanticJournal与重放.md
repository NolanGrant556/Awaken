# SemanticJournal 与重放

## 权威范围

本文唯一定义 SemanticJournal v2 的记录种类、耐久/临时边界、写入顺序、失败语义、耐久重放来源与语义顺序。09 协议域唯一拥有 wire projector、`streamCursor` / `seq` / `semanticSeq` 字段、SSE cursor 和断线续传；01 中间区域只拥有 Desktop reducer/projector。

## 语义记录合同

Go authority 是 [`src/agentcore/semantic_record.go`](../../src/agentcore/semantic_record.go)。v2 `SemanticRecord` 具有 `schema_version`、`record_id`、`seq`、session/thread/run/turn/parent identity、`kind`、timestamp 和 payload。

当前稳定 kind：

```text
thread, run_started, user_message, assistant_message,
tool_call, tool_result, artifact, file_change, turn_diff,
permission_decision, skill_activated, resource_receipt,
runtime_notice, recovery_decision, turn_boundary, run_terminal,
persistence_warning
```

`persistence_warning` 是 schema 中的保留 kind；当前执行路径没有“写入退化但继续副作用”的模式。实际写入故障采用 fail-stop `persistence_error`，不能因枚举存在而宣称 degraded mode 已实现。

## 耐久与临时边界

设计上必须耐久：用户输入、最终 assistant message、完整 tool call 参数、canonical tool result、artifact/file change/turn diff、permission decision、skill/resource/runtime/recovery 事实、turn boundary 和 terminal。

当前 v2 `user_message` 耐久保存用户文本、有序 `attachments` 和 `composer_tokens`。附件字段保存 identity、kind、MIME、path/display name、尺寸、字节数与 runtime-owned `storage_key`；image base64 不重复写入 JSONL。`attachment_count` 只是兼容字段，不是恢复来源。

`messagesFromSemanticRecords` 只把可成功 hydrate 的附件注入模型历史；`durableRunsFromSemanticRecords` 则恢复 `Run.InputAttachments` / `Run.ComposerTokens` 供 Desktop 投影。若 runtime-owned image blob 缺失，durable run 仍保留其历史卡 identity，但不将无内容的图片注入模型上下文；不扫描目录寻找替代物。修复前已写入、只有 `attachment_count` 的旧 v2 记录没有足够事实可补回，只恢复可确认文本； v1 adapter/recorder 保留旧记录本来已有的附件与 token 字段。

尚存的用户输入时序缺口是：v2 `user_message` 由 Agent goroutine 在 Skill 预激活之后写入；预激活失败可能只有 run 开始/终态而没有用户消息。这一缺口不改变 tool call 的 append-before-side-effect 门，但意味着“所有被接受输入都已在预激活前耐久”尚未实现。

仅用于 live 体验：reasoning/text delta、stdout/stderr delta、args delta、spinner/progress。重放从耐久事实重新投影，不要求逐字重现临时动画。

模型重放使用 canonical outcome 的 `ModelPayload`，UI 使用 display/projection；两者不得从彼此的文案反推。失败工具不能在重放时凭 UI 卡片推断出不存在的 artifact。

## 写入与发布顺序

[`src/agentcore/semantic_journal.go`](../../src/agentcore/semantic_journal.go)、[`src/runtime/journal_store.go`](../../src/runtime/journal_store.go) 与 [`src/runtime/live_semantic_recorder.go`](../../src/runtime/live_semantic_recorder.go) 共同实现：

1. tool call 在执行任何副作用前 append。
2. journal 在锁内分配单调 `seq`，以 `record_id` 去重。
3. tool result 与其 artifact/file facts 作为同一语义批次写入。
4. 非终态记录 append 成功后才向 live bus 发布。
5. terminal 必须 flush/fsync 成功后才对外发布。

append/flush 最多尝试 3 次，退避 50 ms、200 ms；这只是短暂 I/O 恢复，不是无限重试。

## 写入故障：fail-stop

任何持久化失败被 latch 后：

- 不再启动新的副作用工具；
- Runner 尝试用独立、最多 2 秒的背景上下文写入 `persistence_error` terminal；
- live 只发布能够与耐久事实对应的错误终态；
- 不切换到内存-only 或 workspace transcript 继续运行。

这条边界保证 UI 不会声称一项不可恢复的副作用“成功完成”。若连错误终态也无法落盘，服务返回明确持久化失败并保留原文件，不伪造成功。

## Live 与 replay 的耐久来源

v2 `SemanticRecord.seq` 只定义 journal 内的单调语义顺序。live 与 replay 必须读取同一批耐久记录并经过 09 协议域定义的同一 wire projector；active EventBus 可以附带传输恢复位置，纯 durable replay 只保证 journal 语义顺序。

本文不定义 wire 字段名称、帧展开公式、SSE 查询参数或 Desktop 重连选择；这些只查 [事件与字段契约](../09_AGUI与A2UI协议/事件与字段契约.md) 和 [HTTP 与 SSE 接口契约](../09_AGUI与A2UI协议/HTTP与SSE接口契约.md)。journal record sequence 与 transport cursor 必须保持分层，不能互作订阅水位或持久主键。

## 重启与损坏恢复

- JSONL 不完整尾行可忽略；中段损坏返回 `ErrJournalMiddleCorruption`。
- 重启发现开放 tool call：追加合成 interrupted outcome，错误含义为 `process_restarted_outcome_unknown`；transport/semantic/process 均保持 unknown，不重执行工具。
- 随后追加使用02终态权威列出的 `process_restarted` reason的terminal，使线程有可解释结尾。
- v1 adapter 只映射可确认字段，不发明 artifact、process exit 或副作用状态。

## 不变量与当前不支持

- append-before-side-effect、terminal-after-fsync 不可绕过。
- live 与 replay 必须使用同一语义 projector；不得维护两套事件解释。
- canonical outcome 完整保存，UI/模型投影各自有界和脱敏。
- 当前不支持 persistence degraded continuation。
- 当前不支持重启后自动重试 outcome unknown 的工具。
- 不支持从旧 v2 count-only 记录、prompt 文字或附件目录推断未持久化的结构化附件/token。
- journal record sequence 与 09 域定义的 transport/wire sequence 不得合并成一个“序号”。

## 测试与 Desktop 验收

自动化 owner：

- [`src/agentcore/semantic_journal_test.go`](../../src/agentcore/semantic_journal_test.go)
- [`src/runtime/journal_store_semantic_test.go`](../../src/runtime/journal_store_semantic_test.go)
- [`src/runtime/live_semantic_recorder_test.go`](../../src/runtime/live_semantic_recorder_test.go)
- [`src/runtime/semantic_replay_test.go`](../../src/runtime/semantic_replay_test.go)
- [`src/runtime/semantic_user_message_test.go`](../../src/runtime/semantic_user_message_test.go)
- [`src/protocol/agui/semantic_projector_test.go`](../../src/protocol/agui/semantic_projector_test.go)
- [`apps/desktop/src/renderer/runtime/thread-transcript.test.ts`](../../apps/desktop/src/renderer/runtime/thread-transcript.test.ts)
- [`apps/desktop/src/renderer/runtime/client.test.ts`](../../apps/desktop/src/renderer/runtime/client.test.ts)

`semantic_user_message_test.go` 覆盖新 v2 记录的有序附件/token、服务重建、image blob hydrate/缺失、旧 count-only 不猜造和 v1 兼容；Desktop transcript 用例覆盖 durable run 到 prompt item 的投影。

真实 Desktop 验收需要使用新写入的完整 v2 记录，对同一线程比较重启前 live 和重启后 replay：消息、结构化附件/token、工具终态、artifact、时长/状态不得重复或漂移；人为中断运行后重开，开放工具显示“结果未知/已中断”，绝不能自动再次执行。
