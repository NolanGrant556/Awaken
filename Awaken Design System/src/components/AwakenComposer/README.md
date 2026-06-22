# AwakenComposer

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenComposer` 用于组合完整任务输入体验：文本输入、添加上下文、Plan/Act 模式切换、模型选择和发送/停止按钮。它适合中心任务入口或较完整的输入面板；父级负责受控 value、模式/模型状态、ready 判断和提交后的运行逻辑，本组件负责把子控件编排成一个 composer 表面并向外回调。

## 文件

- `AwakenComposer.tsx` - 组件实现。
- `AwakenComposer.css` - 组件局部样式入口。
- `AwakenComposer.states.ts` - 可观测状态与模拟 props。
- `AwakenComposer.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
