# AwakenModeToggle

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenModeToggle` 用于在 Composer 控件区显示并切换 `Plan` 与 `Act` 两种模式。父级负责持有当前模式、处理 `onChange` 后的业务影响和禁用条件；组件只负责呈现 pill、计算下一模式并触发回调，不决定模式含义，也不直接启动执行。

## 文件

- `AwakenModeToggle.tsx` - 组件实现。
- `AwakenModeToggle.css` - 组件局部样式入口。
- `AwakenModeToggle.states.ts` - 可观测状态与模拟 props。
- `AwakenModeToggle.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
