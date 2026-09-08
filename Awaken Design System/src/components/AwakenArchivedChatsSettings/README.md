# AwakenArchivedChatsSettings

Settings 固定末项「已归档的聊天」的页面组件（对应设计契约 `docs/01_桌面工作台/会话管理/实现方案.md` 相应章节–相应章节）。纯受控 UI-only：分组、行数据、日期显示串都由调用方传入，组件不读取 Runtime、Electron、FileStore 或项目 store；行为只通过 callbacks 传出。

## Props

- `groups: AwakenArchivedChatGroup[]`：已按当前搜索与项目筛选过滤后的可见分组。组含 `id`、`label`（项目名 / “对话” / “未知项目（ID 后 8 位）”）与 `rows`；行含 `threadId`、`title`、`archivedAtLabel`（locale 日期显示串）、`archivedAtTitle`（辅助完整日期时间）。空组不应传入。
- `query` / `onQueryChange`：搜索框受控值，placeholder 为“搜索已归档聊天”；匹配与大小写折叠由调用方负责。
- `projectFilterOptions` / `projectFilterValue` / `onProjectFilterChange`：项目筛选 `AwakenSelect`，第一项为“所有项目”。
- `filtersActive?: boolean`：搜索或筛选是否生效。`groups` 为空时据此区分空库文案“还没有已归档的聊天”与无结果文案“没有匹配的已归档聊天”（后者附“清除搜索和筛选条件”按钮，回调 `onClearFilters`）。
- `busyThreadIds?: readonly string[]`：进行中（取消归档/删除）的 thread id，对应行的两个操作按钮禁用。
- `loading?: boolean`：加载态（`role="status"`）。
- `errorMessage?: string | null` + `onRetry`：载入失败错误态（`role="alert"` + “重试”），不伪装成空库。
- `onUnarchive(threadId)`：主操作“取消归档”。
- `onDelete(threadId)`：危险操作垃圾桶按钮；只暴露删除意图，统一确认 Dialog 由 Desktop 侧负责。

## 结构

标题区由外层 Settings 框架提供，组件不含页头。顶部为搜索框 + 项目筛选下拉；下方按组渲染组标题与行。每行显示截断标题（`title` 辅助完整名）、归档日期（`title` 辅助完整日期时间）、“取消归档”按钮与危险色删除按钮。

取消归档的成功或失败反馈不属于本组件的列表布局；Desktop consumer 接收 `onUnarchive` 意图后，把结果交给 `AwakenSettings` 的顶部 Toast。组件不得在列表末尾追加 notice 段落。

Gallery 入口是 `#/components/awaken-archived-chats-settings`，states 覆盖多项目分组、独立“对话”组、未知项目组、长标题、busy 行、空库、无结果、加载与错误。
