# AwakenInspectorRail

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenInspectorRail` 用于组织右侧检查器面板的标题、状态、标签入口和可滚动详情区，适合承载改动、终端、浏览器或文件等检查内容。父级负责提供 tabs、具体详情内容和状态文案；组件只负责外层网格和滚动区域，不决定有哪些检查器视图，也不管理 tab 状态。

## 文件

- `AwakenInspectorRail.tsx` - 组件实现。
- `AwakenInspectorRail.css` - 组件局部样式入口。
- `AwakenInspectorRail.states.ts` - 可观测状态与模拟 props。
- `AwakenInspectorRail.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
