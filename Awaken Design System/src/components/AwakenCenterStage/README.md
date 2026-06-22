# AwakenCenterStage

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenCenterStage` 用于把起始任务入口、空状态或核心 composer 限制在主工作区中央的固定宽度内。它适合应用首页、无运行时的首屏或专注输入模式；父级负责传入具体 children、选择宽度和顶部间距，子级内容负责自己的展示与交互，本组件只做居中容器。

## 文件

- `AwakenCenterStage.tsx` - 组件实现。
- `AwakenCenterStage.css` - 组件局部样式入口。
- `AwakenCenterStage.states.ts` - 可观测状态与模拟 props。
- `AwakenCenterStage.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
