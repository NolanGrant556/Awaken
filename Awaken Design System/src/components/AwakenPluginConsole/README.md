# AwakenPluginConsole

插件详情与八类组件控制台。主页面只呈现插件名称、可选版本、插件级操作和来源中立的紧凑组件目录，不直接投影包内长 description；组件详情、配置和动作统一进入居中的 `AwakenDialog`。读取组件正文、保存变量、切换运行状态及卸载等产品行为由宿主注入。

敏感变量只显示配置状态。MCP `source` 是安装包内源文件的原始文本；宿主可通过 `sourceEditable` 与 source edit callbacks 提供受控编辑，但 Design System 不直接写文件。`recognized_unsupported`、`ignored_by_dialect` 和 `invalid` 组件不应由宿主标记为可切换。

## Props 契约

- identity：`name`、`displayName`、`version`；名称完整显示并可自然换行，不能使用 ellipsis 截断或裁切字形；厂商方言、来源、digest 和插件状态矩阵不属于常驻 UI；
- state：`enabled`、`components`、`variables`、`loading`、`busy`、`errorMessage`；
- dialog：`selectedDetail`、`detailLoading`、`activeDetailTab`，正文与 source 只在宿主按需读取后传入；
- source edit：`sourceEditing`、`sourceDraft` 与 `onSourceEditStart/onSourceDraftChange/onSourceEditCancel/onSourceSave` 只负责编辑态展示；校验、digest CAS 和原子保存由宿主/Core 拥有；
- mutations：所有 `on*` 回调只表达用户意图。组件不直接访问 Core、文件系统、Keychain 或网络，也不对 mutation 做乐观持久化。

`loading` 显示组件骨架，`detailLoading` 只加载独立 Dialog。宿主 mutation 完成后必须重读插件 inventory/detail，再把新状态传回组件。
