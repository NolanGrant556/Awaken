# AwakenInspectorTabs

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenInspectorTabs` 用于在检查器内切换固定视图，包括改动、浏览器、终端和文件，并支持 badge、禁用态和键盘方向键导航。父级负责持有 `activeTabId`、渲染对应面板和处理 `onTabChange`；组件只负责 tablist 交互和按钮状态，不承载具体面板内容。

## 文件

- `AwakenInspectorTabs.tsx` - 组件实现。
- `AwakenInspectorTabs.css` - 组件局部样式入口。
- `AwakenInspectorTabs.states.ts` - 可观测状态与模拟 props。
- `AwakenInspectorTabs.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
