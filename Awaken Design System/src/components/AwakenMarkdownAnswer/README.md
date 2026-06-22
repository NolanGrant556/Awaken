# AwakenMarkdownAnswer

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenMarkdownAnswer` 用于展示 Agent 回答中的简化 Markdown 内容，并用 active/done/error 表达生成状态。父级负责传入回答文本和状态；组件只解析段落、无序/有序列表和行内 code，不负责完整 Markdown 语法、代码高亮、来源引用或消息流控制。

## 文件

- `AwakenMarkdownAnswer.tsx` - 组件实现。
- `AwakenMarkdownAnswer.css` - 组件局部样式入口。
- `AwakenMarkdownAnswer.states.ts` - 可观测状态与模拟 props。
- `AwakenMarkdownAnswer.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
