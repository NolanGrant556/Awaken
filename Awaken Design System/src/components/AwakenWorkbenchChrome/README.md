# AwakenWorkbenchChrome

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenWorkbenchChrome` 用于承载 Workbench 顶部的 chrome 控件，包括侧栏切换、折叠态的新任务入口和右侧状态插槽。它适合放在 `AwakenAppShell` 的 chrome 区域；父级负责侧栏折叠状态、新任务行为和右侧内容，本组件只负责这些控件的顶部布局与 tooltip 包装。

## 文件

- `AwakenWorkbenchChrome.tsx` - 组件实现。
- `AwakenWorkbenchChrome.css` - 组件局部样式入口。
- `AwakenWorkbenchChrome.states.ts` - 可观测状态与模拟 props。
- `AwakenWorkbenchChrome.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
