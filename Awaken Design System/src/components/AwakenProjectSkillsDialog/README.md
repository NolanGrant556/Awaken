# AwakenProjectSkillsDialog

项目 Skill 文件夹打开后的受控窗口。负责展示项目名称、路径、数量和统一 `AwakenSkillCard` 列表。

## Props

- `open`：窗口是否打开。
- `project`：当前项目及其 Skills；为 `null` 时不渲染窗口。
- `onOpenChange`：关闭或重新打开请求。
- `onSkillPreview`：卡片主体请求预览。
- `onSkillToggle`：卡片开关请求改变状态。

组件不读取项目目录、不加载 `SKILL.md`，也不拥有 runtime 状态。
