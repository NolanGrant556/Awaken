# AwakenKbd

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenKbd` 用于以内联键帽形式展示快捷键提示，并根据平台给普通按键补齐 `⌘` 或 `Ctrl+` 前缀。父级负责决定要展示哪个快捷键以及平台来源；组件只负责视觉样式和前缀展示，不绑定键盘事件，也不注册快捷键。

## 文件

- `AwakenKbd.tsx` - 组件实现。
- `AwakenKbd.css` - 组件局部样式入口。
- `AwakenKbd.states.ts` - 可观测状态与模拟 props。
- `AwakenKbd.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
