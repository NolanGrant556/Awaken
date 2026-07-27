# AwakenSidebar

左侧栏的 UI-only 组合组件。Gallery 中只注册“「左侧栏」组件库”这一处入口：页面先展示 Awaken Desktop 当前完整组合，再集中展示当前组合中使用的独立小组件。每个公开小组件仍拥有自己的目录、API、states 和 README；真实组合由组件族 Demo 统一观测，不再为内部小组件维护重复的顶层 Demo。

## 用途

`AwakenSidebar` 用于组合桌面端左侧导航栏，包括新建任务、一级导航、项目树、独立对话列表和账户触发位置。父级负责提供各区数据、选中项、回调和是否显示分区；组件负责把这些子组件按侧边栏布局组装起来，不直接管理路由、会话加载、项目读取或账户面板内容。

## 当前 Desktop 组合

- `AwakenWorkbenchSidebarFrame + AwakenSidebar`：完整左侧栏组合与外层边界。
- `AwakenNewTaskButton`：`New Agent` 主入口。
- `AwakenPrimaryNav`：连接器与技能入口。
- `AwakenProjectTree + AwakenDropdown`：“项目工作区”项目/线程树与“对话”扁平 thread 列表，以及项目/会话的真实更多操作菜单。
- 顶层项目通过按住拖拽或 `Alt + ↑/↓` 排序；`onReorderProjects` 只回传完整顺序，持久化仍由 consumer 负责。
- `AwakenSidebarSectionAction`：项目全部展开/收起、对话分区展开/收起、创建项目、新增对话和项目行内动作。
- `AwakenSidebarThreadSpinner`：只在 running / cancelling 线程行末显示的运行指示。
- `AwakenProfileButton`：只作为 Nolan 账户入口在完整侧栏中出现；它与额度面板的完整组合归入独立“「账户」组件库”。
- `AwakenDialog + AwakenIconButton`：保留统一模态与关闭按钮能力；Gallery 在本组件族中观测创建项目、移除项目和重命名会话三个 App 级真实流程。
- `AwakenSelectableCard`：只在创建项目弹窗中展示 Desktop 当前支持的本地文件夹来源，不展示尚未支持的远程或云端来源。
- `AwakenProjectFoldersEditor`：在创建项目和编辑项目两个弹窗中观测项目的文件夹列表、主文件夹徽标和每行更多操作；Gallery 只模拟回调，不打开系统对话框、不触碰磁盘。

项目树与对话列表同时展示当前真实支持的更多菜单、展开与选中状态，以及“暂无项目”“暂无会话”两种空状态。项目区和对话区位于同一个中部纵向滚动容器，顶部 New Agent/一级导航与底部账户固定；滚动条使用窄轨道。`AwakenDropdown` 保留自己的组件目录与公开导出，但不再单独占用 Gallery 入口；`AwakenTooltip` 随侧栏区块动作和其他真实消费组件观测，`AwakenAvatar` 仍保留独立入口。New Agent 加号、项目树箭头和账户入口箭头直接使用 `lucide-react`，不再经过重复的 `AwakenIcon` 包装层。

侧栏不保留品牌行；顶部直接从 `New Agent` 开始。带项目身份的 thread 位于“项目工作区”树下，空项目身份的 thread 位于独立“对话”分区。

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
- 原 `awaken-dialog`、`awaken-dropdown`、`awaken-icon` 与 `awaken-selectable-card` URL 均跳转到“「左侧栏」组件库”；技能、设置和连接器各自在自己的组件族中继续观测对应弹窗。
