# AwakenFileChangeSummary

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenFileChangeSummary` 用于汇总一次执行产生的文件改动，展示文件标题、路径、说明和每个文件的 active/done/error 状态。父级负责从真实 diff、计划或运行结果中整理 `files` 数据；组件只负责展示列表、状态图标和空列表提示，不读取文件系统，也不计算 diff。

## 文件

- `AwakenFileChangeSummary.tsx` - 组件实现。
- `AwakenFileChangeSummary.css` - 组件局部样式入口。
- `AwakenFileChangeSummary.states.ts` - 可观测状态与模拟 props。
- `AwakenFileChangeSummary.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
