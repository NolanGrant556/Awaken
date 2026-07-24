# AwakenFolder

Skills 与记忆项目入口共用的文件夹原子组件。它只拥有文件夹造型、名称、辅助信息、受控展开状态和打开回调，不读取目录，也不拥有 Dialog 或 Markdown 预览。

## Props

- `label`：文件夹名称；单行超出时省略。
- `meta`：可选辅助信息，例如 Skill 数量或记忆主题数量。
- `size`：`default` 是标准展示；`compact` 用于记忆项目；`dense` 用于项目 Skills 等需要提高列密度的网格，并保持完整按钮点击区域。
- `open`：受控展开状态，并同步 `aria-expanded`。
- `showNativeTitle`：默认保留原生 `title`，供记忆页等长名称入口悬停查看完整内容；项目 Skills 显式关闭，避免与卡面名称重复的系统提示。
- `onOpen`：用户打开文件夹时触发，由 consumer 决定后续窗口和数据。
- 其余按钮属性透传到根按钮。

## 边界

- 不含 Project、Skill、Memory、Electron、runtime 或路径语义。
- Skills 与记忆应直接消费本组件；业务窗口、项目选择和 Markdown 预览仍由各自 owner 管理。
- `AwakenProjectSkillFolder` 仅保留为旧 API 的兼容外壳。
