# AwakenToolTraceLine

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToolTraceLine` 用于在执行流中展示单次工具调用或工具结果的标题、细节和状态图标。它可以是纯展示行，也可以在传入 `onSelect` 后成为可点击按钮，用于打开终端、文件或 Inspector 详情；父级负责工具数据、选择目标和详情面板，本组件只负责单条轨迹的可读呈现。

## 文件

- `AwakenToolTraceLine.tsx` - 组件实现。
- `AwakenToolTraceLine.css` - 组件局部样式入口。
- `AwakenToolTraceLine.states.ts` - 可观测状态与模拟 props。
- `AwakenToolTraceLine.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
