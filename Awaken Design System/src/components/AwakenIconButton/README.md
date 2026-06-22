# AwakenIconButton

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenIconButton` 用于呈现只有图标的工具按钮，例如关闭面板、刷新状态或展开检查器。父级负责提供图标、可访问 `label` 和点击行为；组件只负责按钮尺寸、方形/圆形外观、悬浮反馈和 tooltip，不决定按钮动作，也不渲染文字按钮内容。

## 文件

- `AwakenIconButton.tsx` - 组件实现。
- `AwakenIconButton.css` - 组件局部样式入口。
- `AwakenIconButton.states.ts` - 可观测状态与模拟 props。
- `AwakenIconButton.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
