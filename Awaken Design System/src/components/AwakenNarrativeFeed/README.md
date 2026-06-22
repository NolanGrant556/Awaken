# AwakenNarrativeFeed

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenNarrativeFeed` 用于承载一次运行叙事中的消息、步骤、产物摘要或其他子节点。它适合放在 Run Workbench 的主叙事区域；父级负责决定要展示哪些子内容、排序和空态文案，子级记录负责自身的视觉表达。当前组件只提供带 `aria-label` 的列表容器，`emptyTitle` 和 `emptyDetail` 暂未渲染。

## 文件

- `AwakenNarrativeFeed.tsx` - 组件实现。
- `AwakenNarrativeFeed.css` - 组件局部样式入口。
- `AwakenNarrativeFeed.states.ts` - 可观测状态与模拟 props。
- `AwakenNarrativeFeed.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
