# AwakenLightInspectorRail

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenLightInspectorRail` 用于展示轻量检查器侧栏，把导航项、badge、增删行计数、语气色和详情内容放在同一个 aside 中。父级负责提供 items、当前 `activeId`、选中回调和详情内容；组件只负责导航按钮和详情区排版，不计算改动数量，也不拉取检查器数据。

## 文件

- `AwakenLightInspectorRail.tsx` - 组件实现。
- `AwakenLightInspectorRail.css` - 组件局部样式入口。
- `AwakenLightInspectorRail.states.ts` - 可观测状态与模拟 props。
- `AwakenLightInspectorRail.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
