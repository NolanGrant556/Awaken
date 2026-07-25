# AwakenProjectFoldersEditor

此目录包含一个独立的 Awaken 设计系统生产组件和可复用状态；它不再重复占用 Gallery 顶层入口。

## 用途

`AwakenProjectFoldersEditor` 用于「一个项目 = 一组文件夹 + 一个主文件夹」的项目编辑界面。它渲染有序的文件夹行和底部的「添加文件夹」行。创建项目和编辑项目两个 App 级弹窗复用同一个组件。

主文件夹徽标在本组件内实现，设计系统当前没有独立的 Badge 组件。

## 行内呈现

每行只显示文件夹名末段，不显示完整路径；完整路径挂在原生 `title` 上，用于区分同名文件夹。末段按 `/` 和 `\` 两种分隔符切分，Windows 路径不会退化成整条路径；父级传了 `label` 时以 `label` 为准。行尾控件按主文件夹与否分流：

- 主文件夹行显示「主文件夹」徽标；
- 其他行在悬停或键盘聚焦时露出「设为主文件夹」按钮，静默态保留占位以免整行控件跳动；
- 每行常显 `×`，语义是把文件夹移出这一组，不动磁盘。

徽标与「设为主文件夹」按钮共用同一条 `min-width`，两种行的 `×` 才不会左右错位。

## 文件

- `AwakenProjectFoldersEditor.tsx` - 组件实现。
- `AwakenProjectFoldersEditor.css` - 组件局部样式入口。
- `AwakenProjectFoldersEditor.states.ts` - 空态、单文件夹、创建态多文件夹、编辑态带额外菜单、Windows 反斜杠路径与单行提交中六个可复用状态。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用；选择目录、挂载、切换主文件夹和废纸篓都由父级回调完成。
- 通过 props 输入，通过回调输出：切主与移出用 `ProjectFolderRow.onMakePrimary` / `onRemove` 直接接线；`actions` 只承载这两者之外的额外操作，为空时不渲染 `⋯`。
- 破坏性操作不进 `×`。Desktop 的「移到废纸篓」真的动磁盘，只作为 `actions` 出现在编辑项目弹窗；创建项目弹窗的文件夹还没挂到项目上，不提供该项，因此那里没有 `⋯`。
- 哪些操作可用由父级决定：不传 `onMakePrimary` 的行不会露出切主按钮，不传 `onRemove` 的行没有 `×`。
- `busyPath` 只表达单行正在提交，不代表整体禁用。
- Gallery 在“「左侧栏」组件库”的创建项目与编辑项目弹窗中观测本组件。
