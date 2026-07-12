# SemanticJournal 与重放

## 1. 权威与稳定 kind

SemanticJournal v2 是 run 的耐久事实源；09 域拥有 wire event/字段，01 域拥有 Desktop narrative。当前稳定 kind：

```text
thread, run_started, user_message, assistant_message,
tool_call, tool_result, artifact, file_change, turn_diff,
permission_decision, skill_activated, resource_receipt,
mass_message_review_requested, mass_message_review_resolved,
runtime_notice, recovery_decision, turn_boundary,
run_terminal, persistence_warning
```

`permission_decision` 是工具自动策略/transform/专用 gate 的记录，不代表通用 Approval 能力。

## 2. 写入顺序

- 用户 turn 将文本、有序 attachments 和 composer tokens 写入结构化 `user_message`；含 Skill token 的 composer 路径必须在预激活前完成该写入；
- run 主循环写 `run_started`；已准备的 composer turn 被 Agent 消费一次，不再写第二条 `user_message`；
- 有副作用工具必须在 transport 前耐久 `tool_call` 和 decision；
- Skill 成功激活在正文进入执行上下文或 child 启动前写 `skill_activated`/`resource_receipt`；加载失败不写伪 activation，而由结构化 tool error 或 run terminal 表达；
- 群发消息在任何 external call 前写 requested，用户决定后写 resolved，批准的 envelope digest 必须匹配；
- observed changes/artifacts 与 canonical `tool_result` 成批写入；
- 最后写唯一 `run_terminal`。

append/flush 失败时必须 fail-stop；不能继续副作用再补记。取消/崩溃恢复将未闭合 tool call 标为 interrupted/unknown，不把它猜成成功。

## 3. 结构化用户输入

`user_message` 保存文本、有序 attachments 和 composer tokens。附件保存 identity、kind、MIME、路径/显示名、尺寸、字节数与 runtime-owned `storage_key`；image base64 不重复写入 JSONL。重放以结构化字段为源，不从 UI 文案恢复 Slash token。

Skill 预激活失败仍保留已写入的 `user_message`；成功的多 Skill records 成批写入，批次失败时不保留部分 activation。这个顺序保证“用户输入已接受”不会被“Skill 激活失败”擦除。

## 4. 工具、环境和 provider 事实

tool call/result payload 持久保存 tool/environment snapshot ID、provider ID、capabilities、readiness、dependencies、transport/process/semantic 状态和 side-effect state。重放不重新查询当前 registry 或当前机器环境，也不因 provider 后来 ready 而改写旧 run。

模型重放使用 canonical `ModelPayload`，UI 使用 display/projection。失败工具不能在重放时凭卡片文案生成不存在的 change/artifact。

## 5. Skill 上下文恢复

`skill_activated` 保存锚定路径、正文、来源、调用模式和 receipt；服务重建时按 run 恢复 inline SkillContextBlocks。压缩按恢复后的最近调用顺序保留，单 Skill 约 5,000 tokens、合计约 25,000 tokens；6 个满预算块保留最新 5 个。截断块保留受控重读提示和精确 `read_file` 源路径。fork body 属于 child，父记录只保留调用/child 结果，不能注入父后续上下文。

## 6. 群发消息恢复

requested/resolved 是专用耐久 gate。重启后未 resolved 或已拒绝的请求不得发送；批准仅对相同 digest 生效。resolution 写入失败时不得解锁 transport。

## 7. Live 与 replay

`src/protocol/agui/semantic_projector.go` 是 live/replay 共用 projector。`SemanticRecord.seq` 只定义 journal 内单调顺序；SSE cursor、wire seq/semanticSeq 由 09 域定义。v1 adapter 只为旧数据兼容，不得反向成为新写入格式。

## 8. 代码 owner

- `src/agentcore/semantic_record.go`、`semantic_journal.go`：kind/payload 与 Runner 写入。
- `src/runtime/execution_journal.go`、`journal_store.go`、`session.go`：耐久、恢复和专用 review gate。
- `src/protocol/agui/semantic_projector.go`：唯一 wire projector。
