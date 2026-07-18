# AwakenMermaidDiagram

## 用途

`AwakenMermaidDiagram` 用于把已闭合的 Mermaid 图表源码渲染为 SVG。它面向 Markdown renderer 使用，也可以被其他只读 UI 容器复用。

## 规则

- 使用 Mermaid `startOnLoad: false` 和 `securityLevel: "strict"`。
- 渲染失败时显示原始 Mermaid 代码块和简短错误信息。
- 渲染成功时提供展开查看、展开态缩放、自由拖拽平移和复制 Mermaid 源码，不复制渲染后的 SVG。
- 相同源码可以复用 SVG 模板缓存，但每个 inline/expanded 挂载实例必须重新作用域化内部 `id` 及引用，禁止把缓存的完整 DOM 身份原样复用。
- 不执行 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
