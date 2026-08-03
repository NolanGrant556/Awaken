# AwakenAssistantMessage

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenAssistantMessage` 用于渲染助手输出的富 Markdown 回答，支持 GFM、标题、强调、引用、列表、任务列表、文章式表格、链接、行内代码、带语言标签与语法着色的代码块和 Mermaid 图表，并通过 `status` 标记流式、完成或错误样式。它适合消息流或运行叙事中的助手正文；父级负责消息排序、头像、时间戳和流式更新。本组件也可以在完成态显示轻量底部操作区，复制源固定为传入的最终 `markdown`。

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
- fenced code block 只按模型明确声明的语言着色，不做语言猜测；语言标签、token 颜色、行内代码和横线表格共同构成文章式阅读排版，未知语言仍以纯代码显示。
- 底部复制只复制 `markdown` prop，不读取 Thought、工具轨迹或 DOM 渲染文本。
- 通用组件仍接受可选 `elapsedMs` 以展示左侧时间元数据；Awaken Desktop 当前不传该值，最终回答底部只保留右侧无底卡复制图标，不显示 Tooltip 或原生 title 文字，并保留 hover、键盘焦点和纯图标复制成功/失败反馈。无障碍名称继续保留。唯一执行时间由 turn 顶部及其终态 process disclosure 标题承担。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
