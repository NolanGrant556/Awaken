# AwakenTooltip

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenTooltip` 用于给图标按钮、紧凑控件或截断标签补充悬停与键盘聚焦说明。它基于 Floating UI 负责定位、翻转、避让和关闭交互；父级负责提供触发元素和说明内容，本组件不改变子级控件的业务行为，也不替代可见标签。

## 文件

- `AwakenTooltip.tsx` - 组件实现。
- `AwakenTooltip.css` - 组件局部样式入口。
- `AwakenTooltip.states.ts` - 可观测状态与模拟 props。
- `AwakenTooltip.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
