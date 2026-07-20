# AwakenSidebar

左侧栏的 UI-only 组合组件。Gallery 中只注册“「左侧栏」组件库”这一处入口：页面先展示 Awaken Desktop 当前完整组合，再集中展示当前组合中使用的独立小组件。每个公开小组件仍拥有自己的目录、API、states、Demo 和 README，但不单独占用 Gallery 侧栏入口。

## 用途

`AwakenSidebar` 用于组合桌面端左侧导航栏，包括品牌、新建任务、一级导航、对话列表、项目树和账户触发位置。父级负责提供各区数据、选中项、回调和是否显示分区；组件负责把这些子组件按侧边栏布局组装起来，不直接管理路由、会话加载、项目读取或账户面板内容。

## 当前 Desktop 组合

- `AwakenWorkbenchSidebarFrame + AwakenSidebar`：完整左侧栏组合与外层边界。
- `AwakenNewTaskButton`：`New Agent` 主入口。
- `AwakenPrimaryNav`：连接器与技能入口。
- `AwakenProjectTree`：Projects 项目与线程树。
- `AwakenSidebarSectionAction`：筛选、添加文件夹和项目行内动作。
- `AwakenSidebarThreadSpinner`：只在 running / cancelling 线程行末显示的运行指示。
- `AwakenProfileButton`：只作为 Nolan 账户入口在完整侧栏中出现；它与额度面板的完整组合归入独立“「账户」组件库”。

项目树同时展示当前真实支持的项目/会话更多菜单、展开与选中状态，以及“暂无项目”“暂无会话”两种空状态。`AwakenDropdown`、`AwakenTooltip`、`AwakenAvatar` 与 `AwakenIcon` 是多个产品区域共享的通用组件，因此继续保留自己的组件目录入口，只在左侧栏组合中按真实关系复用。

当前 Desktop 传入 `showBrand={false}`、空 conversations 和空 conversation section label，因此 `AwakenBrandRow` 与 `AwakenConversationList` 不在当前可见组合中；它们继续作为可选公开组件和独立 Gallery 入口保留。

## 文件

- `AwakenSidebar.tsx` - 组件实现。
- `AwakenSidebar.css` - 组件局部样式入口。
- `AwakenSidebar.demo.css` - “「左侧栏」组件库” Gallery 专用布局。
- `AwakenSidebar.states.ts` - 可观测状态与模拟 props。
- `AwakenSidebar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
