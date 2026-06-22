# AwakenExploringGroup

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenExploringGroup` 用于在执行叙事中归组展示探索阶段的多个工具动作，例如读取文件、搜索引用或检查依赖。父级负责提供动作列表、整体状态和点击后的业务处理；组件只负责把动作渲染为 `AwakenToolActionRow` 列表，不执行工具调用，也不维护探索流程。

## 文件

- `AwakenExploringGroup.tsx` - 组件实现。
- `AwakenExploringGroup.css` - 组件局部样式入口。
- `AwakenExploringGroup.states.ts` - 可观测状态与模拟 props。
- `AwakenExploringGroup.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
