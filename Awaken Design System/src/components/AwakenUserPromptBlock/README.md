# AwakenUserPromptBlock

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenUserPromptBlock` 用于在执行流中展示用户发起本轮任务的原始提示文本。它适合出现在运行叙事的开头，帮助用户回看当前 run 的目标；父级负责保存提示、决定顺序和是否显示，本组件只负责提示内容的块状展示。

## 文件

- `AwakenUserPromptBlock.tsx` - 组件实现。
- `AwakenUserPromptBlock.css` - 组件局部样式入口。
- `AwakenUserPromptBlock.states.ts` - 可观测状态与模拟 props。
- `AwakenUserPromptBlock.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
