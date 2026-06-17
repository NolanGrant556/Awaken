# Awaken UI Kit — 组件库建设历史档案

> 版本：归档 v1.2
> 状态：Phase 1～6 已完成并推送，最终实现提交 `068296d feat(ui-kit): Phase 5 layout/platform and Phase 6 full page assembly`
> 原始目标：作为 Coding Agent 的实现依据，将 `frontend/demos/awaken-interactive-demo.html` 拆解为一套 UI-only、可复用、可发布的 React 组件库。
> 设计源：`frontend/demos/awaken-interactive-demo.html`（唯一视觉与交互标准）
> 旧组件引用：`frontend/src/components/`（现状参考，不作为视觉标准）

## 0. 归档说明

本文档最初是 Awaken UI Kit 的生产级实现规格，用于指导 Coding Agent 分阶段完成组件库重建。归档时，Phase 1～6 均已实现、review、修正并推送到 `origin/main`。

本文档保留原规格、组件契约、验收标准和分阶段计划，用作后续追溯：

- 为什么从 HTML 交互 demo 拆组件，而不是沿用旧 React 样式。
- 为什么组件库坚持 UI-only、props/callbacks、零后端依赖。
- 每个组件类别的职责边界、数据契约和跨平台预留点。
- Phase 1～6 的实际建设路径、验收方式和已关闭风险。

后续若继续演进组件库，应基于当前实现另开新规格或追加变更记录，不再把本文档当作“下一轮唯一实现指令”。

---

## 1. 组件库定位

- **UI-only React component library**。只负责渲染与交互，不承载业务逻辑，不调用任何 Agent runtime。
- **零后端依赖**。不依赖 Agent runtime、不调用后端 API、不读取 Electron IPC。组件本身不引入 `window.awaken`、`require('electron')` 或 `fetch`。
- **纯 props / callbacks / controlled state 接入业务**。所有数据通过 props 传入，所有事件通过 callbacks 传出。
- **HTML demo 是唯一视觉与交互标准**。旧 React 组件（`frontend/src/components/*`）曾使用旧样式（`styles.css` 绿色渐变主题），本轮实现仅将其作为现状参考，最终以 HTML demo 的浅灰中性设计为准。
- **桌面产品优先**。当前面向 Electron 桌面端，最小窗口宽度 `920px`，不考虑移动端网页适配。macOS / Windows titlebar 通过 `platform` prop 预留适配点。
- **向可发布库靠拢**。当前阶段仍在 `frontend` 内部开发，但目录结构、导出方式、类型定义按可发布 npm 包设计。

---

## 2. 设计源拆解

### 2.1 关键布局（来自 HTML demo）

| 区域 | 规格 |
|---|---|
| AppShell | `grid-template-columns: var(--sidebar-width) minmax(0, 1fr)`，全屏 fixed |
| Sidebar | `width: 300px`（小屏 `280px`），`grid-template-rows: auto auto minmax(0, 1fr) auto`，背景 `#f2f2f2` |
| Main | `grid-template-rows: 56px minmax(0, 1fr)`，背景 `#f8f8f7` |
| TopBar | `height: 56px`，`padding: 12px 20px 12px 14px`，sticky top |
| CenterStage | `width: 768px`（小屏 `720px`），`max-width: calc(100vw - var(--sidebar-width) - 120px)`，`margin-top: 96px`（矮屏 `72px`） |
| Composer | 最小高度 `116px`，`padding: 12px 16px`，圆角 `16px`，背景 `rgba(55,53,47,0.04)` |
| RunPanel | `margin-top: 38px`，`padding-bottom: 56px`，默认隐藏，显示时 `panel-in` 动画 |
| ModelMenu | `width: 220px`，fixed 定位，基于 trigger 右下对齐，边界防溢出 |
| Toast | `max-width: 320px`，fixed `right: 24px; bottom: 24px` |

### 2.2 视觉 Token（来自 HTML demo，转义为语义 token）

所有组件禁止写散落硬编码颜色与尺寸，必须使用以下 token。

#### 颜色

| 语义 token | 值 | 用途 |
|---|---|---|
| `--aw-bg-main` | `#f8f8f7` | 主内容区背景 |
| `--aw-bg-nav` | `#f2f2f2` | 侧边栏背景 |
| `--aw-bg-white` | `#ffffff` | 卡片、菜单、弹窗背景 |
| `--aw-bg-fill-light` | `rgba(55,53,47,0.04)` | composer 背景、hover 浅底 |
| `--aw-bg-fill-main` | `rgba(55,53,47,0.06)` | 按钮 hover、nav active |
| `--aw-bg-fill-dark` | `rgba(55,53,47,0.08)` | send-button 默认背景 |
| `--aw-text-primary` | `#34322d` | 主标题、正文 |
| `--aw-text-secondary` | `#5e5e5b` | 副标题、图标默认色 |
| `--aw-text-tertiary` | `#858481` | 时间、meta、占位 |
| `--aw-text-disabled` | `#b9b9b7` | placeholder、禁用 |
| `--aw-text-blue` | `#0081f2` | 运行中 step mark |
| `--aw-text-onblack` | `#ffffff` | 黑底上的字 |
| `--aw-border-main` | `rgba(0,0,0,0.06)` | 主边框、divider |
| `--aw-border-light` | `rgba(0,0,0,0.04)` | 极浅边框、composer 边框 |
| `--aw-border-active` | `rgba(0,0,0,0.3)` | focus-visible outline |
| `--aw-button-black` | `#1a1a19` | send-button ready 状态 |
| `--aw-success` | `#25ba3b` | 成功态 |
| `--aw-warning` | `#efa201` | 警告态 |
| `--aw-error` | `#f25a5a` | 错误态 |

#### 尺寸与间距

| 语义 token | 值 |
|---|---|
| `--aw-space-1` | `2px` |
| `--aw-space-2` | `4px` |
| `--aw-space-3` | `8px` |
| `--aw-space-4` | `10px` |
| `--aw-space-5` | `12px` |
| `--aw-space-6` | `14px` |
| `--aw-space-7` | `16px` |
| `--aw-space-8` | `20px` |
| `--aw-space-9` | `24px` |
| `--aw-space-10` | `28px` |
| `--aw-space-11` | `32px` |
| `--aw-space-12` | `38px` |
| `--aw-space-13` | `40px` |
| `--aw-space-14` | `48px` |
| `--aw-space-15` | `56px` |
| `--aw-space-16` | `96px` |

#### 圆角

| 语义 token | 值 |
|---|---|
| `--aw-radius-sm` | `8px` |
| `--aw-radius-md` | `12px` |
| `--aw-radius-lg` | `16px` |
| `--aw-radius-pill` | `999px` |

#### 阴影

| 语义 token | 值 |
|---|---|
| `--aw-shadow-menu` | `0 16px 42px rgba(0,0,0,0.12)` |
| `--aw-shadow-toast` | `0 14px 34px rgba(0,0,0,0.12)` |

#### 字体

| 语义 token | 值 |
|---|---|
| `--aw-font-sans` | `-apple-system, BlinkMacSystemFont, "Segoe UI Variable Display", "Segoe UI", Helvetica, "Apple Color Emoji", Arial, sans-serif, "Segoe UI Emoji", "Segoe UI Symbol"` |
| `--aw-font-mono` | `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace` |

#### 字号与行高（来自 demo 实际使用值）

| 语义 token | size / line-height / weight | 用途 |
|---|---|---|
| `--aw-text-xs` | `12px / 16px / 400` | meta、时间、badge |
| `--aw-text-sm` | `13px / 18px / 400~560` | 列表标题、step 主标题 |
| `--aw-text-base` | `14px / 20px / 400~650` | nav、按钮、run-head |
| `--aw-text-md` | `15px / 22px / 400` | quick-chip、composer textarea |
| `--aw-text-lg` | `15px / 24px / 400` | composer textarea（实际行高） |
| `--aw-text-xl` | `30px / 40px / 620` | hero-title |

### 2.3 关键交互（来自 HTML demo）

| 交互 | 行为 | 组件化归属 |
|---|---|---|
| 输入 ready 状态 | textarea 有内容时 send-button 变黑 | `Composer` + `SendButton` |
| Cmd/Ctrl+Enter 运行 | 快捷键触发 onSubmit | `ComposerInput` 监听，callback 传出 |
| 模型菜单开关 | 点击 model-pill 弹出，点击选项或 Escape 关闭 | `ModelSelector` + `Dropdown` |
| Plan/Act 切换 | 点击 mode-pill 切换文本 | `ModeToggle` |
| 快捷任务填充 | 点击 quick-chip 将文本填入 composer | `QuickActionChip` onClick 传出文本 |
| Toast | 固定位置，自动 1.5s 消失 | `Toast` / `ToastProvider` |
| 运行时间线推进 | step 依次进入 active -> done | `RunTimeline` + `RunStep`，数据驱动 |
| 新建任务 | 清空输入、关闭 run panel、聚焦输入 | `NewTaskButton` onClick callback |
| 项目树展开/折叠 | details/summary 行为 | `ProjectTree` 内部状态或 controlled |

### 2.4 Demo Mock 行为 vs 组件化行为

以下行为是 demo 为了演示效果写的 mock，**不应固化到组件内部**：

- `runDemo()` 的定时推进逻辑（`setTimeout(advance, 760)`）—— 应由业务层通过更新 `RunStepItem[]` 的 `status` 驱动。
- `showToast()` 的定时隐藏 —— 应由 `Toast` 或业务层管理生命周期。
- `syncSendState()` 直接读取 DOM —— 应由 `Composer` 的 `value` prop 计算后传给 `SendButton`。
- `positionModelMenu()` 的 window resize 重算 —— 应由 `Dropdown` 内部使用 Popover 逻辑自动处理。

---

## 3. 目录结构定稿

```
frontend/src/
├── components/
│   ├── primitives/          # 原子组件，无业务语义
│   ├── inputs/              # 输入类
│   ├── selectors/           # 选择器类
│   ├── navigation/          # 导航与侧边栏
│   ├── layout/              # 布局框架
│   ├── content/             # 内容展示
│   ├── feedback/            # 运行反馈与提示
│   ├── overlays/            # 浮层基础能力
│   ├── platform/            # 跨平台适配
│   ├── assets/              # 内嵌 logo、图标等静态资源
│   └── index.ts             # 统一导出
├── styles/
│   ├── tokens.css           # 所有 CSS 变量（第 7 节）
│   └── components.css       # 组件级公共样式（如 panel-in 动画、滚动条隐藏）
├── types.ts                 # 全局 UI 类型定义（第 5 节）
└── App.tsx                  # 调试/预览入口（非组件库一部分）
```

**说明**：
- `overlays/` 不直接对应 demo 的某个区域，而是提供 `Popover` / `Menu` 基础定位能力，供 `ModelSelector` 使用。避免每个组件各自实现 portal 与边界检测。
- `platform/` 预留 macOS / Windows 的 titlebar、keyboard modifier、window chrome 适配，当前至少实现 `PlatformProvider` 或 `platform` prop 透传。
- `assets/` 存放内嵌 logo（SVG/PNG），通过 `import` 引入，确保打包时随组件库输出。

---

## 4. 组件生产级说明

### 4.1 Primitives

#### Icon
- **职责**：SVG 图标统一包装，统一 `18x18` 视口、stroke 样式。
- **文件路径**：`primitives/Icon.tsx`
- **Props**：
  - `size?: number` — 默认 18
  - `strokeWidth?: number` — 默认 1.85
  - `children: ReactNode` — svg path/circle 等内容
- **状态**：无交互状态。
- **可访问性**：`aria-hidden="true"` 由调用方决定；若图标单独作为按钮内容，按钮负责 `aria-label`。
- **来源**：HTML demo `.icon` 类。
- **禁止**：禁止内嵌网络图片；禁止依赖 `lucide-react` 以外的图标库（当前项目已用 lucide，允许继续使用，但 Icon 组件本身保持包装层）。

#### IconButton
- **职责**：方形/圆形图标按钮。
- **文件路径**：`primitives/IconButton.tsx`
- **Props**：
  - `label: string` — 用于 `aria-label` 与 `title`
  - `variant?: 'square' | 'circle'` — 默认 square
  - `size?: 'sm' | 'md'` — sm=32px, md=34px（md 用于现有组件兼容）
  - `children: ReactNode`
  - 标准 button props 透传
- **Callback**：`onClick?: () => void`
- **必须支持的状态**：default, hover, active, focus-visible, disabled
- **可访问性**：必须有 `aria-label`；focus-visible 显示 `--aw-border-active` outline，offset 2px。
- **来源**：HTML demo `.square-button`、`.icon-button`。
- **禁止**：禁止内部写死边框颜色；禁止直接读取 Electron API。

#### Button
- **职责**：文本按钮，支持图标。
- **文件路径**：`primitives/Button.tsx`
- **Props**：
  - `variant?: 'default' | 'primary' | 'ghost'` — primary 对应 send-button ready 态样式
  - `size?: 'sm' | 'md' | 'lg'`
  - `icon?: ReactNode`
  - `children: ReactNode`
  - 标准 button props 透传
- **Callback**：`onClick?: () => void`
- **必须支持的状态**：default, hover, active, focus-visible, disabled, loading
- **来源**：HTML demo `.new-task`、`.toolbar-pill`、`.top-pill`。
- **禁止**：禁止在组件内部判断发送条件（如输入是否为空）。

#### Pill
- **职责**：圆角标签，可带图标，用于模型选择、模式切换等。
- **文件路径**：`primitives/Pill.tsx`
- **Props**：
  - `variant?: 'default' | 'bordered'` — bordered 带 `1px solid --aw-border-main`
  - `size?: 'sm' | 'md'` — sm=32px(h), md=40px(h)
  - `icon?: ReactNode`
  - `children: ReactNode`
  - `onClick?: () => void`
  - `active?: boolean`
  - `disabled?: boolean`
- **必须支持的状态**：default, hover, active, focus-visible, disabled
- **来源**：HTML demo `.top-pill`、`.model-pill`、`.toolbar-pill`、`.quick-chip`。

#### StatusPill
- **职责**：状态标签，带颜色语义。
- **文件路径**：`primitives/StatusPill.tsx`
- **Props**：
  - `tone: 'local' | 'permission' | 'neutral' | 'success' | 'warning' | 'error'`
  - `icon?: ReactNode`
  - `children: ReactNode`
- **必须支持的状态**：各 tone 对应背景、边框、文字色。
- **来源**：现有组件 `UiPrimitives.tsx` 中的 `StatusPill`，视觉按 HTML demo 的配色重新调整。
- **禁止**：禁止绑定后端状态字段名（如 `isLocal`），只接收 `tone` 枚举。

#### Avatar
- **职责**：用户头像，支持文字 fallback。
- **文件路径**：`primitives/Avatar.tsx`
- **Props**：
  - `src?: string` — 图片 URL
  - `fallback: string` — 1~2 个大写字母
  - `size?: 'sm' | 'md'` — sm=28px, md=32px
- **必须支持的状态**：empty（无 src 时显示 fallback）
- **来源**：HTML demo `.avatar`。

#### Divider
- **职责**：水平分割线。
- **文件路径**：`primitives/Divider.tsx`
- **Props**：
  - `spacing?: 'sm' | 'md' | 'lg'` — 上下 margin
- **来源**：HTML demo step 之间的 `border-top`。

#### Kbd
- **职责**：键盘快捷键标签，如 `⌘N`。
- **文件路径**：`primitives/Kbd.tsx`
- **Props**：
  - `children: ReactNode`
  - `platform?: PlatformKind` — 决定显示 `⌘` 或 `Ctrl`
- **来源**：HTML demo `.nav-badge` 中的快捷键文本。

---

### 4.2 Inputs

#### ComposerInput
- **职责**：多行任务输入框，受控组件，支持 auto-resize。
- **文件路径**：`inputs/ComposerInput.tsx`
- **Props**：
  - `value: string`
  - `onChange: (value: string) => void`
  - `onSubmit?: () => void` — 由 Cmd/Ctrl+Enter 触发
  - `placeholder?: string`
  - `minRows?: number` — 默认 2（对应 demo 约 50px 高度）
  - `maxRows?: number` — 默认 6
  - `disabled?: boolean`
- **Callback**：`onChange`, `onSubmit`
- **必须支持的状态**：default, focus, disabled, empty, error（边框变红）, long-text（自动增高）
- **可访问性**：`aria-label="输入任务"`；focus 时 textarea 本身无 outline（由外层 composer-box 控制或保留默认 focus-visible）。
- **来源**：HTML demo `.composer textarea`。
- **禁止**：禁止内部直接调用运行逻辑；禁止读取 `window.awaken`。

#### Composer
- **职责**：完整输入卡片，组合输入框、工具栏、模型选择、上下文按钮。
- **文件路径**：`inputs/Composer.tsx`
- **Props**：
  - `value: string`
  - `onChange: (value: string) => void`
  - `onSubmit?: () => void`
  - `onAddContext?: () => void`
  - `mode: 'Plan' | 'Act'`
  - `onModeChange?: (mode: 'Plan' | 'Act') => void`
  - `model: ModelOption`
  - `models: ModelOption[]`
  - `onModelChange?: (model: ModelOption) => void`
  - `placeholder?: string`
  - `disabled?: boolean`
  - `isReady?: boolean` — 控制 send-button 视觉状态（由业务层根据 value 计算）
- **Callback**：`onChange`, `onSubmit`, `onAddContext`, `onModeChange`, `onModelChange`
- **必须支持的状态**：default, focus（输入框聚焦时 composer-box 可能有视觉变化）, disabled, empty, ready（send-button 变黑）
- **来源**：HTML demo `.composer`、`.composer-box`、`.composer-toolbar`。
- **禁止**：禁止内部硬编码模型列表；禁止内部判断 `isReady` 逻辑（必须由 props 传入）。

#### ContextButton
- **职责**：添加上下文按钮（+）。
- **文件路径**：`inputs/ContextButton.tsx`
- **Props**：继承 `IconButton`，固定 `label="添加上下文"`、图标为 Plus。
- **Callback**：`onClick?: () => void`
- **来源**：HTML demo `#addButton`。

#### SendButton
- **职责**：发送/运行按钮，圆形，带向上箭头图标。
- **文件路径**：`inputs/SendButton.tsx`
- **Props**：
  - `ready: boolean` — 控制黑底白字 vs 灰底白字
  - `onClick?: () => void`
  - `disabled?: boolean`
- **必须支持的状态**：default, ready, hover（ready 时 `translateY(-1px)`）, active, focus-visible, disabled, loading（可扩展旋转图标）
- **来源**：HTML demo `.send-button`。
- **禁止**：禁止内部读取输入框内容来决定 ready 状态。

---

### 4.3 Selectors

#### Dropdown
- **职责**：通用下拉菜单浮层。基于 `overlays/Popover` 实现。
- **文件路径**：`selectors/Dropdown.tsx`
- **Props**：
  - `trigger: ReactNode`
  - `children: ReactNode` — 菜单内容
  - `open?: boolean`
  - `onOpenChange?: (open: boolean) => void`
  - `placement?: 'bottom-start' | 'bottom-end'` — 默认 bottom-end（model menu 右对齐）
  - `offset?: number` — 默认 6px
- **必须支持的状态**：open, closed, empty（无 children 时不渲染）
- **可访问性**：Escape 关闭；点击外部关闭；焦点进入菜单后支持上下箭头导航（如菜单项为 button，浏览器默认支持 Tab）。
- **来源**：HTML demo `.model-menu` 的通用抽象。
- **禁止**：禁止直接操作 `document.body` 样式；禁止直接绑定 window resize（由 Popover 内部处理）。

#### ModelSelector
- **职责**：模型下拉选择器。
- **文件路径**：`selectors/ModelSelector.tsx`
- **Props**：
  - `value: ModelOption`
  - `options: ModelOption[]`
  - `onChange?: (option: ModelOption) => void`
  - `disabled?: boolean`
- **Callback**：`onChange`
- **必须支持的状态**：default, open, disabled, empty（options 为空时显示占位）
- **来源**：HTML demo `.model-pill` + `.model-menu`。
- **禁止**：禁止内部硬编码模型列表。

#### ModeToggle
- **职责**：Plan / Act 模式切换按钮。
- **文件路径**：`selectors/ModeToggle.tsx`
- **Props**：
  - `mode: 'Plan' | 'Act'`
  - `onChange?: (mode: 'Plan' | 'Act') => void`
  - `disabled?: boolean`
- **Callback**：`onChange`
- **必须支持的状态**：default, hover, active, focus-visible, disabled
- **来源**：HTML demo `#modeButton`。

---

### 4.4 Navigation

#### BrandRow
- **职责**：侧栏品牌区，显示 logo 与名称。
- **文件路径**：`navigation/BrandRow.tsx`
- **Props**：
  - `logoSrc?: string` — 覆盖默认内嵌 logo
  - `title?: string` — 默认 "Awaken"
  - `meta?: string` — 副标题，可选
- **必须支持的状态**：default, loading（logo 加载中可显示占位）
- **来源**：HTML demo `.brand-row`。
- **禁止**：禁止从网络加载 logo；默认使用 `assets/logo.svg`（或现有 `logo.png` 转内嵌）。

#### NewTaskButton
- **职责**：新建任务按钮。
- **文件路径**：`navigation/NewTaskButton.tsx`
- **Props**：
  - `onClick?: () => void`
  - `shortcut?: string` — 如 "N"，由组件根据 platform 渲染为 `⌘N` 或 `Ctrl+N`
  - `platform?: PlatformKind`
- **Callback**：`onClick`
- **必须支持的状态**：default, hover, active, focus-visible, disabled
- **来源**：HTML demo `.new-task`。

#### PrimaryNav
- **职责**：侧栏主导航列表。
- **文件路径**：`navigation/PrimaryNav.tsx`
- **Props**：
  - `items: NavItem[]`
  - `activeId?: string`
  - `onSelect?: (id: string) => void`
- **Callback**：`onSelect`
- **必须支持的状态**：default, hover, active, focus-visible
- **来源**：HTML demo `.nav-list` 中的前三个按钮（WorkFlow、插件 & Skills、定时任务）。

#### ConversationList
- **职责**：对话列表。
- **文件路径**：`navigation/ConversationList.tsx`
- **Props**：
  - `items: ConversationItem[]`
  - `activeId?: string`
  - `onSelect?: (id: string) => void`
  - `onContextMenu?: (id: string, event: React.MouseEvent) => void`
- **Callback**：`onSelect`, `onContextMenu`
- **必须支持的状态**：default, hover, active, empty（items 为空时显示占位文案）, long-text（溢出截断）
- **来源**：HTML demo `.task-list`。

#### ProjectTree
- **职责**：项目树，支持展开/折叠、选择、loading 状态。
- **文件路径**：`navigation/ProjectTree.tsx`
- **Props**：
  - `items: ProjectItem[]`
  - `activeId?: string`
  - `expandedIds?: string[]` — **受控**：当前展开的节点 ID 数组
  - `defaultExpandedIds?: string[]` — **非受控**：初始展开的节点 ID 数组
  - `onExpandedChange?: (ids: string[]) => void` — 展开状态变化 callback（受控时必须）
  - `onSelect?: (id: string) => void`
- **Callback**：`onSelect`, `onExpandedChange`
- **必须支持的状态**：default, hover, active, selected, loading（`isLoading` 时显示旋转图标）, empty, collapsed, expanded, long-text
- **展开/折叠状态规则**：
  - **受控模式**：传入 `expandedIds`，展开变化通过 `onExpandedChange(ids: string[])` 回调，调用方负责更新 `expandedIds`。这是推荐模式。
  - **非受控模式**：只传 `defaultExpandedIds`，组件内部自行管理状态，适用于无需外部同步的场景。
  - **禁止混用**：不要同时传入 `expandedIds` 和 `ProjectItem.expanded`；节点数据中不应保存展开状态。
- **来源**：HTML demo `.project-list`。
- **禁止**：禁止内部直接调用文件系统 API。

#### ProfileButton
- **职责**：侧栏底部用户资料按钮。
- **文件路径**：`navigation/ProfileButton.tsx`
- **Props**：
  - `name: string`
  - `avatarFallback: string`
  - `avatarSrc?: string`
  - `onClick?: () => void`
- **Callback**：`onClick`
- **必须支持的状态**：default, hover, active, focus-visible
- **来源**：HTML demo `.profile-button`。

#### Sidebar
- **职责**：完整侧边栏，内部组合 BrandRow、NewTaskButton、PrimaryNav、ConversationList、ProjectTree、ProfileButton。
- **文件路径**：`navigation/Sidebar.tsx`
- **Props**（data-driven，全部可选）：
  - `title?: string` — 品牌名称，默认 "Awaken"
  - `navItems?: NavItem[]` — 主导航列表
  - `activeNavId?: string` — 当前激活的导航项
  - `conversations?: ConversationItem[]` — 对话列表
  - `activeConversationId?: string` — 当前对话 ID
  - `projects?: ProjectItem[]` — 项目树数据
  - `activeProjectId?: string` — 当前选中项目 ID
  - `expandedIds?: string[]` — 受控展开状态（节点 ID 数组）
  - `defaultExpandedIds?: string[]` — 默认展开节点 ID 数组（非受控）
  - `profileName?: string` — 底部用户名
  - `profileAvatarFallback?: string` — 头像 fallback 字符
  - `profileAvatarSrc?: string` — 头像图片 URL
  - `newTaskShortcut?: string` — 新建任务快捷键文本（如 "⌘N"），由 Kbd 渲染
- **Callbacks**：
  - `onSelectNav?: (id: string) => void`
  - `onSelectConversation?: (id: string) => void`
  - `onSelectProject?: (id: string) => void`
  - `onNewTask?: () => void`
  - `onExpandedChange?: (ids: string[]) => void` — 展开状态变化
  - `onProfileClick?: () => void`
- **说明**：
  - 采用纯 data-driven 模式，不暴露子组件 slot。业务层需要完全自定义时，可直接组合子组件（BrandRow / PrimaryNav / ConversationList / ProjectTree / ProfileButton）。
  - 内部管理滚动区域、overflow hidden。
- **必须支持的状态**：overflow（内容超出时滚动，滚动条隐藏）
- **来源**：HTML demo `.sidebar`。

---

### 4.5 Layout

#### AppShell
- **职责**：应用外壳，包含 sidebar 与 main 区域。
- **文件路径**：`layout/AppShell.tsx`
- **Props**：
  - `sidebar: ReactNode`
  - `main: ReactNode`
  - `platform?: PlatformKind` — 影响 titlebar 样式
- **必须支持的状态**：small-window（窗口宽度 <= 1180px 时 sidebar 收缩为 280px，由 CSS media query 处理，组件本身不监听 resize）
- **来源**：HTML demo `.app-shell`。

#### TopBar
- **职责**：主内容区顶部操作栏。
- **文件路径**：`layout/TopBar.tsx`
- **Props**：
  - `children?: ReactNode` — 右侧操作区内容
- **来源**：HTML demo `.topbar`。

#### WindowTitleBar
- **职责**：自定义标题栏，用于 Windows 无边框窗口或 macOS 的 traffic light 区域占位。
- **文件路径**：`layout/WindowTitleBar.tsx`
- **Props**：
  - `platform: PlatformKind`
  - `onMinimize?: () => void`
  - `onMaximize?: () => void`
  - `onClose?: () => void`
- **必须支持的状态**：macOS（显示 traffic light 占位）、Windows（显示最小化/最大化/关闭按钮）
- **来源**：现有 `styles.css` `.traffic-row`。
- **禁止**：禁止直接调用 Electron `window.close()`；必须通过 callbacks 传出。

#### CenterStage
- **职责**：主内容区居中容器。
- **文件路径**：`layout/CenterStage.tsx`
- **Props**：
  - `children: ReactNode`
  - `width?: number` — 默认 768
  - `topMargin?: number` — 默认 96
- **来源**：HTML demo `.center-stage`。

---

### 4.6 Content

#### Hero
- **职责**：主区欢迎标题。
- **文件路径**：`content/Hero.tsx`
- **Props**：
  - `title: string`
  - `subtitle?: string`
- **来源**：HTML demo `.hero-title`。

#### QuickActionGrid
- **职责**：快捷任务 chips 网格容器。
- **文件路径**：`content/QuickActionGrid.tsx`
- **Props**：
  - `children: ReactNode`
- **来源**：HTML demo `.quick-actions`。

#### QuickActionChip
- **职责**：单个快捷任务 chip。
- **文件路径**：`content/QuickActionChip.tsx`
- **Props**：
  - `icon?: ReactNode`
  - `label: string`
  - `prompt: string` — 点击后填充到 composer 的文本
  - `onClick?: (prompt: string) => void`
  - `disabled?: boolean`
- **Callback**：`onClick(prompt)`
- **必须支持的状态**：default, hover, active, focus-visible, disabled
- **来源**：HTML demo `.quick-chip`。
- **禁止**：禁止内部直接操作 composer 的 DOM 或 state；必须通过 callback 传出 prompt。

---

### 4.7 Feedback

#### RunTimeline
- **职责**：Agent 运行步骤时间线容器。
- **文件路径**：`feedback/RunTimeline.tsx`
- **Props**：
  - `title: string`
  - `status: 'ready' | 'running' | 'done' | 'error'`
  - `steps: RunStepItem[]`
  - `children?: ReactNode` — 底部附加内容，如 ArtifactGrid
- **必须支持的状态**：ready（隐藏或占位）, running（panel-in 动画进入）, done, error
- **来源**：HTML demo `.run-panel`、`.timeline`。
- **禁止**：禁止内部写死步骤推进定时器；必须由 props 驱动。

#### RunStep
- **职责**：单个运行步骤。
- **文件路径**：`feedback/RunStep.tsx`
- **Props**：
  - `index: number` — 显示序号
  - `title: string`
  - `description?: string`
  - `tag?: string` — 右侧 code 标签，如 "scope"
  - `status: 'pending' | 'active' | 'done' | 'error'`
- **必须支持的状态**：pending（默认）, active（蓝色脉冲动画）, done（绿色勾选）, error（红色）
- **来源**：HTML demo `.step`。

#### ArtifactGrid
- **职责**：产物卡片网格。
- **文件路径**：`feedback/ArtifactGrid.tsx`
- **Props**：
  - `items: ArtifactItem[]`
- **来源**：HTML demo `.artifact-row`。

#### ArtifactCard
- **职责**：单个产物卡片。
- **文件路径**：`feedback/ArtifactCard.tsx`
- **Props**：
  - `title: string`
  - `description?: string`
  - `onClick?: () => void`
- **必须支持的状态**：default, hover, active, focus-visible
- **来源**：HTML demo `.artifact`。

#### ApprovalCard
- **职责**：本地 Agent 权限确认弹窗/气泡。在执行高风险操作（如写文件、执行命令、访问特定路径）前请求用户确认。
- **文件路径**：`feedback/ApprovalCard.tsx`
- **Props**：
  - `title: string` — 操作名称，如 "执行 shell 命令"
  - `description?: string` — 操作说明，如 "即将执行：rm -rf /tmp/awaken-*"
  - `riskTone?: 'safe' | 'warning' | 'danger'` — 风险色调，对应绿色/橙色/红色边框和图标色
  - `approveLabel?: string` — 确认按钮文字，默认 "允许"
  - `rejectLabel?: string` — 拒绝按钮文字，默认 "拒绝"
  - `onApprove?: () => void` — 用户确认 callback
  - `onReject?: () => void` — 用户拒绝 callback
  - `open?: boolean` — 受控显示状态
  - `onOpenChange?: (open: boolean) => void`
- **Callback**：`onApprove`, `onReject`
- **必须支持的状态**：open/closed；riskTone 决定边框色、图标、approve 按钮色
- **可访问性**：`role="alertdialog"` 或 `role="dialog"`；Escape 触发 onReject
- **说明**：
  - 不依赖后端，仅通过 props 传入数据和 callbacks 传出事件。
  - 底层可由 `Popover` 实现（作为确认弹窗而非下拉菜单），或作为独立 overlay。
  - 风格与 HTML demo 一致：白底、圆角、图标、左右双按钮。
- **来源**：Awaken 作为 Agent 工作台的核心安全交互，HTML demo 中无直接对应，需按 demo 整体风格设计。

#### Toast
- **职责**：轻量提示 toast。
- **文件路径**：`feedback/Toast.tsx`
- **Props**：
  - `message: string`
  - `visible: boolean`
  - `duration?: number` — 默认 1500ms
  - `onClose?: () => void`
- **必须支持的状态**：visible（panel-in 动画）, hidden
- **可访问性**：`role="status"` 或 `role="alert"`；建议配合 `ToastProvider` 管理堆叠与自动关闭。
- **来源**：HTML demo `.toast`。
- **禁止**：禁止内部直接操作全局状态；必须由 props 控制 visible。

---

### 4.8 Overlays

#### Popover
- **职责**：浮层基础定位能力，提供 portal、边界检测、点击外部关闭、Escape 关闭。
- **文件路径**：`overlays/Popover.tsx`
- **Props**：
  - `trigger: ReactNode`
  - `content: ReactNode`
  - `open?: boolean`
  - `onOpenChange?: (open: boolean) => void`
  - `placement?: 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end'`
  - `offset?: number` — 默认 6
  - `matchTriggerWidth?: boolean`
- **必须支持的状态**：open, closed
- **可访问性**：焦点陷阱（可选，menu 场景建议实现）；点击外部关闭；Escape 关闭。
- **实现方案**：**允许引入 `@floating-ui/react`**。`@floating-ui/react` 成熟稳定，手写 portal + viewport detection + flip/shift 容易出错。Phase 2 实现阶段已采用该方案。详见 Phase 2 依赖变更。
- **来源**：HTML demo `.model-menu` 的定位逻辑抽象。
- **禁止**：禁止直接读取 `window.innerWidth` 做同步渲染（应使用 ResizeObserver 或防抖）。

---

### 4.9 Platform

#### PlatformProvider
- **职责**：通过 React Context 提供当前平台信息，供子组件消费。
- **文件路径**：`platform/PlatformProvider.tsx`
- **Props**：
  - `platform: PlatformKind`
  - `children: ReactNode`
- **提供值**：
  - `platform: PlatformKind`
  - `isMac: boolean`
  - `isWindows: boolean`
  - `modifierKey: 'cmd' | 'ctrl'` — 用于快捷键显示与监听提示
- **来源**：跨平台适配需求预留。
- **禁止**：禁止内部检测 `navigator.platform`（应由应用层传入，保持组件库环境无关）。

#### usePlatform
- **职责**：Hook，消费 `PlatformProvider`。
- **文件路径**：`platform/usePlatform.ts`
- **返回值**：`{ platform, isMac, isWindows, modifierKey }`

---

## 5. 类型定义定稿

> **现状**：`frontend/src/types.ts` 当前仅含 `ConversationItem`、`ProjectItem` 两个类型。
> **目标**：本节定义全部 UI 类型，实现阶段将其写入 `frontend/src/types.ts`。
> 类型是 UI 数据模型，不绑定后端 Agent 结构。

```typescript
// frontend/src/types.ts

// === 基础枚举 ===

export type PlatformKind = 'macos' | 'windows' | 'linux' | 'web';

export type ComponentSize = 'sm' | 'md' | 'lg';

export type ComponentTone =
  | 'neutral'   // 默认灰
  | 'local'     // 绿色-本地模式
  | 'permission' // 橙色-权限
  | 'success'   // 绿色-成功
  | 'warning'   // 橙色-警告
  | 'error';    // 红色-错误

export type RunStepStatus = 'pending' | 'active' | 'done' | 'error';

export type ModelSelectorPlacement = 'bottom-start' | 'bottom-end';

// === 导航 ===

export type NavItem = {
  id: string;
  label: string;
  icon?: React.ReactNode;   // lucide-react 图标组件
  shortcut?: string;        // 如 "N", "Shift+P"
  badge?: string | number;  // 右上角徽章
  disabled?: boolean;
};

// === 对话 ===

export type ConversationItem = {
  id: string;
  label: string;       // 任务标题
  timeLabel: string;   // 显示文本，如 "now", "1h", "2h"
  disabled?: boolean;
};

// === 项目树 ===

export type ProjectItem = {
  id: string;
  label: string;
  timeLabel?: string;           // 如 "当前", "3"
  children?: ProjectItem[];     // 子项目/会话
  isLoading?: boolean;          // 展开时显示 loading ring
  disabled?: boolean;
  // 注意：展开状态由 ProjectTree 组件通过 expandedIds / defaultExpandedIds 管理，
  // 不在节点数据中保存，避免 controlled/uncontrolled 状态重复。
};

// === 快捷任务 ===

export type QuickActionItem = {
  id: string;
  label: string;       // chip 显示文本
  prompt: string;      // 点击后填入 composer 的完整文本
  icon?: React.ReactNode;
  disabled?: boolean;
};

// === 模型选项 ===

export type ModelOption = {
  id: string;
  label: string;
  disabled?: boolean;
};

// === 运行步骤 ===

export type RunStepItem = {
  id: string;
  index: number;        // 显示序号（从 1 开始）
  title: string;
  description?: string;
  tag?: string;         // 右侧 code 标签，如 "scope"
  status: RunStepStatus;
};

// === 产物 ===

export type ArtifactItem = {
  id: string;
  title: string;
  description?: string;
  onClick?: () => void;
};

// === Toast ===

export type ToastItem = {
  id: string;
  message: string;
  duration?: number;    // ms，默认 1500
  tone?: ComponentTone;
};

// === Approval / Permission ===

export type ApprovalRiskTone = 'safe' | 'warning' | 'danger';

export type ApprovalItem = {
  id: string;
  title: string;
  description?: string;
  riskTone: ApprovalRiskTone;
  approveLabel?: string;
  rejectLabel?: string;
};

// === Composer 状态 ===

export type ComposerMode = 'Plan' | 'Act';

export type ComposerReadyState = boolean; // 由业务层根据 value.length > 0 计算后传入
```

---

## 6. 状态矩阵

以下状态每个关键组件必须覆盖，并在实现时通过 visual regression 或手动检查验证。

| 状态 | 说明 | 关键组件 |
|---|---|---|
| default | 默认态 | 全部 |
| hover | 鼠标悬停 | Button, IconButton, Pill, QuickActionChip, NavRow, TaskRow, ProjectRow, ProjectSession, ArtifactCard |
| active / selected | 按下或选中 | NavRow, TaskRow, ProjectRow, ProjectSession, ModelMenu item, ModeToggle |
| focus-visible | 键盘聚焦 | 全部可交互组件（button, textarea, [tabindex]） |
| disabled | 禁用 | Button, IconButton, Pill, ComposerInput, SendButton, ModelSelector, QuickActionChip |
| loading | 加载中 | ProjectTree node, SendButton（可选旋转图标） |
| empty | 无数据 | ConversationList, ProjectTree, RunTimeline, ArtifactGrid |
| error | 错误态 | ComposerInput（边框变红）, RunStep, Toast |
| success / warning / neutral | 状态色 | StatusPill, RunStep, Toast |
| long text | 文本溢出 | ConversationItem label, ProjectItem label, ModelOption label, Hero title, RunStep title/description |
| small desktop window / minimum width | 窗口 <= 1180px 或最小 920px | AppShell, CenterStage, Sidebar（CSS media query 处理） |

### 特殊状态说明

- **focus-visible**：所有可交互元素必须有可见 focus 指示。使用 `outline: 1px solid var(--aw-border-active); outline-offset: 2px;`。禁止直接写死 `outline: none` 而不提供替代方案。
- **prefers-reduced-motion**：在 `components.css` 中全局处理：
  ```css
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after {
      animation-duration: 1ms !important;
      transition-duration: 1ms !important;
      scroll-behavior: auto !important;
    }
  }
  ```
- **文本截断**：所有单行文本溢出必须使用 `overflow: hidden; text-overflow: ellipsis; white-space: nowrap;`。多行文本如需截断，使用 `-webkit-line-clamp` 并标注兼容性。
- **滚动区域**：Sidebar scroll、Workspace scroll 必须隐藏滚动条（`::-webkit-scrollbar { width: 0; }`），但保留鼠标滚轮功能。
- **z-index 层级**：
  - AppShell: auto
  - TopBar sticky: 2
  - Popover / Dropdown: 10
  - Toast: 20

---

## 7. 样式与 Token 规范

### 7.1 tokens.css

文件路径：`frontend/src/styles/tokens.css`

必须包含第 2.2 节列出的全部语义 token。组件只能通过 `var(--aw-*)` 引用，禁止写死颜色值。

### 7.2 components.css

文件路径：`frontend/src/styles/components.css`

包含：
- `panel-in` 动画（`@keyframes panel-in`）
- `pulse` 动画（`@keyframes pulse`，用于 RunStep active）
- `prefers-reduced-motion` 全局重置
- 滚动条隐藏公共类（如 `.aw-scroll-hidden`）
- 文本截断公共类（如 `.aw-truncate`）

### 7.3 Logo / Assets 内嵌

- Logo 文件放在 `components/assets/logo.svg`（优先 SVG，如现有 PNG 则同时保留并注明迁移计划）。
- 组件通过 `import logo from '../assets/logo.svg'` 引入，确保 Vite/Rollup 打包时内联或输出到产物目录。
- `BrandRow` 的默认实现使用此 import，不依赖外部 URL。

---

## 8. 包边界与消费方式

### 8.1 统一导出

`frontend/src/components/index.ts` 必须显式导出所有公共组件与类型：

```typescript
export { Icon } from './primitives/Icon';
export { IconButton } from './primitives/IconButton';
// ... 所有组件

export type { NavItem, ConversationItem, ProjectItem, QuickActionItem, ModelOption, RunStepItem, ArtifactItem, ToastItem, PlatformKind, ComponentSize, ComponentTone } from '../types';
```

### 8.2 CSS 入口

业务层消费时引入：

```typescript
import '@awaken/ui-kit/styles/tokens.css';
import '@awaken/ui-kit/styles/components.css';
```

当前阶段在 `frontend/src/main.tsx` 中引入即可。

### 8.3 Assets 打包

- 使用 Vite 的 `assetsInlineLimit` 或 SVGR 将 logo 转为 React 组件（推荐 `@svgr/rollup`），确保组件库发布时无需额外静态文件服务器。
- 若保持 PNG，确保 `vite.config.ts` 的 `build.assetsInlineLimit` 足够大（如 4096），使小图内联为 base64。

### 8.4 package.json 后续补充

当前 `frontend/package.json` 为应用级配置。未来若拆分为独立包，需补充：

```json
{
  "name": "@awaken/ui-kit",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js"
    },
    "./styles/*": "./dist/styles/*"
  },
  "peerDependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  }
}
```

当前阶段不修改 package.json，但组件代码按此边界编写。

---

## 9. 验证标准与归档验收

实现阶段按以下标准验收；归档时，Phase 1～6 已按这些标准完成验证。

### 9.1 构建与类型检查

```bash
# 在 frontend/ 目录执行
npm run build          # 必须无 error
npx tsc --noEmit       # 必须无类型错误
```

### 9.2 组件预览

在 `App.tsx` 中组装完整页面，复刻 HTML demo 所有区域。验收：
- 每个组件至少渲染一次，无 console.error
- `npm run dev` 能启动，手动点过关键交互路径

### 9.3 关键交互验证（手动）

| 验收项 | 操作 | 预期结果 |
|---|---|---|
| Composer ready 态 | 输入任意文本 | SendButton 背景变为 `#1a1a19`（黑色） |
| Cmd/Ctrl+Enter | 输入文本后按快捷键 | 触发 onSubmit callback |
| ModelSelector 开关 | 点击 model-pill | Dropdown 出现，选中后关闭 |
| ModeToggle | 点击 Plan/Act pill | 文字切换，触发 onChange callback |
| QuickActionChip | 点击任意 chip | 触发 onClick(prompt) callback |
| RunTimeline 数据驱动 | 传入不同 status 的 steps | 对应蓝色脉冲/绿色完成/红色错误态 |
| Toast 显示/隐藏 | 传入 visible=true | panel-in 动画出现，duration 后消失 |
| Escape 关闭 | ModelSelector 打开时按 Escape | Dropdown 关闭 |

### 9.4 无后端依赖检查

```bash
# 组件目录应返回空
grep -r "window.awaken" frontend/src/components/
grep -r "require('electron')" frontend/src/components/
grep -r "fetch(" frontend/src/components/
```

### 9.5 可访问性

- 所有 button/input 元素必须有 `aria-label`（由组件 props 传入或组件内部提供）
- focus-visible：键盘 Tab 遍历所有可交互元素，outline 清晰可见
- `prefers-reduced-motion`：系统开启后，所有动画（panel-in、pulse、hover transition）应失效或变为瞬时

### 9.6 布局与长文本

- 窗口宽度 920px~1920px 范围内布局正常（CSS media query 处理）
- 长文本（100+ 字符）不撑破布局：所有 row-title、project-label、step-title 使用 `text-overflow: ellipsis; white-space: nowrap`
- Sidebar 内容超出时滚动条隐藏（`::-webkit-scrollbar { width: 0 }`）

### 9.7 Assets 打包

构建后 `dist/` 目录中 logo 以内联 SVG 或 base64 形式存在，`BrandRow` 无网络请求时正常显示。

---

## 10. 分阶段实现计划

> 归档状态：以下 6 个 Phase 已全部完成。此节保留为实施路径记录，而不是待办清单。

### Phase 1：地基（tokens、types、assets、primitives）— 已完成

**目标**：建立基础设施工具，所有组件能渲染。

1. 创建 `frontend/src/styles/tokens.css`：
   - 写入第 2.2 节全部 `--aw-*` token（颜色、尺寸、圆角、阴影、字体）
   - 使用 `:root { --aw-*: xxx; }` 格式

2. 创建 `frontend/src/styles/components.css`：
   - 写入 `@keyframes panel-in`、`@keyframes pulse`
   - 写入 `.aw-scroll-hidden`（滚动条隐藏）
   - 写入 `.aw-truncate`（文本截断）
   - 写入 `prefers-reduced-motion` 全局重置

3. 扩展 `frontend/src/types.ts`：
   - 写入第 5 节全部类型定义
   - 替换现有 2 个类型为完整定义

4. 放置 logo 到 `frontend/src/components/assets/`：
   - 将 HTML demo 中的内联 SVG brand-mark 提取为 `logo.svg`
   - 验证 Vite 能正确打包

5. 实现 primitives（8 个组件）：
   - `primitives/Icon.tsx`：SVG 包装，18x18，stroke 1.85
   - `primitives/IconButton.tsx`：方形图标按钮，aria-label 必须
   - `primitives/Button.tsx`：文本按钮，支持 icon
   - `primitives/Pill.tsx`：圆角标签，default/bordered 变体
   - `primitives/StatusPill.tsx`：带 tone 色语义的状态标签
   - `primitives/Avatar.tsx`：头像，支持 fallback
   - `primitives/Divider.tsx`：水平分割线
   - `primitives/Kbd.tsx`：键盘快捷键标签，platform-aware

6. 创建 `frontend/src/components/index.ts`：
   - 导出以上 8 个 primitives 及全部公共类型

**验收**：`npm run build` 通过；App.tsx 中渲染每个 primitive 无报错。

---

### Phase 2：核心输入与选择器（composer、dropdown/model selector、quick actions）— 已完成

**目标**：Composer 和相关选择器组件完整可用。

**依赖变更**：
```json
// frontend/package.json
{
  "dependencies": {
    "@floating-ui/react": "^0.x"
  }
}
```
`@floating-ui/react` 用于 Popover 的 viewport detection、flip、shift。手写实现容易出边界 bug，此处允许引入。

1. 实现 `overlays/Popover.tsx`：
   - 基于 React portal
   - 支持 `placement`（bottom-start/bottom-end）
   - 边界防溢出（viewport detection）
   - 点击外部关闭、Escape 关闭

2. 实现 `selectors/Dropdown.tsx`：
   - 组合 Popover
   - 支持 controlled `open` + `onOpenChange`

3. 实现 `selectors/ModelSelector.tsx`：
   - 基于 Dropdown
   - 受控 `value` + `options` + `onChange`

4. 实现 `selectors/ModeToggle.tsx`：
   - Plan/Act 切换
   - onChange callback

5. 实现 `inputs/ComposerInput.tsx`：
   - textarea，受控 `value` + `onChange`
   - auto-resize（minRows/maxRows）
   - `onSubmit`（Cmd/Ctrl+Enter 触发）

6. 实现 `inputs/SendButton.tsx`：
   - `ready` prop 控制黑底白字
   - hover 时 `translateY(-1px)`

7. 实现 `inputs/ContextButton.tsx`：
   - 继承 IconButton，固定 label="添加上下文"

8. 实现 `inputs/Composer.tsx`：
   - 组合 ComposerInput + ContextButton + ModeToggle + ModelSelector + SendButton
   - 所有 callbacks 透传

9. 实现 `content/QuickActionChip.tsx`：
   - `onClick(prompt)` callback 传出

10. 实现 `content/QuickActionGrid.tsx`：
    - flex-wrap gap 容器

**验收**：Composer 可输入、切换模型、切换模式、点击快捷任务；Dropdown 边界防溢出；focus-visible 可见。

---

### Phase 3：导航与侧边栏（sidebar/navigation/project tree）— 已完成

**目标**：完整侧边栏，所有导航组件可用。

1. 实现 `navigation/BrandRow.tsx`：
   - 内嵌 logo.svg import
   - 默认显示 "Awaken"

2. 实现 `navigation/NewTaskButton.tsx`：
   - `onClick` callback
   - `Kbd` 渲染快捷键

3. 实现 `navigation/PrimaryNav.tsx`：
   - `NavItem[]` 数据驱动
   - `onSelect` callback

4. 实现 `navigation/ConversationList.tsx`：
   - `ConversationItem[]` 数据驱动
   - 长文本截断、空状态

5. 实现 `navigation/ProjectTree.tsx`：
   - 支持 `details/summary` 原生展开或受控展开
   - `isLoading` 显示 loading ring
   - 长文本截断

6. 实现 `navigation/ProfileButton.tsx`：
   - Avatar + name + onClick

7. 实现 `navigation/Sidebar.tsx`：
   - 组合以上全部组件
   - 内部管理滚动区域

**验收**：Sidebar 可滚动、项目树可展开折叠、长文本截断、空状态显示。

---

### Phase 4：运行反馈（run feedback、toast、artifact、approvals）— 已完成

**目标**：时间线、产物、权限确认组件完整可用。

1. 实现 `feedback/RunStep.tsx`：
   - `status: 'pending' | 'active' | 'done' | 'error'`
   - active 时 `animation: pulse 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite`
   - done 时绿色背景

2. 实现 `feedback/RunTimeline.tsx`：
   - `RunStepItem[]` 数据驱动
   - `panel-in` 动画进入
   - 支持 empty 状态

3. 实现 `feedback/ArtifactCard.tsx`：
   - hover/active/focus-visible

4. 实现 `feedback/ArtifactGrid.tsx`：
   - `repeat(3, minmax(0, 1fr))` 网格

5. 实现 `feedback/ApprovalCard.tsx`：
   - `title`、`description`、`riskTone`
   - `onApprove` / `onReject` callbacks
   - 受控 `open` / `onOpenChange`
   - riskTone: safe（绿）→ warning（橙）→ danger（红）
   - Escape 关闭并触发 onReject
   - 底层可复用 Popover 定位逻辑

6. 实现 `feedback/Toast.tsx`：
   - `visible` prop 控制显示
   - `panel-in` 动画
   - `duration` 自动消失（业务层或外部定时器控制）

**验收**：RunTimeline 数据驱动步骤状态变化正确；ApprovalCard 三种 riskTone 正确显示；Toast 动画正常；ArtifactGrid 长文本不溢出。

---

### Phase 5：布局与平台适配（AppShell/platform/titlebar）— 已完成

**目标**：完整 AppShell 和跨平台适配。

1. 实现 `layout/CenterStage.tsx`：
   - `width: 768px`（默认）
   - `max-width: calc(100vw - var(--sidebar-width) - 120px)`
   - `margin-top: 96px`（默认）

2. 实现 `layout/TopBar.tsx`：
   - `height: 56px`，sticky
   - 右侧操作区

3. 实现 `layout/WindowTitleBar.tsx`：
   - macOS：显示 traffic light 占位（红黄绿三个点）
   - Windows：显示最小化/最大化/关闭按钮
   - 所有按钮通过 callbacks 传出

4. 实现 `platform/PlatformProvider.tsx`：
   - React Context
   - 提供 `platform`、`isMac`、`isWindows`、`modifierKey`

5. 实现 `platform/usePlatform.ts`：
   - Hook 消费 PlatformProvider

6. 实现 `layout/AppShell.tsx`：
   - `grid-template-columns: var(--sidebar-width) minmax(0, 1fr)`
   - 组合 Sidebar + main

**验收**：AppShell 在 920px~1920px 范围内布局正常；WindowTitleBar 根据 platform 显示不同样式；PlatformProvider 的 modifierKey 正确。

---

### Phase 6：预览验证与清理 — 已完成

**目标**：完整 demo 可运行，所有验收标准通过。

1. 在 `App.tsx` 中组装完整页面：
   - 使用 Phase 1-5 全部组件
   - 复刻 HTML demo 所有区域

2. 运行 `npm run build` 与 `npx tsc --noEmit`

3. 手动验证第 9 节全部 7 个验收类目

4. 清理旧文件（确认替换完成后）：
   - `frontend/src/components/UiPrimitives.tsx`（被 primitives/ 替代）
   - `frontend/src/components/ChatComposer.tsx`（被 inputs/Composer 替代）
   - `frontend/src/components/AppShell.tsx`（被 layout/AppShell 替代）
   - `frontend/src/components/Sidebar.tsx`（被 navigation/Sidebar 替代）
   - `frontend/src/components/ChatHome.tsx`
   - `frontend/src/components/ConversationList.tsx`（被 navigation/ConversationList 替代）
   - `frontend/src/components/ProjectTree.tsx`（被 navigation/ProjectTree 替代）
   - `frontend/src/styles.css`（被 styles/tokens.css + styles/components.css 替代）

5. 更新 `frontend/src/components/index.ts` 确保导出完整

6. 验证 assets 打包：logo 以内联形式存在于 dist/

---

## 11. 归档总结

### 11.1 规格定稿阶段改了哪些章节

| 章节 | 修改内容 |
|---|---|
| **第 1 节** | 明确"零后端依赖"的具体含义；说明现有 React 组件使用旧绿色渐变样式，仅作现状参考；标注 platform prop 预留 macOS/Windows titlebar 适配点 |
| **Sidebar API（第 4.4 节）** | 改为 data-driven 模式，提供完整 SidebarProps：navItems、conversations、projects、profile、activeId、onSelect 等，不再暴露子组件 slot |
| **ProjectTree（第 4.4 节 + 第 5 节）** | 移除 `ProjectItem.expanded`；定稿 `expandedIds[]`（受控）+ `defaultExpandedIds[]`（非受控）+ `onExpandedChange` 契约；禁止混用 |
| **Popover（第 4.8 节）** | 明确允许引入 `@floating-ui/react`；在 Phase 2 增加 package.json 依赖变更说明 |
| **Phase 2** | 增加 `@floating-ui/react` 依赖变更说明 |
| **Phase 4** | 新增 ApprovalCard 组件（`title`、`description`、`riskTone`、`onApprove`、`onReject`），加入 5 个实现步骤 |
| **类型定义（第 5 节）** | 新增 `ApprovalRiskTone` 和 `ApprovalItem` 类型 |
| **第 9 节** | 拆分为 7 个子节，增加可执行验证命令；增加关键交互验证表 |
| **第 10 节** | 每个 Phase 增加"目标"；每个步骤细化到具体文件；Phase 6 列出需删除的 8 个旧文件 |
| **Props 一致性** | Pill 补 `disabled?: boolean`；QuickActionChip 补 `disabled?: boolean` |
| **风险表（第 11.3 节）** | Popover 和 ProjectTree 风险已解决，标注 ~~删除线~~ |

### 11.2 为什么可以作为历史文档沉淀

1. **token 值已与 HTML demo 完全对齐**：第 2.2 节语义 token 的值（如 `--aw-bg-main: #f8f8f7`）与 HTML demo 原始值一一对应，仅添加 `--aw-` 前缀以区分命名空间。

2. **组件职责边界清晰**：每个组件的 props/callback 契约、必须支持的状态、禁止依赖项、来源区域均已明确。

3. **Sidebar API 已定稿**：采用 data-driven 模式，提供完整字段清单，实现时无需猜测 slot vs props 的边界。

4. **ProjectTree 展开/折叠契约已统一**：`ProjectItem.expanded` 已移除，改为 `expandedIds[]` + `defaultExpandedIds[]` + `onExpandedChange`，避免 controlled/uncontrolled 混用。

5. **Popover/Dropdown 方案已明确并落地**：允许引入 `@floating-ui/react`，Phase 2 已采用该方案，无需手写边界检测。

6. **ApprovalCard 已纳入组件体系**：Agent 权限确认是 Awaken 核心交互，已在 Phase 4 中定义完整契约。

7. **类型定义完整且 UI 模型化**：不绑定 Agent runtime，全部为 UI 数据模型，实现阶段可直接复制到 `types.ts`。

8. **分阶段计划已被验证可执行**：6 个 Phase 均按文件清单和验收标准完成实现，最终形成可运行的组件库预览与完整主界面。

9. **验证标准已形成闭环**：构建命令、类型检查、交互验收表、grep 检查和浏览器布局验证已用于最终 review。

### 11.3 已关闭风险与后续边界

| 风险 | 归档状态 |
|---|---|
| ~~**Popover 边界检测复杂度**~~ | ~~已引入 `@floating-ui/react`，Dropdown/ModelSelector 复用 Popover 边界处理~~ |
| ~~**ProjectTree 展开/折叠状态**~~ | ~~已使用 `expandedIds`/`defaultExpandedIds`/`onExpandedChange`，`ProjectItem.expanded` 已移除~~ |
| ~~**WindowTitleBar 的 Electron 窗口控制**~~ | ~~已关闭：组件只暴露 callbacks，Electron IPC 由应用层接入，组件库保持 UI-only~~ |
| ~~**logo SVG 内嵌打包**~~ | ~~已关闭：`BrandRow` 从本地 `components/assets/logo.svg` import，构建产物无网络 logo 请求~~ |
| ~~**旧文件删除时机**~~ | ~~已关闭：Phase 6 删除旧 `UiPrimitives`、旧 `ChatComposer`、旧 `AppShell`、旧 `Sidebar`、旧 `styles.css` 等替代文件，并完成构建验证~~ |

后续边界：本文档只记录 UI Kit 从 HTML demo 到 React 组件库的第一轮建设。Electron 主进程、真实 Agent runtime、后端数据接入、发布 npm 包和跨项目迁移，应另开文档设计。
