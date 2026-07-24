# AwakenProjectSkillsDialog

项目 Skill 文件夹打开后的受控窗口。项目名称只在窗口标题中出现，内容区展示路径、数量和统一 `AwakenSkillCard` 列表。

## Props

- `open`：窗口是否打开。
- `project`：当前项目及其 Skills；为 `null` 时不渲染窗口。
- `onOpenChange`：关闭或重新打开请求。
- `onSkillPreview`：卡片主体请求预览。
- `onSkillToggle`：卡片开关请求改变状态。
- `onSkillDelete`：存在时为项目 Skill 卡片显示删除入口并回传请求。
- `deletingSkillId`：标记正在删除的 Skill，禁用其卡片操作。

组件不读取项目目录、不加载 `SKILL.md`、不判断删除权限，也不拥有 runtime 状态。
