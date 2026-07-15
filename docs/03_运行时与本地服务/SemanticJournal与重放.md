# SemanticJournal 与重放

## 1. 权威与稳定 kind

SemanticJournal v2 是 run 内容与恢复的耐久事实源；09 域拥有 wire event/字段，01 域拥有 Desktop narrative。当前稳定 kind：

```text
thread, run_started, user_message, assistant_message,
tool_call, tool_result, artifact, file_change, turn_diff,
permission_decision, skill_activated, resource_receipt,
runtime_notice, recovery_decision, context_compacted, turn_boundary,
run_terminal, persistence_warning
```

`permission_decision` 是工具自动策略或 transform 的记录，不代表通用 Approval 能力。

## 2. Journal-first 写入顺序

- composer 将文本、有序 attachments 和 tokens 写入结构化 `user_message`；含 Skill token 的路径必须在预激活前完成该写入；
- run 主循环写 `run_started`，已准备的 composer turn 被 Agent 消费一次，不再写第二条 user message；
- 有副作用工具必须在 transport 前耐久 `tool_call` 和 decision；
- Skill 成功激活在正文进入执行上下文或 child 启动前写 `skill_activated`/`resource_receipt`；失败不写伪 activation；
- observed changes、artifacts 与 canonical `tool_result` 成批写入；
- 上下文压缩结果在替换内存消息前写并 flush `context_compacted`；
- 最后写唯一 `run_terminal`。

append 或 flush 失败必须 fail-stop；不能继续副作用或先切换内存上下文再补记。取消/崩溃恢复将未闭合 tool call 标为 interrupted/unknown，不猜成成功。

## 3. 结构化用户输入与附件

`user_message` 保存文本、有序 attachments 和 composer tokens。附件保存 identity、kind、MIME、路径/显示名、尺寸、字节数与 runtime-owned `storage_key`；image base64 不写入 JSONL。重放以结构化字段为源，不从 UI 文案恢复 Slash token，也不扫描附件目录发明历史。

Skill 预激活失败仍保留已写入的 `user_message`；成功的多 Skill records 成批写入，批次失败不保留部分 activation。

## 4. `context_compacted` 契约

`context_compacted` payload 保存：

```text
compact_boundary_id
mode
messages[]
pre_estimate_tokens
post_estimate_tokens
agent_id               # 仅 child runner
```

每条 compacted message 保存 role、content、tool calls/result pairing、name、error 标志、boundary ID 和无 blob 的 attachment identity/storage key。第一条消息必须带与 payload 一致的 boundary；role 只允许 user/assistant/tool；完整消息集必须通过 tool-result pairing 校验。

重放按 journal seq 处理：遇到合法 `context_compacted` 时，用其完整 `messages` 替换之前累计的模型上下文，并清空待合并 assistant 片段；之后的记录继续追加。记录缺 messages、boundary 不一致、role 非法或 tool pairing 非法时，保留压缩前最后一份有效上下文，不让坏记录擦除历史。该记录仍留在 journal 供诊断。

父 runner 与 child runner 各自持久化自己的压缩边界；child payload 带 `agent_id`，不能把 child body 注入父上下文。

## 5. 工具、环境与 provider 事实

tool call/result payload 持久保存 tool/environment snapshot ID、provider ID、capabilities、readiness、dependencies、transport/process/semantic 状态和 side-effect state。重放不查询当前 registry 或当前机器环境，也不因 provider 后来 ready 而改写旧 run。

模型历史使用 canonical `ModelPayload`，UI 使用 display/projection。失败工具不能在重放时凭卡片文案生成不存在的 change/artifact。

## 6. Skill 与任务输入恢复

`skill_activated` 保存锚定路径、正文、来源、调用模式和 receipt；服务重建时按 run 恢复 inline SkillContextBlocks。压缩按恢复后的最近调用顺序保留，单 Skill 约 5,000 tokens、合计约 25,000 tokens；6 个满预算块保留最新 5 个。截断块必须保留精确 `read_file` 源路径。

消息发送不写 requested/resolved 审核记录，也不在重启恢复时重建 pending review；它只保留普通 tool call/result 与 provider outcome 事实。

## 7. Live 与 replay

`src/protocol/agui/semantic_projector.go` 是 live/replay 共用 projector。`SemanticRecord.seq` 只定义 journal 内单调顺序；SSE cursor、wire seq/semanticSeq 由 09 域定义。旧 v1 adapter 只读既有测试/兼容输入，不得成为新写入格式。

## 8. Owner 与验证

- kind/payload 与 Runner：`src/agentcore/semantic_record.go`、`semantic_journal.go`、`runner.go`。
- 耐久与模型历史恢复：`src/runtime/journal_store.go`、`transcript_store.go`、`session.go`。
- wire 投影：`src/protocol/agui/semantic_projector.go`。

自动化必须覆盖压缩 append/flush 先于内存替换、持久化失败保留旧内存上下文、重建 `RunService` 后 summary+tail 精确恢复、坏 compaction fail closed、tool pairing 与 attachment storage key 重放。真实 Desktop 还需执行一次触发压缩的长会话，完整重启后继续追问并检查 provider request 使用压缩后的上下文。
