# AwakenButton

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenButton` 用于承载 Awaken 界面里的文字按钮操作，提供 default、primary、ghost 三种层级，以及尺寸、图标、加载和禁用状态。它适合表单提交、审批动作和普通工具栏命令；父级负责业务语义、点击回调和何时禁用，本组件只负责按钮视觉、基础交互反馈和原生 button 属性透传。

## 文件

- `AwakenButton.tsx` - 组件实现。
- `AwakenButton.css` - 组件局部样式入口。
- `AwakenButton.states.ts` - 可观测状态与模拟 props。
- `AwakenButton.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
