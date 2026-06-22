# AwakenPromptCard

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenPromptCard` 用于在运行叙事中固定展示用户提交的原始任务提示、来源标签和处理状态。父级负责传入 prompt 文本、状态文案以及它在 feed 中的位置；组件只负责卡片式呈现、长文本滚动和状态标签展示，不修改 prompt 内容。

## 文件

- `AwakenPromptCard.tsx` - 组件实现。
- `AwakenPromptCard.css` - 组件局部样式入口。
- `AwakenPromptCard.states.ts` - 可观测状态与模拟 props。
- `AwakenPromptCard.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
