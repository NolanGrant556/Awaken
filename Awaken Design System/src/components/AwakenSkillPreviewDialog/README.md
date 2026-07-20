# AwakenSkillPreviewDialog

`SKILL.md` 的受控只读阅读窗口。正文区域是窗口内唯一纵向滚动容器，完整 frontmatter 使用安全文本展示，Markdown 关闭 raw HTML、Mermaid 和外部图片请求。

## Props

- `open`、`onOpenChange`：受控窗口状态。
- `contentKey`：切换 Skill 时用于重置滚动位置。
- `frontmatterRaw`：包含原始 `---` 分隔符的完整 frontmatter。
- `markdown`：frontmatter 后的 Markdown 正文。
- `loading`、`errorMessage`：加载与失败状态。

组件不读取文件；并发请求隔离、路径安全和文件大小限制仍由 Desktop/Core 负责。
