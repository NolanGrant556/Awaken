# AwakenTopBar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenTopBar` 用于提供页面顶部的粘性操作区，默认把子级内容排列到右侧，适合承载运行状态、工作区选择、筛选或页面级操作。父级负责决定放入哪些控件和它们的交互，本组件只负责顶部栏的高度、边界和布局容器。

## 文件

- `AwakenTopBar.tsx` - 组件实现。
- `AwakenTopBar.css` - 组件局部样式入口。
- `AwakenTopBar.states.ts` - 可观测状态与模拟 props。
- `AwakenTopBar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
