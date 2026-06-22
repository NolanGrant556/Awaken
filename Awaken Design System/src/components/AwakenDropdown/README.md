# AwakenDropdown

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenDropdown` 用于把触发器和浮层内容组合成下拉容器，常用于模型选择、视图切换或轻量操作菜单。父级负责传入触发器、菜单内容、受控 open 状态和选择逻辑；组件只提供基于 `AwakenPopover` 的浮层外壳、定位和基础菜单容器，不理解菜单项含义，也不替父级管理选中值。

## 文件

- `AwakenDropdown.tsx` - 组件实现。
- `AwakenDropdown.css` - 组件局部样式入口。
- `AwakenDropdown.states.ts` - 可观测状态与模拟 props。
- `AwakenDropdown.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
