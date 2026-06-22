# AwakenConversationList

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenConversationList` 用于在侧栏列出任务会话或运行记录，展示标题、时间标签、可选状态徽标、选中态和禁用态。它适合 Awaken 的会话导航区；父级负责加载、排序、过滤、activeId 和右键菜单行为，本组件只负责把 `ConversationItem[]` 渲染成可选择的行并触发选择/上下文菜单回调。

## 文件

- `AwakenConversationList.tsx` - 组件实现。
- `AwakenConversationList.css` - 组件局部样式入口。
- `AwakenConversationList.states.ts` - 可观测状态与模拟 props。
- `AwakenConversationList.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
