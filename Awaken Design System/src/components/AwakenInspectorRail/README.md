# AwakenInspectorRail

此目录包含 Awaken Desktop 当前消费的标准右侧 Inspector rail、可观测状态与组件演示。

## 用途

`AwakenInspectorRail` 负责右侧 rail 的可选标题、导航项、计数/语气状态和详情插槽。父级提供 `items`、当前 `activeId`、选择回调与详情内容；组件不读取 runtime、文件或工具数据。没有标题时导航从内容区顶部开始。

当前 Desktop 在普通 Inspector 中固定提供“文件”和“浏览器”入口；存在 Subagent 时可增加“子任务”，用户明确固定媒体后可增加“媒体”。点击文件/浏览器后 Desktop 用产品侧 `ProjectFilesWorkspace` 替换这个 rail；子任务和媒体详情作为 `children` 进入详情插槽。文件夹组、筛选、目录读取、Artifact identity 和路径安全都不属于本组件。当前仍未支持通用 tool/artifact tabs、Changes、Terminal、侧边任务或编辑保存。Gallery 必须如实展示这一边界。

## 文件

- `AwakenInspectorRail.tsx` - 组件实现。
- `AwakenInspectorRail.css` - 组件局部样式入口。
- `AwakenInspectorRail.states.ts` - 当前 Desktop 可观测状态。
- `AwakenInspectorRail.demo.tsx` - Gallery 中带主舞台上下文的完整右栏演示。
- `AwakenInspectorRail.demo.css` - 300px 右栏宽度和完整高度预览样式。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 或 Desktop 当前支持边界变化时，同步更新状态、演示和右侧栏权威文档。
