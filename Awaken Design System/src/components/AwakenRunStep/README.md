# AwakenRunStep

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenRunStep` 用于展示一次运行流程中的单个步骤，包括序号、标题、说明、标签和 pending/active/done/error 状态。父级负责决定步骤顺序和状态更新；组件只负责单行步骤的视觉反馈，通常由 `AwakenRunTimeline` 负责排列多条步骤。

## 文件

- `AwakenRunStep.tsx` - 组件实现。
- `AwakenRunStep.css` - 组件局部样式入口。
- `AwakenRunStep.states.ts` - 可观测状态与模拟 props。
- `AwakenRunStep.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
