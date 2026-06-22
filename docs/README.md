# Awaken Docs

这个目录用于记录当前项目现状、架构边界和接手指引。目标是让 0 上下文 Coding Agent 不需要读取历史对话，也能知道现在应该从哪里开始。

## 必读入口

```text
../README.md
coding-agent-handoff.md
```

## 按任务继续读

- 改 runtime、protocol、AG-UI、A2UI：读 `agui-a2ui-architecture.md`。
- 单独调试 `awaken-core`：读 `runtime-development.md`。
- 需要对照 AG-UI / A2UI 上游仓库：读 `upstream-references.md`。
- 改 Electron desktop consumer：读 `../apps/desktop/README.md`。

当前 Desktop 主产品形态是 Run Workbench。最终验收要以真实 LLM run、Desktop visual QA 和 `apps/desktop Awaken Design System` 产品路径关键词检查为准；组件 Gallery 只作为组件视觉辅助。

## 文件说明

- `../README.md`：仓库总入口。说明当前产品形态、目录、常用命令和最高层硬边界。
- `coding-agent-handoff.md`：0 上下文 Coding Agent 的执行入口。说明接手步骤、当前 owner 划分、改代码前检查和不要做的事。
- `agui-a2ui-architecture.md`：AG-UI / A2UI / runtime / desktop 的实现契约。协议映射、事件链路、approval/artifact 边界以这里为准。
- `runtime-development.md`：仅面向 Go runtime/API 调试；Desktop 使用与开发仍走 `npm run dev:desktop`。
- `upstream-references.md`：AG-UI / A2UI 上游参考说明。记录 upstream、固定 commit、clone 方式和参考范围；只有需要对照上游时才读。
- `../apps/desktop/README.md`：Electron desktop consumer 的本地开发和 bridge/adapter 契约。

## 职责边界

- 总览和常用命令放 `../README.md`。
- 具体执行顺序和工作区接手规则放 `coding-agent-handoff.md`。
- AG-UI / A2UI 的当前实现细节放 `agui-a2ui-architecture.md`。
- AG-UI / A2UI 的上游仓库与固定 commit 放 `upstream-references.md`。
- 子项目自己的启动、构建和局部边界放各自目录的 `README.md`。
- 已完成或归档的历史规格只放在明确标注为档案的文件里。

## 文档规则

本目录文档应解释当前仓库事实和工程边界。除非文件明确是档案，否则不要写历史流水账或聊天过程总结。

跨文档可以保留一句话摘要，但不要复制整段实现细节。实现细节只维护在对应唯一事实源里。
