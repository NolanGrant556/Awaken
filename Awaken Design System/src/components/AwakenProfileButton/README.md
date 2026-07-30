# AwakenProfileButton

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它集中在“「账户」组件库” Gallery 入口中查看。

## 用途

`AwakenProfileButton` 用于在侧边栏底部展示当前用户或本机配置身份，并作为账户/设置菜单的触发器。父级负责传入姓名、副标题、头像和点击后打开的菜单；组件通过 `expanded` 与 `controls` 关联 `AwakenProfileMenuPanel`，自身只负责身份行的头像、文本截断、箭头和交互反馈。

## 文件

- `AwakenProfileButton.tsx` - 组件实现。
- `AwakenProfileButton.css` - 组件局部样式入口。
- `AwakenProfileButton.states.ts` - 可观测状态与模拟 props。
- `AwakenProfileButton.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
