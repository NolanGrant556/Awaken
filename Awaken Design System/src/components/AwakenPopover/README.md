# AwakenPopover

此目录包含一个独立的 Awaken 设计系统生产组件和可复用状态；它不再重复占用 Gallery 顶层入口。

## 用途

`AwakenPopover` 用于把一个触发器和一段浮层内容组合成可点击打开、可 Esc/外部点击关闭的弹出面板。当前真实消费包括账户额度面板，以及由 `AwakenDropdown` 承载的项目/会话更多菜单；父级负责提供具体触发器和内容，组件负责打开状态、定位、焦点管理和可选受控模式，不解释浮层里的业务动作。

## 文件

- `AwakenPopover.tsx` - 组件实现。
- `AwakenPopover.css` - 组件局部样式入口。
- `AwakenPopover.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- Gallery 在“「账户」组件库”和“「左侧栏」组件库”的真实组合中观测本组件；原 `awaken-popover` URL 跳转到“「账户」组件库”。
