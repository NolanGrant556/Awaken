# AwakenActivityLine

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenActivityLine` 用于展示执行过程中的单条活动状态，例如正在读取文件、已完成检查或运行失败。它适合放在执行时间线、工具调用摘要和任务进度列表里；父级负责提供状态来源、排序和多条记录的布局，本组件只根据 `title`、`detail`、`status` 渲染一行状态提示，不发起操作，也不管理子级内容。

## 文件

- `AwakenActivityLine.tsx` - 组件实现。
- `AwakenActivityLine.css` - 组件局部样式入口。
- `AwakenActivityLine.states.ts` - 可观测状态与模拟 props。
- `AwakenActivityLine.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
