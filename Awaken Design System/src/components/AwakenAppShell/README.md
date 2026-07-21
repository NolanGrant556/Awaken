# AwakenAppShell

此目录包含 Awaken 应用外壳组件及其可观测状态；Gallery 由“「工作台框架」组件库”统一承载。

## 用途

`AwakenAppShell` 用于搭建 Awaken 应用的全局外壳，把可选顶部 chrome、左侧 sidebar 和主工作区 main 放进稳定的两栏网格。它适合桌面端或 Web 端的根布局；父级负责传入实际导航、顶部控制和工作台内容，并控制平台类型与侧栏收起状态，本组件只分配布局槽位和可见性，槽位内部组件继续负责各自交互。

## 文件

- `AwakenAppShell.tsx` - 组件实现。
- `AwakenAppShell.css` - 组件局部样式入口。
- `AwakenAppShell.demo.css` - Gallery 预览画布与槽位占位样式。
- `AwakenAppShell.states.ts` - 可观测状态与模拟 props。
- `AwakenAppShell.demo.tsx` - 工作台框架组件库内的应用外壳演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
