# AwakenWorkbenchShell

此目录包含 Workbench 主舞台与 Inspector 的布局组件、可观测状态，以及统一的“工作台框架”Gallery 入口。

## 用途

`AwakenWorkbenchShell` 负责主舞台和可选 Inspector 的两栏布局、Inspector 折叠/覆盖式展开槽位与 resize 边界。父级负责运行内容、折叠/展开状态、Inspector 表面切换和 resize 交互；组件不读取运行数据，也不决定 Inspector 详情。

Gallery 的“「工作台框架」组件库”统一展示 Desktop 当前消费的 `AwakenAppShell`、`AwakenWorkbenchChrome`、`AwakenWorkbenchShell` 和 `AwakenInspectorRail`。其中普通 Inspector 当前只在顶部显示可用的“文件”入口；点击后的项目文件工作区由 Desktop 产品层组合，不由布局组件读取文件系统。

覆盖式展开时，Inspector 占据 shell 内完整可用宽度，不改写用户已保存的普通右栏宽度。`inspectorSurfaceActive=false` 只会让表面不可见、不可交互，不会卸载它；因此 Desktop 可以在“当前会话 / 文件”顶部标签间切换并保留预览状态。

## 文件

- `AwakenWorkbenchShell.tsx` - 组件实现。
- `AwakenWorkbenchShell.css` - 组件局部样式入口。
- `AwakenWorkbenchShell.states.ts` - 当前布局状态。
- `AwakenWorkbenchShell.demo.tsx` - “工作台框架”组件库 Gallery 入口，首屏用真实组合缩略图标出四个框架组件的嵌套位置。
- `AwakenWorkbenchShell.demo.css` - Gallery 关系图、组件边界标注与响应式缩略场景样式。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 滚动、overlay、composer、折叠和 resize ownership 变化时，必须同步中间交互区契约与真实 Desktop 验收。
