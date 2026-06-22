# AwakenAssistantMessage

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenAssistantMessage` 用于渲染助手输出的轻量 markdown 回答，支持段落、列表、表格和行内代码，并通过 `status` 标记流式、完成或错误样式。它适合消息流或运行叙事中的助手正文；父级负责消息排序、头像、时间戳、流式更新和操作按钮，本组件只把传入的 markdown 文本转换为可读内容。

## 文件

- `AwakenAssistantMessage.tsx` - 组件实现。
- `AwakenAssistantMessage.css` - 组件局部样式入口。
- `AwakenAssistantMessage.states.ts` - 可观测状态与模拟 props。
- `AwakenAssistantMessage.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
