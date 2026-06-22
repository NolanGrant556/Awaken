# AwakenDivider

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenDivider` 用于在面板、表单、叙事流或设置区块之间放置横向分隔线，并通过 `spacing` 调整上下留白。父级负责决定分隔线出现的位置和上下文语义；组件只输出一条视觉分隔线，不承载标题、内容或布局分组逻辑。

## 文件

- `AwakenDivider.tsx` - 组件实现。
- `AwakenDivider.css` - 组件局部样式入口。
- `AwakenDivider.states.ts` - 可观测状态与模拟 props。
- `AwakenDivider.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
