# AwakenSkillPreviewDialog

`SKILL.md` 的受控阅读窗口。桌面视口采用约 `820 × 720` 的固定阅读比例，视口不足时自适应收缩。顶部固定展示图标、名称、紧邻名称的来源小字、说明和启停开关；中间带边框的正文区域是窗口内唯一纵向滚动容器；可删除来源在底部左侧显示删除入口，小卡片不承载删除。完整 frontmatter 使用安全等宽文本直接排在正文流中，不添加独立卡片边框、圆角或底色；fenced code block 保留代码容器的边框、圆角和底色，并只在自身横向滚动；Markdown 关闭 raw HTML、Mermaid 和外部图片请求。

## Props

- `open`、`onOpenChange`：受控窗口状态。
- `contentKey`：切换 Skill 时用于重置滚动位置。
- `frontmatterRaw`：包含原始 `---` 分隔符的完整 frontmatter。
- `markdown`：frontmatter 后的 Markdown 正文。
- `loading`、`errorMessage`：加载与失败状态。
- `skill`、`onEnabledChange`：预览顶部的受控 Skill 信息与启停。
- `deletable`、`deleting`、`onDelete`：受控底部删除操作；不提供“立刻试用”。

同目录还导出 `AwakenSkillPreviewContent`、`AwakenSkillPreviewSurface` 与 `AwakenSkillPreviewDeleteButton`，供需要持续挂载既有 Dialog 的 consumer 复用完全相同的信息区、文档区、状态和底部操作视觉。独立预览应直接消费 `AwakenSkillPreviewDialog`，不能在 consumer 中复制预览 DOM 或 CSS。

组件不读取文件；并发请求隔离、路径安全和文件大小限制仍由 Desktop/Core 负责。
