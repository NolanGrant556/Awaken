# AwakenWindowTitleBar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenWindowTitleBar` 用于渲染桌面窗口顶部的系统控制区：macOS 显示三色窗口点，Windows 显示最小化、最大化和关闭按钮。父级负责传入当前平台和 Electron 窗口操作回调，本组件只负责平台对应的视觉控件，不负责拖拽区域、窗口生命周期或真实系统调用。

## 文件

- `AwakenWindowTitleBar.tsx` - 组件实现。
- `AwakenWindowTitleBar.css` - 组件局部样式入口。
- `AwakenWindowTitleBar.states.ts` - 可观测状态与模拟 props。
- `AwakenWindowTitleBar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
