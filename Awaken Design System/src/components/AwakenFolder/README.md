# AwakenFolder

Skills 与记忆项目入口共用的文件夹原子组件。它只拥有文件夹造型、名称、辅助信息、受控展开状态和打开回调，不读取目录，也不拥有 Dialog 或 Markdown 预览。

## Props

- `label`：文件夹名称；单行超出时省略，原生 `title` 保留完整内容。
- `meta`：可选辅助信息，例如 Skill 数量或记忆主题数量。
- `size`：`default` 用于 Skills，`compact` 用于记忆项目等密集网格。
- `open`：受控展开状态，并同步 `aria-expanded`。
- `onOpen`：用户打开文件夹时触发，由 consumer 决定后续窗口和数据。
- 其余按钮属性透传到根按钮。

## 边界

- 不含 Project、Skill、Memory、Electron、runtime 或路径语义。
- Skills 与记忆应直接消费本组件；业务窗口、项目选择和 Markdown 预览仍由各自 owner 管理。
- `AwakenProjectSkillFolder` 仅保留为旧 API 的兼容外壳。
