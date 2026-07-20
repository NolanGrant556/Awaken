# AwakenProjectSkillFolder

项目 Skills 视图的白色竖向档案资料夹入口。组件用顶部标签耳、后封套、两层纸张、较低的前封面和右上角圆形穿孔建立文件夹轮廓，只负责造型、项目名称、Skill 数量和交互回调，不读取项目目录，也不拥有 Dialog 或 Skill 预览状态。

## Props

- `projectName`：显示在文件夹正面的小字号项目名称。
- `skillCount`：可选的 Skill 数量。
- `open`：受控的项目窗口状态；为 `true` 时文件夹持续保持展开，并同步 `aria-expanded`。
- `onOpen`：用户点击文件夹后触发，由 Desktop 决定打开哪个项目窗口。
- 其余按钮属性透传到根 `<button>`。

## 交互

- 鼠标悬停和键盘聚焦时，文件夹抬升；前封面轻微向左打开，两层内容页依次向右展开。
- 项目 Dialog 打开期间，来源文件夹由 `open` 状态保持展开；只有关闭 Dialog 后才恢复闭合。
- `prefers-reduced-motion` 下取消 3D 展开，仅保留轻微位置反馈。
- 禁用态不响应 `onOpen`，也不播放展开动效。
- 颜色全部继承 Design System 浅色/暗色 tokens；Gallery 提供主题切换用于并行验收。

## 边界

- 不依赖 Electron、runtime、项目路径或 `window.awaken`。
- Gallery 里的项目 Skills 弹窗只用于 mock 观测；真实项目数据、弹窗和 `SKILL.md` 预览仍由 Desktop 拥有。
