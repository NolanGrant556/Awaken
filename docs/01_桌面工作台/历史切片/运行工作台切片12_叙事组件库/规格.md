# Slice 12: Narrative 组件库

## 目标

把 Run Narrative 的展示模式沉淀为可复用 UI-only 组件，减少 Desktop 主界面里手写 prompt/thinking/exploring/tool/file/answer DOM。

## 范围

- 复用已存在的 `PromptCard`、`ActivityLine`、`ThinkingBlock`、`FileChangeSummary`、`MarkdownAnswer`。
- 新增 `NarrativeFeed`、`ExploringGroup`、`ToolActionRow`。
- 新组件只通过 controlled props、children、callbacks 工作，不引入 runtime、AG-UI、A2UI、Electron、SSE 或 `window.awaken`。
- Desktop 用新组件渲染 feed、exploring、tool action，并保留 Inspector callback。
- Playground 提供 narrative 组件独立预览，覆盖 idle/running/approval/done 和长文本。

## 验收标准

- narrative 组件可在 playground 独立预览。
- 状态覆盖 active、done、error、waiting 中适用状态。
- 长文件名、长中文、长 code span 不撑破布局。
- active item 动画不造成 layout shift。
- 可交互组件可 keyboard focus，并触发 Inspector callback。

## 暂不做

- 不做复杂 markdown 插件生态。
- 不做虚拟列表。
- 不做 canvas/graph 叙事组件。
