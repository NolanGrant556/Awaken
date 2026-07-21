# AwakenAssistantMessage

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenAssistantMessage` 用于渲染助手输出的富 Markdown 回答，支持 GFM、标题、强调、引用、列表、任务列表、表格、链接、行内代码、代码块和 Mermaid 图表，并通过 `status` 标记流式、完成或错误样式。它适合消息流或运行叙事中的助手正文；父级负责消息排序、头像、时间戳和流式更新。本组件也可以在完成态显示轻量底部操作区，复制源固定为传入的最终 `markdown`。

## 文件

- `AwakenAssistantMessage.tsx` - 组件实现。
- `AwakenAssistantMessage.css` - 组件局部样式入口。
- `AwakenAssistantMessage.states.ts` - 可观测状态与模拟 props。
- `AwakenAssistantMessage.demo.tsx` - 独立 Gallery 演示。
- `../AwakenMarkdownRenderer` - Markdown/GFM 渲染器。
- `../AwakenMermaidDiagram` - Mermaid 图表渲染器。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 不渲染任意 raw HTML；Mermaid 只来自 fenced code block。
- 底部复制只复制 `markdown` prop，不读取 Thought、工具轨迹或 DOM 渲染文本。
- 最终回答底部操作区固定为左侧执行时间、右侧复制按钮；`elapsedMs` 缺失时复制按钮仍保持右对齐。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
