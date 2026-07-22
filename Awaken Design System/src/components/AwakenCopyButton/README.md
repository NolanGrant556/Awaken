# AwakenCopyButton

此目录包含一个独立的 Awaken 设计系统组件及其可观测状态。它不再单独占用 Gallery 目录入口；真实复制状态随“助手消息”和“输入栏”组件族一起观测。

## 用途

`AwakenCopyButton` 用于把指定文本复制到剪贴板，并在按钮内展示复制中后的成功或失败反馈。它适合放在回答、代码片段、文件路径或终端命令旁边；父级负责传入要复制的 `text`、展示文案和业务回调，组件只负责触发复制、显示 tooltip/按钮状态，不决定复制内容来源，也不持久化复制结果。

## 文件

- `AwakenCopyButton.tsx` - 组件实现。
- `AwakenCopyButton.css` - 组件局部样式入口。
- `AwakenCopyButton.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- 原 `awaken-copy-button` URL 跳转到“助手消息”组件库；组件源码和公开导出继续供 `AwakenAssistantMessage`、`AwakenUserPromptBlock` 等正式组件使用。
