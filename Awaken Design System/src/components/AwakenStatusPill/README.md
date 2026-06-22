# AwakenStatusPill

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenStatusPill` 用于展示短小的语义状态标签，例如本地运行中、需要权限、已完成、警告或错误。它适合放在侧栏条目、顶部状态区、运行摘要和列表行内；父级负责决定状态含义、文案、图标和 tone，本组件只负责紧凑的视觉呈现，不负责状态计算或交互。

## 文件

- `AwakenStatusPill.tsx` - 组件实现。
- `AwakenStatusPill.css` - 组件局部样式入口。
- `AwakenStatusPill.states.ts` - 可观测状态与模拟 props。
- `AwakenStatusPill.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
