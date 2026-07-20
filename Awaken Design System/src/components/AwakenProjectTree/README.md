# AwakenProjectTree

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它集中在“「左侧栏」组件库” Gallery 入口中查看。

## 用途

`AwakenProjectTree` 用于展示工作区项目、会话分组或文件组的树形列表，支持展开、选中、加载中和禁用节点。父级负责提供树数据、当前选中项、数据加载和持久化展开状态；组件只管理或回传展开 id，并渲染节点层级与点击选择，不主动读取文件系统。

线程的 `isLoading` 使用独立的 `AwakenSidebarThreadSpinner`，对应 Desktop 中 `running | cancelling` run；项目和线程的 `menuItems` 继续由业务层传入。无项目显示“暂无项目”，空项目展开后显示“暂无会话”。

## 文件

- `AwakenProjectTree.tsx` - 组件实现。
- `AwakenProjectTree.css` - 组件局部样式入口。
- `AwakenProjectTree.states.ts` - 可观测状态与模拟 props。
- `AwakenProjectTree.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
