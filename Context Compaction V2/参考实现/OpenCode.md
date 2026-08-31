# OpenCode 源码参考

## 基线

- 路径：`/Users/nolangrant/Opencode`
- 分支：`dev`
- 审计 SHA：`10765ff2a9da8c3b88e4de873aa383a49c318912`
- 状态：审计时干净

实现前重新核对 HEAD 和 V2/legacy 入口。

## 1. V2 可复用设计

### 1.1 Provider-neutral 固定预算与完整请求估算

V2 默认：

```text
buffer = 20,000
keep.tokens = 8,000
summary output <= 4,096
```

见 [compaction.ts](/Users/nolangrant/Opencode/packages/core/src/session/compaction.ts:12)。这些是固定 token，不是 10%。

触发前估算主要覆盖：

```text
system + messages + tools
```

见 [compaction.ts](/Users/nolangrant/Opencode/packages/core/src/session/compaction.ts:232)。Awaken 应借鉴“完整模型请求参与触发/验证”，但不能照搬四字符估算。

### 1.2 Rolling prior summary + recent

第二次压缩输入包含 prior summary、上次 recent 和本次被淘汰 head：[compaction.ts](/Users/nolangrant/Opencode/packages/core/src/session/compaction.ts:178)。

新 checkpoint 持久化 summary 与 recent：[compaction.ts](/Users/nolangrant/Opencode/packages/core/src/session/compaction.ts:220)。下一请求包装成 `<conversation-checkpoint>`：[to-llm-message.ts](/Users/nolangrant/Opencode/packages/core/src/session/runner/to-llm-message.ts:147)。

### 1.3 Context Epoch

Context Epoch 单独保存 baseline、system source snapshots 和事件序号，压缩边界可以重建新 baseline：[context-epoch.ts](/Users/nolangrant/Opencode/packages/core/src/session/context-epoch.ts:46)。

每个 provider attempt 会重新选择 agent/model、加载 system/skill/reference guidance 并物化工具：[runner/llm.ts](/Users/nolangrant/Opencode/packages/core/src/session/runner/llm.ts:168)。

### 1.4 Session 与模型 context 分离

- 完整 messages 投影：[session.ts](/Users/nolangrant/Opencode/packages/core/src/session.ts:304)
- 压缩后的 context view：[store.ts](/Users/nolangrant/Opencode/packages/core/src/session/store.ts:39)
- durable aggregate history：[session.ts](/Users/nolangrant/Opencode/packages/core/src/session.ts:352)

压缩改变模型视图，不等于删除 session 历史。

### 1.5 重复压缩测试

- 第二次压缩携带 prior summary/recent：[session-runner.test.ts](/Users/nolangrant/Opencode/packages/core/test/session-runner.test.ts:1078)
- recent 只保留完整序列化消息，不切半：[session-runner.test.ts](/Users/nolangrant/Opencode/packages/core/test/session-runner.test.ts:1151)
- compact 后重建 Context Epoch baseline：[session-runner.test.ts](/Users/nolangrant/Opencode/packages/core/test/session-runner.test.ts:1039)
- provider overflow 只恢复一次：[session-runner.test.ts](/Users/nolangrant/Opencode/packages/core/test/session-runner.test.ts:1212)

## 2. Legacy 参考

Legacy recent tail：

```text
keep = clamp(floor(usable × 25%), 2,000, 15,000)
```

见 [legacy compaction.ts](/Users/nolangrant/Opencode/packages/opencode/src/session/compaction.ts:115)。它保留原始结构化 recent turns，并滚动合并 previous summary：[legacy compaction.ts](/Users/nolangrant/Opencode/packages/opencode/src/session/compaction.ts:363)。

Legacy 每轮重新装配 tools、MCP、permissions、plugins、environment、instructions 和 reminders，入口见 [prompt.ts](/Users/nolangrant/Opencode/packages/opencode/src/session/prompt.ts:1180)。

## 3. Awaken 应采用

- provider-neutral `summary + recent` checkpoint；
- prior checkpoint 显式滚动合并；
- Context Epoch 保存 system baseline/source snapshots；
- session/history 与模型 context view 分离；
- system/messages/tools 共同参与容量判断；
- 多次 compact 和完整消息边界测试；
- provider overflow 只做一次有界 reactive recovery。

## 4. Awaken 不应照搬

- V2 当前 token 估算约为序列化字符数/4，不是 provider tokenizer；Awaken 应继续使用分类 estimator + provider anchor 校准；
- V2 recent 是序列化 string，工具结果会截断；Awaken 应保留 typed complete turns 和 durable refs；
- V2 的固定 8K/4K/20K 不适合硬 10% 多模型合同；
- 不忽略模型独立 input limit；
- 不把 Todo/Plan 只留给 summary；Awaken 已有 RunPlan authority，应显式引用；
- 不把 legacy 25% tail 当完整请求 postcondition。

## 5. 测试环境边界

审计时尝试收集聚焦测试，但当前 checkout 缺少 `node_modules` 中的 `@opencode-ai/llm` 与 `effect`，测试在 collection 阶段失败。因此以上测试是当前源码中的设计证据，不是本次实际通过记录。
