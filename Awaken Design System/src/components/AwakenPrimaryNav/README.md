# AwakenPrimaryNav

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它集中在“「左侧栏」组件库” Gallery 入口中查看。

## 用途

`AwakenPrimaryNav` 用于渲染侧边栏中的一级导航入口，例如任务、项目、运行或设置等主要区域。父级负责提供导航项、当前选中项和路由/视图切换逻辑；组件只负责列表按钮、徽章、禁用态和选择回调，不持有路由状态。

## 文件

- `AwakenPrimaryNav.tsx` - 组件实现。
- `AwakenPrimaryNav.css` - 组件局部样式入口。
- `AwakenPrimaryNav.states.ts` - 可观测状态与模拟 props。
- `AwakenPrimaryNav.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
