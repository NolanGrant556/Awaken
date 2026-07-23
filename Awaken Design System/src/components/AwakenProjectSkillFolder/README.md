# AwakenProjectSkillFolder

旧项目 Skills API 的兼容语义外壳，视觉和交互统一委托给原子组件 `AwakenFolder`。新 consumer 应直接使用 `AwakenFolder`；本组件只负责把 `projectName`、`skillCount` 和 `countLabel` 映射为通用名称与辅助信息。

## Props

- `projectName`：显示在文件夹正面的小字号项目名称。
- `skillCount`：可选的 Skill 数量。
- `countLabel`：可选的自定义数量文案；传入时覆盖由 `skillCount` 生成的默认 Skill 数量，用于 Memory 等复用场景。
- `size`：`default` 保持项目 Skills 的原尺寸；`compact` 提供较小的文件夹和点击占位，供项目记忆等密集网格使用。
- `open`：受控的项目窗口状态；为 `true` 时文件夹持续保持展开，并同步 `aria-expanded`。
- `onOpen`：用户点击文件夹后触发，由 Desktop 决定打开哪个项目窗口。
- 其余按钮属性透传到根 `<button>`。

## 交互

- 鼠标悬停和键盘聚焦时，文件夹抬升；前封面轻微向左打开，两层内容页依次向右展开。
- 项目名称保持单行，超出正面可用宽度时以省略号截断；根按钮的原生标题保留完整名称供悬停查看。
- 项目 Dialog 打开期间，来源文件夹由 `open` 状态保持展开；只有关闭 Dialog 后才恢复闭合。
- `prefers-reduced-motion` 下取消 3D 展开，仅保留轻微位置反馈。
- 禁用态不响应 `onOpen`，也不播放展开动效。
- 颜色全部继承 Design System 浅色/暗色 tokens；Gallery 提供主题切换用于并行验收。

## 边界

- 不依赖 Electron、runtime、项目路径或 `window.awaken`。
- Gallery 里的项目 Skills 弹窗只用于 mock 观测；真实项目数据、弹窗以及 `SKILL.md`/Memory Markdown 预览仍由 Desktop 拥有。
