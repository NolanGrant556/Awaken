# AwakenWorkbenchChrome

此目录包含 Workbench 顶部控制组件及其可观测状态；Gallery 由“「工作台框架」组件库”统一承载。

## 用途

`AwakenWorkbenchChrome` 用于承载 Workbench 顶部的 chrome 控件，包括侧栏切换、折叠态的新任务入口，以及中间标签和右侧状态两个插槽。它适合放在 `AwakenAppShell` 的 chrome 区域；父级负责侧栏折叠状态、新任务行为和标签内容，本组件只负责这些控件的顶部布局与 tooltip 包装。

## 文件

- `AwakenWorkbenchChrome.tsx` - 组件实现。
- `AwakenWorkbenchChrome.css` - 组件局部样式入口。
- `AwakenWorkbenchChrome.states.ts` - 可观测状态与模拟 props。
- `AwakenWorkbenchChrome.demo.tsx` - 工作台框架组件库内的顶部控制演示。
- `AwakenWorkbenchChrome.demo.css` - 顶部控制在完整工作台宽度内的演示样式。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- macOS 状态为原生红绿灯保留独立安全区，顶部控制不得紧贴原生窗口按钮；Desktop 主窗口固定为 100% 页面缩放，Design System 在 44px 顶部栏内以 `-0.75px` 控制偏移对齐，Electron Main 以 `trafficLightPosition: { x: 18, y: 14 }` 放置原生红绿灯，双方中心点必须处于同一条水平线。
- 公共 API 变化时同步更新可观测状态。
