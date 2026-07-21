# AwakenInspectorRail

此目录包含 Awaken Desktop 当前消费的标准右侧 Inspector rail、可观测状态与组件演示。

## 用途

`AwakenInspectorRail` 负责右侧 rail 的标题、导航项、计数/语气状态和详情插槽。父级提供 `items`、当前 `activeId`、选择回调与详情内容；组件不读取 runtime、文件或工具数据。

当前 Desktop 只传入一个禁用的“右侧栏待开发”占位项，尚未支持 Changes、Browser、Terminal、Files 标签或详情面板。Gallery 必须如实展示这一当前边界，不把可扩展 props 写成已接线能力。

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
