# AwakenComposerDock

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenComposerDock` 用于承载固定在工作台底部的 composer 内容，提供停靠容器和边界样式。它适合 Run Workbench 的底部输入区；父级负责决定何时显示、传入 `AwakenComposerBar` 或其他 children，子级 composer 负责输入和提交交互，本组件只负责容器位置与外观。

## 文件

- `AwakenComposerDock.tsx` - 组件实现。
- `AwakenComposerDock.css` - 组件局部样式入口。
- `AwakenComposerDock.states.ts` - 可观测状态与模拟 props。
- `AwakenComposerDock.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
