# AwakenMarkdownRenderer

## 用途

`AwakenMarkdownRenderer` 是 Awaken 助手正文的富 Markdown 渲染器，负责把 Markdown 文本转换为设计系统排版。它支持 GFM、表格、任务列表、代码块和 Mermaid fenced code block；父级负责传入文本和布局容器。

## 规则

- 不渲染任意 raw HTML。
- Mermaid 只从 `mermaid` 代码块进入 `AwakenMermaidDiagram`。
- 不访问 runtime、Electron、AG-UI、A2UI 或文件系统。
