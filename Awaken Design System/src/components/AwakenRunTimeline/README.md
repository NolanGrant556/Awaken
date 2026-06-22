# AwakenRunTimeline

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenRunTimeline` 用于汇总一次运行的整体状态和步骤列表，帮助用户看清任务从准备、运行到完成或出错的进度。父级负责传入标题、运行状态、步骤数据和可选的补充子内容；组件负责面板、状态标识、空步骤提示以及调用 `AwakenRunStep` 渲染每一步。

## 文件

- `AwakenRunTimeline.tsx` - 组件实现。
- `AwakenRunTimeline.css` - 组件局部样式入口。
- `AwakenRunTimeline.states.ts` - 可观测状态与模拟 props。
- `AwakenRunTimeline.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
