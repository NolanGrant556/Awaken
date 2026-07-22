# AwakenContextButton

此目录包含输入栏内部使用的 Awaken 设计系统组件和可观测演示状态。

## 用途

`AwakenContextButton` 用于在 composer 工具栏中触发“添加上下文”的动作，视觉上是带加号的方形 icon button。它适合打开文件、目录、网页或其他上下文选择入口；父级负责实际 picker、上下文数据和禁用条件，本组件只负责固定图标、可访问标签和点击回调。

## 文件

- `AwakenContextButton.tsx` - 组件实现。
- `AwakenContextButton.css` - 组件局部样式入口。
- `AwakenContextButton.states.ts` - 可观测状态与模拟 props。
- Gallery 不再为它提供独立原子组件入口；可用与禁用状态统一在 `AwakenComposerBar.demo.tsx` 的“输入栏”组件族中观测，原 `awaken-context-button` URL 会跳转到该入口。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
