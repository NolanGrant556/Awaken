# AwakenProjectSkillsDialog

项目 Skill 文件夹打开后的受控固定尺寸窗口。项目名称只在窗口标题中出现，内容区展示路径、数量和统一 `AwakenSkillCard` 列表。项目列表保持约 `980 × 640`；切换到预览时复用同一个 Dialog 节点并采用约 `820 × 720` 的阅读比例，内容超出时只在正文区域滚动。

## Props

- `open`：窗口是否打开。
- `project`：当前项目及其 Skills；为 `null` 时不渲染窗口。
- `onOpenChange`：关闭或重新打开请求。
- `onSkillPreview`：卡片主体请求预览。
- `onSkillToggle`：卡片开关请求改变状态。
- `detailView`：在同一个 Dialog 内把卡片列表切换为预览或删除确认内容；`mode="preview"` 自动应用统一 Skill 预览正文布局，consumer 注入 `AwakenSkillPreviewContent` 与统一删除按钮即可。小卡片不承载删除操作，切换期间不卸载 Dialog、遮罩或项目窗口上下文。
- `onDetailClose`：详情状态下关闭按钮或 Escape 的返回请求，consumer 应切回项目 Skill 列表。

组件不读取项目目录、不加载 `SKILL.md`、不判断删除权限，也不拥有 runtime 状态。
