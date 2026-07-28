# AwakenRunStage

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenRunStage` 用于构成运行工作区的主舞台：上方是可滚动的运行叙事内容，下方可挂载 composer。父级负责提供具体 feed、时间线、输入区、滚动 ref 和滚动事件处理；组件只负责区域结构和滚动容器，不管理运行数据。

父级传入 `contentInactive` 时，只会隐藏并禁用叙事滚动区，不会卸载或隐藏 composer。这个状态用于 Inspector 覆盖中间区域时继续保留底部输入。

## 文件

- `AwakenRunStage.tsx` - 组件实现。
- `AwakenRunStage.css` - 组件局部样式入口。
- `AwakenRunStage.states.ts` - 可观测状态与模拟 props。
- `AwakenRunStage.demo.tsx` - “运行阶段”组件族 Gallery 演示；先展示 `AwakenRunStage + AwakenExecutionFeed + AwakenComposerDock` 的真实组合，再补充 RunStage 自身状态。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
