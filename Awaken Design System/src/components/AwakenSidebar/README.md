# AwakenSidebar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenSidebar` 用于组合桌面端左侧导航栏，包括品牌、新建任务、一级导航、对话列表、项目树和账号入口。父级负责提供各区数据、选中项、回调和是否显示分区；组件负责把这些子组件按侧边栏布局组装起来，不直接管理路由、会话加载或项目读取。

## 文件

- `AwakenSidebar.tsx` - 组件实现。
- `AwakenSidebar.css` - 组件局部样式入口。
- `AwakenSidebar.states.ts` - 可观测状态与模拟 props。
- `AwakenSidebar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
