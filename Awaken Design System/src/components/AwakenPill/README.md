# AwakenPill

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenPill` 用于展示可点击的轻量选项，例如模式筛选、上下文范围、运行视图标签或状态切换。父级负责管理选中值、禁用状态和点击逻辑；组件只负责 pill 的尺寸、边框样式、激活态、截断和可选图标展示。

## 文件

- `AwakenPill.tsx` - 组件实现。
- `AwakenPill.css` - 组件局部样式入口。
- `AwakenPill.states.ts` - 可观测状态与模拟 props。
- `AwakenPill.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
