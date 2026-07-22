# AwakenIconButton

此目录包含一个独立的 Awaken 设计系统生产组件和可复用状态；它不再重复占用 Gallery 顶层入口。

## 用途

`AwakenIconButton` 用于呈现只有图标的工具按钮。当前真实消费包括输入栏的添加上下文按钮和各类弹窗的关闭按钮；父级负责提供图标、可访问 `label` 和点击行为，组件只负责按钮尺寸、方形/圆形外观、悬浮反馈和可选 tooltip，不决定按钮动作，也不渲染文字按钮内容。`showTooltip={false}` 只关闭可见提示与原生 `title`，不会移除按钮的 `aria-label`。

## 文件

- `AwakenIconButton.tsx` - 组件实现。
- `AwakenIconButton.css` - 组件局部样式入口。
- `AwakenIconButton.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- Gallery 在“输入栏”组件族的添加上下文动作和各组件族的真实弹窗中观测本组件；原 `awaken-icon-button` URL 跳转到“输入栏”组件族。
