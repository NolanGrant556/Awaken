# Context Compaction V2 开发上下文

## 目的

为另一个 Coding Agent 提供一套可独立执行的上下文压缩改造资料，使 Awaken 在多模型、重复压缩、重启和模型切换后保持任务连续性，并把压缩后的完整 provider-visible input 硬性控制在有效输入窗口的 10% 以内。

本目录只保存本次开发上下文、剩余缺口、开发顺序和参考设计，不是产品事实的第二权威。当前已经落地的能力只以 `docs/02_主智能体闭环/上下文窗口与压缩.md` 及当前代码/测试为准；本目录保留到 Provider-native 与真实 Desktop/provider matrix 完成后再按 `AGENTS.md` 删除。

## 结论先行

不要把本任务理解成修改一个比例常量。目标系统应当是：

```text
完整 SemanticJournal / Transcript（不可因压缩删除）
                    ↓
          Context Projector V2
                    ↓
 PostCompactContextBundleV2 + 当前 World State
                    ↓
            exact protocol lowering
                    ↓
  完整 provider-visible input <= EffectiveInput × 10%
```

连续性来自四条独立通道：

1. 宿主持有的 typed `TaskCheckpoint`；
2. 最近完整 turns；
3. 每轮重注入的 Instruction、Plan、Memory、Skill、Tool、Subagent 状态；
4. 可按 sequence/digest 精确回读的完整当前线程历史。

自由文本 summary 只承担语义叙事，不能承担唯一真相。

## 阅读顺序

1. [AGENTS.md](AGENTS.md)：实施约束与生命周期。
2. [当前事实与问题](00_当前事实与问题.md)：为什么现有实现会出现 30% 和压后失忆。
3. [目标架构与数据契约](01_目标架构与数据契约.md)：目标状态、数据结构、状态机和预算合同。
4. [开发计划](02_开发计划.md)：按依赖排序的实施阶段。
5. [测试与验收](03_测试与验收.md)：自动化、真实 provider 和 Desktop 门禁。
6. [代码 Owner 与改动地图](04_代码Owner与改动地图.md)：现有 owner、建议新增模块和文档同步面。
7. 三家源码参考：
   - [Codex](参考实现/Codex.md)
   - [Claude Code](参考实现/Claude-Code.md)
   - [OpenCode](参考实现/OpenCode.md)

## 本次冻结的源码基线

| 仓库 | 基线 | 说明 |
| --- | --- | --- |
| Awaken | `a7f1ac2c4c4e9756a6ee57c14438bb2ba7f06d85` | 当前实现审计基线；工作树另有用户 Plugin/MCP 修改，不属于本任务 |
| Codex | `0a12b855a0b21068108a8a3b311d492712737e0f` | `main`，审计时干净 |
| OpenCode | `10765ff2a9da8c3b88e4de873aa383a49c318912` | `dev`，审计时干净 |
| Claude Code 快照 | 无 Git 元数据 | `/Users/nolangrant/claudecode源代码` 是源码快照，不能声明 upstream HEAD |

实现 Agent 必须重新读取当前 HEAD；不要把以上 SHA 当作永远有效。

## 目标范围

### 范围内

- proactive、manual、reactive compaction；
- 单次和重复压缩；
- restart/resume/replay；
- root 与 Subagent 状态；
- Memory、Skill、RunPlan、Hook、dynamic tools；
- Chat、Responses、Messages 三协议；
- provider-native compaction 的可选适配；
- 当前线程有界历史回读；
- 完整请求硬 10% postcondition；
- Desktop 诊断与真实验收。

### 不在首版范围

- 删除或重写完整历史；
- 把 provider opaque state 转成跨供应商通用内容；
- 用向量数据库替代 thread transcript；
- 把长期 Memory 与当前 thread checkpoint 合并成一个存储；
- 为达到 10% 静默降级安全规则或用户意图；
- 未经 exact capability 证明的 provider 原生 compaction 自动探测。

## 最终交付应回答的问题

- 压缩后完整请求是否真的不超过 10%，而不是只压 conversation tail？
- 一次、三次、五次压缩后，用户要求、纠正、文件、错误、计划和 Subagent 结果是否仍可继续？
- live、restart、resume 后的 provider-visible context 是否一致？
- 模型切换时是否丢弃 provider opaque state、但保留 provider-neutral checkpoint？
- mandatory floor 超过 10% 时是否明确失败并给出分类，而不是假成功？
- 原始 transcript 是否仍可审计并被模型按需有界回读？

## 文档状态

- 当前事实：Phase 1–4 的 provider-neutral bundle、checkpoint/coverage、current-thread history、三协议 prepared-body 硬 10% 门禁及 v1/v2 replay 已在当前 dirty work 中实现并通过本地自动化；K3 `openai_chat`、kimi-for-coding `anthropic_messages`、DeepSeek `openai_responses` 已取得真实 Desktop 单次压缩与压后继续执行基线。MiniMax-M3 多 PDF 样本也已在真实 Desktop 从 254K/267K 压至 22K/267K（8%），并从压前断点继续完成任务；正式证据以 `docs/02_主智能体闭环/验收.md` 为准。
- 剩余缺口：自定义 API 低窗口多 PDF、同一任务重复 3/5 次压缩、压后完整退出重启、压后切换模型/协议和 mandatory floor 真实失败尚未完成，因此本目录暂不满足删除条件。这些项目中只有第一项是自定义 API 专项，其余属于 V2 通用稳定性验收。
- 当前实现：Provider-native adapter（Phase 5）已完成 exact OpenAI Responses mock/自动化链路；native output 只有通过 identity/digest/硬 10% 校验才物化，失败或超限回落本地 V2，Chat/Messages/custom 不继承。真实 OpenAI 账号调用以及 Windows/Linux 实机矩阵不属于本次 V2 收口要求，也不阻塞本目录后续完成判断；未执行的实机路径仍不能写成已通过。
- 三家参考：只说明可借鉴机制，不构成 Awaken 当前支持证明。
