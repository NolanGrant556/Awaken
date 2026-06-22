# AwakenPopover

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenPopover` 用于把一个触发器和一段浮层内容组合成可点击打开、可 Esc/外部点击关闭的弹出面板。它适合承载模型选择、更多操作、账户菜单或轻量设置；父级负责提供具体触发器和内容，组件负责打开状态、定位、焦点管理和可选受控模式，不解释浮层里的业务动作。

## 文件

- `AwakenPopover.tsx` - 组件实现。
- `AwakenPopover.css` - 组件局部样式入口。
- `AwakenPopover.states.ts` - 可观测状态与模拟 props。
- `AwakenPopover.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
