# AwakenDropdown

此目录包含一个独立的 Awaken 设计系统组件和可观测状态。它不单独占用 Gallery 目录入口，而是在“「左侧栏」组件库”的项目树真实更多菜单中观测。

## 用途

`AwakenDropdown` 用于把触发器和浮层内容组合成下拉容器。当前 Desktop 由 `AwakenProjectTree` 消费，用于项目和会话的更多操作菜单。父级负责传入触发器、菜单内容、受控 open 状态和选择逻辑；组件只提供基于 `AwakenPopover` 的浮层外壳、定位和基础菜单容器，不理解菜单项含义，也不替父级管理选中值。

## 文件

- `AwakenDropdown.tsx` - 组件实现。
- `AwakenDropdown.css` - 组件局部样式入口。
- `AwakenDropdown.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- 原 `awaken-dropdown` URL 跳转到“「左侧栏」组件库”；不要重新添加虚构的模型或视图菜单独立演示。
