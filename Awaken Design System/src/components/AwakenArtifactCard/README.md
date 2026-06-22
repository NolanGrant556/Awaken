# AwakenArtifactCard

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenArtifactCard` 用于展示单个文件产物或变更项，包含标题、路径、说明和新增/修改/删除/移动图标。它适合放在运行结果、变更摘要或 Inspector 产物列表中；父级负责提供产物数据和点击后的打开逻辑，本组件只负责单项视觉表达，是否成组排列由 `AwakenArtifactGrid` 或其他父级容器处理。

## 文件

- `AwakenArtifactCard.tsx` - 组件实现。
- `AwakenArtifactCard.css` - 组件局部样式入口。
- `AwakenArtifactCard.states.ts` - 可观测状态与模拟 props。
- `AwakenArtifactCard.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
