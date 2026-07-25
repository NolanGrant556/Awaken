# Awaken Design System

Awaken Design System 是 Awaken 的 UI-only 组件系统。它包含可复用 React 组件、CSS tokens、组件局部样式、可观测 demo states，以及用于肉眼检查组件真实形态的本地 Gallery。

这套组件必须可以从 Awaken Desktop 中抽离后继续复用。它不能依赖 Go runtime、Electron、AG-UI、A2UI、SSE、本地文件系统或 `window.awaken`。

## 目录职责

```text
Awaken Design System/
├── src/                         # 设计系统源码，只放可复用 UI 能力
│   ├── components/              # 独立组件目录，每个组件一个文件夹
│   ├── tokens/                  # 全局设计令牌与组件基础样式
│   ├── hooks/                   # UI-only React hooks
│   ├── assets/                  # 设计系统可复用静态资源
│   ├── index.ts                 # 包主出口，Desktop 与外部项目应从这里导入
│   └── types.ts                 # 设计系统共享类型
├── gallery/                     # 本地组件观测服务源码
│   ├── registry.ts              # 组件 URL 与 demo 注册表，组件观测入口的事实源
│   ├── AwakenGalleryApp.tsx     # Gallery 壳层与 hash 路由
│   ├── main.tsx                 # Gallery Vite 入口
│   └── styles.css               # Gallery 专用样式，不应变成组件样式事实源
├── smoke/                       # 自动化渲染冒烟检查
│   └── render-smoke.ts          # server-render 所有 Awaken*Demo
├── dist/                        # 构建产物，由 build 生成，不手写维护
├── DESIGN_SYSTEM_GUIDE.md       # Awaken 视觉消费与 Agent 作者规则
├── index.html                   # Gallery HTML 入口
├── package.json                 # @awaken/design-system 包、脚本与依赖声明
├── vite.config.ts               # Gallery 与包构建配置
├── tsconfig.json                # 开发期 TypeScript 配置
└── tsconfig.build.json          # 类型声明构建配置
```

关键边界：

- `src/components/` 是组件真实实现；`gallery/` 只负责观测，不承载产品逻辑。
- `src/tokens/` 是颜色、间距、字体、边框、组件基础类等设计令牌事实源。
- `dist/` 是生成目录，可以删除后重建；不要在里面修 bug。
- `DESIGN_SYSTEM_GUIDE.md` 是后续视觉改动的作者规则，优先于临时审美判断。
- `gallery/registry.ts` 决定哪些独立组件或组件族拥有顶层观测 URL，以及入口所属的 Gallery 目录分区。
- Gallery 左侧的“Awaken 设计系统 / 组件目录”是固定目录头，只有下方的目录分区和组件入口参与纵向滚动。
- `src/index.ts` 决定哪些能力对 Desktop 或外部项目公开。

## 入口契约

Awaken Desktop 的正式平台入口由仓库根 `README.md` 唯一定义。macOS 当前从仓库根目录启动和关闭：

```bash
npm run open:desktop
npm run close:desktop
```

Windows 当前使用前台入口 `npm run dev:desktop`，并在对应终端按 `Ctrl+C` 关闭。平台验收范围和入口细节不在本文档重复维护。

组件独立观测入口：

从仓库根目录启动并打开 Gallery：

```bash
cd /Users/nolangrant/Awaken
npm run open:ui
```

如果只需要启动服务、不需要自动打开浏览器：

```bash
cd /Users/nolangrant/Awaken
npm run dev:ui
```

Gallery 地址：

```text
http://127.0.0.1:5173/#/components/<component-id>
```

然后打开具体组件 URL，例如：

```text
http://127.0.0.1:5173/#/components/awaken-composer-bar
http://127.0.0.1:5173/#/components/awaken-run-stage
http://127.0.0.1:5173/#/components/awaken-workbench-shell
http://127.0.0.1:5173/#/components/awaken-product-logo
http://127.0.0.1:5173/#/components/awaken-memory-settings
```

`npm run dev:ui` 只是启动组件观测服务的命令。真正的观测入口是注册表中的独立组件或组件族 URL。组件族可以在一个入口内集中展示完整组合及其可复用小组件；`http://127.0.0.1:5173/` 会自动落到某个已注册 URL，本身不作为独立入口。

## 组件 URL 规则

每个组件的 `component-id` 由组件名转换为 kebab case：

```text
AwakenComposerBar -> awaken-composer-bar
AwakenExecutionFeed -> awaken-execution-feed
AwakenWorkbenchShell -> awaken-workbench-shell
```

已注册组件 URL 的唯一事实源是：

```text
Awaken Design System/gallery/registry.ts
```

同一产品能力下的小组件可以集中到一个组件族入口，例如：

```text
http://127.0.0.1:5173/#/components/awaken-skills
http://127.0.0.1:5173/#/components/awaken-sidebar
http://127.0.0.1:5173/#/components/awaken-account
http://127.0.0.1:5173/#/components/awaken-settings
http://127.0.0.1:5173/#/components/awaken-connectors
```

Gallery 左栏按“原子组件”“左侧栏组件”“设置页组件”“中间交互区组件”“布局 / 组合组件”五个目录分区。每个分区标题都是可访问的展开/收起按钮，显示当前入口数量；首次打开 URL 时只展开当前组件所属分区，切换 hash 到其他分区的组件时对应分区自动展开。所有 Gallery 注册项必须在 `registry.ts` 显式声明分类，不能回退为没有归属的一列长清单。

“原子组件”只保留确实需要脱离业务场景单独观测的视觉单元。当前入口为产品 LOGO、开关、文件夹、头像和原始思考动效。`AwakenSwitch` 是全产品布尔启停的唯一视觉原子，开启使用成功绿色、关闭使用中性灰色；Skills、连接器和设置语义外壳只做兼容映射，不再维护各自颜色与几何。`AwakenFolder` 是 Skills 与记忆项目入口共用的中性文件夹原子，只拥有名称、辅助信息和受控展开交互。图标按钮、弹出层、发送按钮、选择卡片、工具提示和记忆光流 primitive 仍保留生产源码和公开导出，但随真实消费组件族观测，不再用虚构状态重复占用目录。`AwakenMemoryLightFlow` 是适配浅色与深色主题的纯装饰 Canvas 光流，以铺满正常页头高度的独立细线构成；`AwakenMemoryLightFlowLayout` 负责把它与现有 `AwakenMemorySettings` 组合为正式记忆页布局，并统一在“记忆设置”入口观测。通用界面图形直接使用 `lucide-react`，不再维护只包裹 SVG path 的 `AwakenIcon`；原 `awaken-icon` URL 跳转到展示其真实替代位置的“「左侧栏」组件库”。其中“产品 LOGO”入口对应 `AwakenProductLogo`，直接拥有 `src/assets/product-logo.png` 这份官方 256×256 方形产品标志。横向 `AWAKEN` 字标仍由 `logo-wordmark.svg` 负责；两者用途不同，不得互相裁切替代。

“左侧栏组件”收纳 `AwakenSidebar`、Skills、Account、Connectors 等工作台侧栏组件族；设置页不再作为左侧栏弹窗归入该目录。“中间交互区组件”只收纳 Desktop 当前采用的 `RunStage`、center/dock `ComposerBar`、执行叙事、提示与回答、工具轨迹和最终交付物组件；该分组的 Gallery 状态统一一行一列展示，便于按完整可用宽度复核中间区组件，但不改变 Desktop 对滚动、overlay、composer 和 runtime 编排的 ownership。其中“输入栏”组件族用真实 Workbench 缩略场景明确区分上方滚动输出与底部停靠输入栏，并从真实“上下文窗口”元信息入口打开 `RunStage.overlay` 中的 `AwakenContextUsagePanel`；随后集中展示 `AwakenComposerBar`、底层 `AwakenComposerInput`、由 `AwakenIconButton` 实现的添加上下文按钮、`AwakenSendButton` 的发送/停止状态、两者的 `AwakenTooltip`，以及 `AwakenUserPromptBlock` 的已提交提示及复制状态。相关组件保留独立源码和公开导出，但不重复占用 Gallery 目录；原 `awaken-composer-input`、`awaken-context-button`、`awaken-user-prompt-block`、`awaken-composer-dock`、`awaken-icon-button`、`awaken-send-button`、`awaken-tooltip` URL 均跳转到该组件族入口。“助手消息”组件族通过已完成回答底部操作区观测 `AwakenCopyButton` 的真实回答复制状态；复制组件保留独立源码和公开导出，原 `awaken-copy-button` URL 跳转到该组件族入口。“运行阶段”组件族先用完整运行缩略场景标出 `AwakenRunStage` 的滚动/停靠 ownership 与 `AwakenExecutionFeed` 的叙事边界，再补充 RunStage 自身状态；两个组件保留独立源码和公开导出，原 `awaken-execution-feed` URL 会跳转到该组件族入口。“工具轨迹”组件族先展示 Desktop 在最终回答后由 `AwakenDisclosure` 收纳“执行过程”的真实组合，再集中观测 `AwakenToolTraceGroup` 与 `AwakenToolTraceLine`；三者保留独立源码和公开导出，原 `awaken-disclosure`、`awaken-tool-trace-group`、`awaken-tool-trace-line` URL 均跳转到统一入口。“「最终交付物」组件库”集中展示 `AwakenDeliverableCard` 的单文件状态与 `AwakenDeliverableList` 的列表组合状态；两者同样保留独立源码和公开导出，原 `awaken-deliverable-card`、`awaken-deliverable-list` URL 会跳转到统一入口。

“布局 / 组合组件”当前只注册“「工作台框架」组件库”，统一展示 Desktop 正在消费的 `AwakenAppShell`、`AwakenWorkbenchChrome`、`AwakenWorkbenchShell` 和标准 `AwakenInspectorRail`。平台值由 Desktop runtime 传入，macOS 窗口按钮和拖拽区域仍由 Electron 原生窗口负责；设计系统不再提供重复的平台 Provider 或模拟窗口标题栏。旧的 `awaken-app-shell`、`awaken-inspector-rail`、`awaken-light-inspector-rail`、`awaken-workbench-chrome` 等 URL 会跳转到统一入口。

“「技能」组件库”入口同时观测完整 Skills 组合、Skill 卡片、项目窗口和 Markdown 预览窗口；其中开关与项目文件夹直接消费原子 `AwakenSwitch`、`AwakenFolder`，并分别在原子组件目录单独观测。Skill/Connector 的旧命名组件只保留兼容外壳，不再拥有独立视觉实现。

“「左侧栏」组件库”同样先观测 Desktop 当前使用的 `AwakenWorkbenchSidebarFrame + AwakenSidebar` 完整组合，再集中展示 New Agent、主导航、项目树、区块动作和线程运行指示，并补充项目/会话更多菜单、项目树空状态，以及创建项目、编辑项目、移除项目和重命名会话四个 App 级真实弹窗流程；账户入口只在完整组合中保留实际位置，不在该组件族内重复展开。`AwakenDropdown` 保留独立源码和公开导出，并在项目树的真实更多菜单中观测；它不再用虚构的模型、视图菜单重复占用原子组件目录，原 `awaken-dropdown` URL 跳转到“「左侧栏」组件库”。跨产品区域复用的 `AwakenDialog` 同样保留独立源码和公开导出，但不再重复占用原子组件目录；弹窗关闭动作继续使用 `AwakenIconButton`，创建项目流程在弹窗内观测 Desktop 当前唯一支持的本地 `AwakenSelectableCard`；`AwakenProjectFoldersEditor` 保留独立源码和公开导出，并在创建项目与编辑项目两个弹窗中观测项目的文件夹列表、主文件夹徽标和文件夹行更多菜单；`AwakenProjectHoverCard` 只能由项目行悬停触发，因此它的状态演示落在项目树的「悬停展示文件夹组成」里，用来观测多文件夹项目和零文件夹项目两种卡片。技能、设置和连接器仍在各自组件族中观测相应弹窗；原 `awaken-dialog` 与 `awaken-selectable-card` URL 均跳转到“「左侧栏」组件库”。

“「账户」组件库”集中展示 `AwakenAccount` 完整组合、`AwakenProfileButton` 个人入口和 `AwakenProfileBalancePanel` 额度/设置面板；`AwakenPopover` 只作为账户与项目菜单等真实组合的浮层能力观测，不再展示虚构的模型、Plan/Act 或权限菜单，原 `awaken-popover` URL 跳转到本组件族。运行时额度请求留在 Desktop，Design System 只接收数据和回调。

“设置页组件”拥有独立顶层目录：`AwakenSettings` 是覆盖 Desktop 内容窗口的左右分栏框架，保留原生窗口 chrome，左侧提供“返回应用”和五个现有分区，右侧由各页面组件独立拥有内容与滚动。模型、搜索、记忆、外观、宠物分别注册 `awaken-model-settings`、`awaken-search-settings`、`awaken-memory-settings`、`awaken-appearance-settings`、`awaken-pets-settings`；旧 `awaken-memory-light-flow` URL 重定向到记忆设置。设置框架不再套 `AwakenDialog`，没有 backdrop、外层圆角卡片、右上角关闭按钮或底部关闭栏。Gallery 只注入 mock 数据；Desktop 的 `DesktopSettingsDialog` 保留运行时读取、保存和副作用，并直接消费这些 UI-only 组件。跨组件族复用的 `AwakenButton` 保留独立源码和公开导出，原 `awaken-button` URL 跳转到设置页框架。

“「连接器」组件库”集中展示 Desktop 直接消费的 `AwakenConnectors` 完整 MCP/CLI 组合，以及连接器卡片、品牌图标、server 开关、CLI 详情、MCP 配置和 MCP 设置弹窗。Gallery 只注入 mock 状态；连接器目录、MCP server snapshot、CLI readiness、安装/授权、配置保存、日志读取与持久化仍由 Desktop Main/Core/Renderer 对应 owner 负责。

## 组件文件夹契约

每个组件都应该独立放在自己的文件夹里；只有拥有顶层 Gallery 入口的组件或组件族必须保留独立 `*.demo.tsx`，被组件族收纳的小组件由组件族 Demo 观测：

```text
src/components/AwakenComponentName/
├── AwakenComponentName.tsx
├── AwakenComponentName.css
├── AwakenComponentName.states.ts
├── AwakenComponentName.demo.tsx # 仅顶层 Gallery 入口必需
├── README.md
└── index.ts
```

文件职责：

- `*.tsx`：组件实现。
- `*.css`：组件局部样式入口。
- `*.states.ts`：可观测状态与 mock props。
- `*.demo.tsx`：由 Gallery 渲染的顶层组件或组件族 demo；被组件族收纳的小组件不重复维护独立 demo。
- `README.md`：给人和 Coding Agent 看的组件契约。
- `index.ts`：生产组件、类型和需要共享的状态导出；不得导出 `*.demo.tsx`。

生产包与 Gallery 使用两个独立入口：

- `src/index.ts` 与 `src/components/index.ts` 是 `@awaken/design-system` 的正式生产出口，只包含 Desktop 可消费的组件、类型、tokens 与资源。
- `gallery/demos.ts` 是 Gallery-only Demo 出口；`gallery/registry.ts` 直接引用各组件的 `*.demo.tsx`。
- `npm run build` 会分别生成 `dist/index.js` / `dist/index.css` 和仅供 smoke 使用的 `dist/gallery.js` / `dist/gallery.css`。Desktop 只消费正式 `index` 出口，不能引入 `gallery` 产物。

## Coding Agent 规则

- 本目录内的可执行源码统一使用 TypeScript：React/JSX 文件使用 `.tsx`，其余逻辑使用 `.ts`；CSS、HTML、JSON、Markdown、静态资源和生成后的 `dist/*.js` 不属于这一源码后缀约束。
- 组件必须保持 UI-only：不能调用 runtime、Electron IPC、AG-UI、A2UI、SSE、本地文件系统或 `window.awaken`。
- 数据通过 props 进入，行为通过 callbacks 传出。
- 组件 props 或视觉状态变化时，必须同步更新可观测 states。
- 新的独立组件或组件族 demo 必须注册到 `gallery/registry.ts`；组件族内部小组件可以只在对应组件族 demo 中集中展示。
- 组件目录 `index.ts` 不得导出 `*.demo.tsx`；新增 Demo 时同时加入 `gallery/demos.ts`，防止 Demo JavaScript 和 Demo CSS 进入 Desktop renderer。
- 瞬时忙碌态（`busy`、`loading`、保存中）不得切换鼠标光标。macOS 把 `wait` / `progress` 画成指针处的系统忙碌转盘，`not-allowed` / `default` 也会在点击瞬间闪一下；只有长期不可用状态才使用 `not-allowed`。忙碌态同样不改变颜色和透明度。
- 仅为承接程序化焦点而存在的 `tabIndex={-1}` 容器必须自行关闭 `:focus-visible` 焦点环。Chromium 在脚本 `focus()` 且上一次输入来自键盘时会让这类容器命中 `:focus-visible`，被 `tokens/components.css` 的全局规则画上整块面板的 outline。`tabIndex={0}` 的键盘可达区域必须保留可见焦点样式。
- 官方方形产品标志只能通过 `AwakenProductLogo` 使用；Desktop 和其他 consumer 不保留或注入第二份方形 Awaken 图标。
- Desktop 应优先通过 `@awaken/design-system` 包出口使用组件，不要从 `Awaken Design System/src/...` 做源码路径 import。
- 独立组件必须能通过自己的 `#/components/<component-id>` URL 打开；归属于组件族的小组件必须在组件族 URL 中可查看和交互。

## 验证

修改组件或 tokens 后运行：

```bash
npm --workspace @awaken/design-system run typecheck
npm --workspace @awaken/design-system run smoke:render
```

`smoke:render` 会从 Gallery-only bundle server-render 所有 `Awaken*Demo` 组件，同时继续用生产 bundle 检查正式组件状态。如果 demo 缺失、渲染失败，或生产/Gallery 构建边界失效，命令会失败。
