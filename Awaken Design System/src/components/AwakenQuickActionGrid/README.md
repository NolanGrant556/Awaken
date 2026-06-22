# AwakenQuickActionGrid

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenQuickActionGrid` 用于排列多条快捷任务建议，并在宽度不足时自动换行居中。父级负责生成每个快捷项、处理点击和空态；组件只负责 flex 布局，不知道子项是 chip、按钮还是其他轻量入口。

## 文件

- `AwakenQuickActionGrid.tsx` - 组件实现。
- `AwakenQuickActionGrid.css` - 组件局部样式入口。
- `AwakenQuickActionGrid.states.ts` - 可观测状态与模拟 props。
- `AwakenQuickActionGrid.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
