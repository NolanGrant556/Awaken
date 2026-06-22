# AwakenWorkbenchShell

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenWorkbenchShell` 用于组织 Workbench 主舞台和可选 Inspector 侧栏，通常把运行舞台作为 `stage`，把变更、终端或文件详情作为 `inspector`。父级负责生成舞台内容、Inspector tabs 和选中状态，本组件只负责两栏布局和 Inspector 区域的外层语义。

## 文件

- `AwakenWorkbenchShell.tsx` - 组件实现。
- `AwakenWorkbenchShell.css` - 组件局部样式入口。
- `AwakenWorkbenchShell.states.ts` - 可观测状态与模拟 props。
- `AwakenWorkbenchShell.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
