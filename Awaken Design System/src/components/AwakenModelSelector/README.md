# AwakenModelSelector

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenModelSelector` 用于在 Composer 控件区选择当前模型，并通过下拉列表展示可选、禁用或空模型状态。父级负责提供真实模型列表、当前 `value`、可用性判断和 `onChange` 后的配置更新；组件只维护下拉开合、渲染选项和拦截禁用项，不探测 provider，也不保存模型偏好。

## 文件

- `AwakenModelSelector.tsx` - 组件实现。
- `AwakenModelSelector.css` - 组件局部样式入口。
- `AwakenModelSelector.states.ts` - 可观测状态与模拟 props。
- `AwakenModelSelector.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
