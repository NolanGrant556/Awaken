# Codex 源码参考

## 基线

- 路径：`/Users/nolangrant/Codex 源代码`
- 分支：`main`
- 审计 SHA：`0a12b855a0b21068108a8a3b311d492712737e0f`
- 状态：审计时干净

实现前重新核对 HEAD 和行号。

## 1. 可复用设计

### 1.1 Rolling checkpoint

本地 compact 会过滤旧 summary，保留近期真实 user messages，并追加最新 summary：

- 识别/排除旧 summary：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact.rs:540>)
- 构造近期 user messages + latest summary：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact.rs:644>)

第二次 compact 的活动历史已经包含第一次 summary，因此形成：

```text
summary_n + new history → summary_n+1
```

### 1.2 Provider-native opaque compaction

remote v2 保留符合规则的近期 items，并追加服务端返回的 opaque compaction item：

- retained message budget 当前为 64K：[compact_remote_v2.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact_remote_v2.rs:75>)
- 构造 retained items + compaction output：[compact_remote_v2.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact_remote_v2.rs:484>)

OpenAI 官方 API 把 compaction item 定义为继续长任务使用的加密、opaque 内容：[Compact a response](https://developers.openai.com/api/reference/java/resources/responses/methods/compact)。

### 1.3 World State 每 step 重建

Codex 不要求 summary 独自记住所有当前环境。每个 step 会重新构造 model instructions、personality、环境、日期、subagents、apps/tools 等 World State：[world_state.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/session/world_state.rs:35>)。

本地 compact 还区分 pre-turn/manual 与 mid-turn：前者允许下一 turn 重注入完整 initial context，后者在 compact 后立即插入 canonical context，设计说明见 [compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact.rs:64>)。

### 1.4 完整 rollout 与 replacement history 分离

压缩不会抹掉完整 rollout。resume 使用最新 replacement checkpoint 加 suffix 重建模型历史。多次 compact/resume/fork 有直接测试：[compact_resume_fork.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/tests/suite/compact_resume_fork.rs:360>)。

### 1.5 UI 10% 不是压后目标

Codex 默认约在 context window 90% 触发 auto compact；这是触发阈值，不是压后必须剩 10%。实现见 [openai_models.rs](</Users/nolangrant/Codex 源代码/codex-rs/protocol/src/openai_models.rs:487>)。

本地路径近期 user message budget 约 20K：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/src/compact.rs:62>)；remote v2 是 64K，再加 summary/opaque item 和重注入上下文。因此不要从 Codex UI 观察推导通用 10% postcondition。

## 2. Awaken 应采用

- rolling checkpoint，而不是每次从零摘要；
- 当前 World State 每轮重注入；
- 完整 session 与 replacement context 分离；
- compact/resume/fork/recompact 的一致性测试；
- exact official provider 下可选 opaque compaction；
- opaque state 与 provider/profile/model identity 绑定。

## 3. Awaken 不应照搬

- 不能把 OpenAI opaque compaction 当作多模型核心 authority；
- 不照搬 20K/64K 固定预算；Awaken 使用 global 10% allocator；
- 不把 `previous_response_id` 当 thread 持久真相；
- 不假设服务端 checkpoint 可被客户端解析或跨模型迁移；
- 不只保留 user messages；Awaken 还要保护完整 turns、tool pairing、Memory、Skill 和 Subagent handoff。

## 4. 关键测试引用

- 多次自动 compact：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/tests/suite/compact.rs:1102>)
- checkpoint 持久化：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/tests/suite/compact.rs:3559>)
- 两次 manual compact 只使用最新 summary：[compact.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/tests/suite/compact.rs:3863>)
- compact/resume/fork/compact/resume：[compact_resume_fork.rs](</Users/nolangrant/Codex 源代码/codex-rs/core/tests/suite/compact_resume_fork.rs:360>)
