# Claude Code 源码参考

## 基线

- 路径：`/Users/nolangrant/claudecode源代码`
- Git：该目录没有 `.git`，不能声明当前 upstream HEAD
- 测试：该快照没有完整 package/test 环境，引用为静态源码证据

实现前确认快照是否更新、缺失文件是否补齐。

## 1. 可复用设计

### 1.1 面向继续工作的 full compact prompt

摘要 prompt 要求覆盖用户消息、文件/代码、错误修正、待办、当前工作和下一步：[prompt.ts](/Users/nolangrant/claudecode源代码/src/services/compact/prompt.ts:61)。

压后提示还包含完整 transcript 路径，并要求需要精确旧内容时 Read，同时直接继续任务、不向用户表现出上下文断裂：[prompt.ts](/Users/nolangrant/claudecode源代码/src/services/compact/prompt.ts:337)。

### 1.2 递归 summary

下一次 summarizer 读取上一 compact boundary 后的内容，其中包括上一次 summary 和新增历史，见 [compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:1292)。

```text
summary_n + new history → summary_n+1
```

### 1.3 压后状态重新注入

full compact 后会独立重新挂载：

- 最近读取文件，单文件/总量有预算：[compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:1415)
- 当前 Plan/Plan Mode：[compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:1470)
- 已调用 Skills 及路径：[compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:1494)
- 异步 Agents/未领取状态：[compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:1562)
- deferred tools、Agent 列表和 MCP 指令：[compact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/compact.ts:563)

这解释了为什么摘要即使有损，当前工作状态仍有其他来源补回。

### 1.4 Task 与 transcript 独立持久化

Task V2 独立存储在 session/team 目录，不依赖聊天摘要：[tasks.ts](/Users/nolangrant/claudecode源代码/src/utils/tasks.ts:190)。

完整会话写 JSONL；compact boundary 成为模型视图的新根，但旧物理记录仍存在。相关 owner：

- transcript 路径：[sessionStorage.ts](/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:198)
- compact boundary 持久化：[sessionStorage.ts](/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:993)
- preserved segment 恢复：[sessionStorage.ts](/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:1823)
- reload 按 parent chain 重建：[sessionStorage.ts](/Users/nolangrant/claudecode源代码/src/utils/sessionStorage.ts:3869)

### 1.5 分层压缩

Claude Code 不是只有 full compact：还存在工具结果控制、microcompact/cache edit、可选 Session Memory 和 full compact。其 auto compact 采用 output reserve 和绝对 buffer，而不是统一压后 10%：[autoCompact.ts](/Users/nolangrant/claudecode源代码/src/services/compact/autoCompact.ts:28)。

## 2. Awaken 应采用

- summary 中保存当前 transcript locator；
- 精确旧内容使用受控 Read，不长期占满上下文；
- 文件、Plan、Skill、Agent、deferred tool 状态独立重注入；
- active task 与完整 transcript 分离；
- microcompact 先处理旧工具结果/媒体，再 full compact；
- 压缩对用户保持单一连续任务感。

## 3. Awaken 不应照搬

- Claude 的 summary 仍是 soft schema；Awaken 需要 typed facts 和 coverage gate；
- 不照搬最高 50K 文件、25K Skill 等独立固定预算；它们与硬 10% 冲突；
- Session Memory 路径在该快照中有 feature gate，不能当成所有版本默认能力；
- 不暴露本地 transcript 物理路径给模型；Awaken 使用 typed locator/tool；
- 不依赖自然语言“不要承认压缩”掩盖真实语义缺失。

## 4. 10% 口径提醒

当前源码公式使用最多 20K summary output reserve 和 13K auto compact buffer。20K 在 200K 窗口上碰巧是 10%，但不是压后完整上下文目标。压后还可能重新加入 system、tools、文件、Skills、Plan 和 Agents。
