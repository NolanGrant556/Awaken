# Context Compaction V2 开发代理约束

本目录是一次性开发上下文包，用于指导 Coding Agent 实现 Awaken `Context Compaction V2`。它不是当前产品能力的第二权威，也不能把目标设计写成已经支持。

## 开始顺序

1. 阅读仓库根 `README.md` 与 `AGENTS.md`。
2. 执行 `git status --short --branch`，保护所有已有修改；默认它们属于用户。
3. 按本目录 [README.md](README.md) 的阅读顺序读完全部上下文。
4. 重新核对 [当前事实与问题](00_当前事实与问题.md) 引用的 Awaken 当前代码，不能只相信本计划。
5. 重新核对三家参考仓库的 HEAD/快照；源码行号可能漂移，引用的语义才是约束。
6. 根据 [开发计划](02_开发计划.md) 建立任务计划，一次只推进一个阶段。

## 硬边界

- 不得直接把 `CompactTarget` 从 30% 改为 10% 后宣称完成。
- 完整 session/Journals 是耐久事实；模型可见上下文只是派生投影。
- 压缩必须 journal-first、fail-closed；持久化失败不得替换内存消息、清 Memory、裁 Skill、消费 Subagent handoff 或切换 Context Epoch。
- 用户要求的“硬 10%”指重建后的完整 provider-visible input，不只是 conversation tail。
- 若 mandatory floor 已超过 10%，必须返回 typed code `COMPACT_TARGET_UNACHIEVABLE`，并使用 lower-snake-case `reason` 说明不可达原因；不得静默删除用户要求、安全规则、当前用户输入或未闭合 tool pairing。
- provider opaque compaction 只能作为 exact provider/profile/protocol/model 的可选优化，不能成为多模型通用真相。
- Chat Completions、Responses、Anthropic Messages 分别 lower；不得从“OpenAI-compatible”推断原生 compaction、state 或 `previous_response_id` 支持。
- 不把自由文本摘要当作任务状态唯一来源；关键事实必须由宿主抽取、持久化和验证。
- 不拆开完整 turn 或 tool call/result pairing。
- 不删除旧 Journal/Transcript；旧 `context_compacted` 记录必须保持可重放。
- 不修改三个参考仓库。

## 文档生命周期

本目录是实施期上下文，不是长期正式知识库。实现通过后：

1. 将最终当前事实迁入 `docs/02_主智能体闭环/上下文窗口与压缩.md` 及相关唯一权威；
2. 同步 `提示词缓存与成本治理.md`、Memory、Skill、Subagent、工具与供应商文档中实际改变的事实；
3. 确认测试和 Desktop 验收引用已迁移；
4. 删除本开发上下文目录，避免形成平行规范。

## 完成声明

只有 [测试与验收](03_测试与验收.md) 中相应阶段的自动化和真实 Desktop 验收在当前工作区实际完成，才能声明该阶段完成。静态源码审计、参考产品行为、旧测试摘要或直接 provider API 不能替代 Awaken Desktop 验收。
