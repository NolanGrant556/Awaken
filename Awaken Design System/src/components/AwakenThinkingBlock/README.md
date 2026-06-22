# AwakenThinkingBlock

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenThinkingBlock` 用于展示模型思考过程中的一段文本，并在标题区标记 `active` 或 `done` 状态。它适合放在执行叙事流或调试视图里承载正在流式更新的推理说明；父级负责提供文本和状态、决定何时显示或替换，本组件只负责卡片式展示和文本滚动。

## 文件

- `AwakenThinkingBlock.tsx` - 组件实现。
- `AwakenThinkingBlock.css` - 组件局部样式入口。
- `AwakenThinkingBlock.states.ts` - 可观测状态与模拟 props。
- `AwakenThinkingBlock.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
