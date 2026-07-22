# AwakenSelectableCard

此目录包含一个独立的 Awaken 设计系统生产组件和可复用状态；它不再重复占用 Gallery 顶层入口。

## 用途

`AwakenSelectableCard` 用于创建项目流程中的来源选择。当前 Awaken Desktop 只支持从这台 Mac 选择本地文件夹，因此真实组合只展示 `Local` 卡片；组件负责选中、禁用、图标和说明布局，父级负责来源状态、文件夹选择与后续流程。

## 文件

- `AwakenSelectableCard.tsx` - 组件实现。
- `AwakenSelectableCard.css` - 组件局部样式入口。
- `AwakenSelectableCard.states.ts` - 与当前本地来源流程一致的可复用状态。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 不把尚未支持的远程仓库或云工作区展示成当前选择项。
- Gallery 在“「左侧栏」组件库”的创建项目弹窗中观测本组件；原 `awaken-selectable-card` URL 跳转到该组件族。
