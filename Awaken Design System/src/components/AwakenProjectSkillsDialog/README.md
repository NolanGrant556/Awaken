# AwakenProjectSkillsDialog

项目 Skill 文件夹打开后的受控固定尺寸窗口。项目名称只在窗口标题中出现，内容区默认以“文件夹数量 + 主文件夹名称”的紧凑摘要表达来源。摘要打开紧凑 `AwakenPopover` 后，“全部文件夹”和每个项目文件夹都是可选择的筛选项，零 Skill 文件夹仍可选择并显示文件夹空态；列表只显示名称、主标记和数量，不展示路径。Skill 卡片只显示其来源文件夹名称。项目列表保持约 `980 × 640`；切换到预览时复用同一个 Dialog 节点并采用约 `820 × 720` 的阅读比例，内容超出时只在正文区域滚动。

## Props

- `open`：窗口是否打开。
- `project`：当前项目及其 Skills；可通过 `folders` 注入文件夹名称、内部路径 identity 与主文件夹标记，并通过每个 Skill 的 `folderPath` 绑定来源文件夹，为 `null` 时不渲染窗口。路径不进入可见 UI，`path` 只作为单文件夹旧数据的兼容回退。
- `onOpenChange`：关闭或重新打开请求。
- `onSkillPreview`：卡片主体请求预览。
- `onSkillToggle`：卡片开关请求改变状态。
- `detailView`：在同一个 Dialog 内把卡片列表切换为预览或删除确认内容；`mode="preview"` 自动应用统一 Skill 预览正文布局，consumer 注入 `AwakenSkillPreviewContent` 与统一删除按钮即可。小卡片不承载删除操作，切换期间不卸载 Dialog、遮罩或项目窗口上下文。
- `onDetailClose`：详情状态下关闭按钮或 Escape 的返回请求，consumer 应切回项目 Skill 列表。

组件不读取项目目录、不推断项目文件夹组成、不加载 `SKILL.md`、不判断删除权限，也不拥有 runtime 状态。
