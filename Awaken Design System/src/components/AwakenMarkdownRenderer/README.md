# AwakenMarkdownRenderer

## 用途

`AwakenMarkdownRenderer` 是 Awaken 助手正文的富 Markdown 渲染器，负责把 Markdown 文本转换为设计系统排版。它支持 GFM、文章式表格、任务列表、带语言标签和语法着色的 fenced code block，以及 Mermaid fenced code block；父级负责传入文本和布局容器。

## 规则

- 不渲染任意 raw HTML。
- 普通 fenced code block 只按显式 `language-*` 声明高亮，不自动猜测语言；未知语言保留原始代码与语言标签。
- 语法着色由 `rehype-highlight` 在 HAST 上生成 token，随后仍经过扩展了 token class 白名单的 `rehype-sanitize`；组件不注入来自模型的 HTML。
- Mermaid 只从 `mermaid` 代码块进入 `AwakenMermaidDiagram`。
- Mermaid 不进入普通语法高亮；只有闭合 fence 才渲染图表，未闭合或失败时保留统一代码表面。
- 不访问 runtime、Electron、AG-UI、A2UI 或文件系统。
