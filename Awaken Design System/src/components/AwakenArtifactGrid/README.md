# AwakenArtifactGrid

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenArtifactGrid` 用于把多个文件产物按固定网格展示，并在 `items` 为空时显示“暂无产物”。它适合运行完成后的产物区或变更概览；父级负责提供 `ArtifactItem[]` 和每个产物的点击回调，网格负责列表语义、空状态和布局，单个卡片内容由 `AwakenArtifactCard` 负责。

## 文件

- `AwakenArtifactGrid.tsx` - 组件实现。
- `AwakenArtifactGrid.css` - 组件局部样式入口。
- `AwakenArtifactGrid.states.ts` - 可观测状态与模拟 props。
- `AwakenArtifactGrid.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
