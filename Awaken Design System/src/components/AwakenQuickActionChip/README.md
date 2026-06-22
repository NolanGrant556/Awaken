# AwakenQuickActionChip

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenQuickActionChip` 用于展示一条可点击的快捷任务建议，点击时把完整 prompt 交给父级处理。它适合放在欢迎态、空运行区或 composer 附近；父级负责把 prompt 写入输入框或立即执行，组件只负责 chip 的标签、图标、禁用态和点击回调。

## 文件

- `AwakenQuickActionChip.tsx` - 组件实现。
- `AwakenQuickActionChip.css` - 组件局部样式入口。
- `AwakenQuickActionChip.states.ts` - 可观测状态与模拟 props。
- `AwakenQuickActionChip.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
