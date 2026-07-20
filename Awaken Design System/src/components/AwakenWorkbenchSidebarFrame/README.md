# AwakenWorkbenchSidebarFrame

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它作为完整组合的一部分集中在“「左侧栏」组件库” Gallery 入口中查看。

## 用途

`AwakenWorkbenchSidebarFrame` 用于给 Workbench 侧栏内容提供统一外层框架，通常包裹 `AwakenSidebar` 或同等侧栏导航内容。父级负责传入具体导航、会话和项目列表，本组件只负责侧栏区域的框架边界，不负责列表数据、选中状态或折叠逻辑。

## 文件

- `AwakenWorkbenchSidebarFrame.tsx` - 组件实现。
- `AwakenWorkbenchSidebarFrame.css` - 组件局部样式入口。
- `AwakenWorkbenchSidebarFrame.states.ts` - 可观测状态与模拟 props。
- `AwakenWorkbenchSidebarFrame.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
