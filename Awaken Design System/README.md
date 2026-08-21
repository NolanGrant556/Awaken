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

## 图标库契约

Awaken 自有的线性图标集由 `src/icons` 唯一拥有，`AwakenIcon` 是唯一渲染出口，Gallery 入口是 `awaken-icon`。

### 分层与 owner

| 文件 | 职责 |
| --- | --- |
| `src/icons/geometry.ts` | 几何内核：op 构造、倒角、光学形状归一化、path 编译 |
| `src/icons/primitives.ts` | 复用原语：文件夹、文件、加减叉、折叠箭头、六边形、齿轮 |
| `src/icons/definitions.ts` | 图标定义。每个图标只声明几何与光学形状 |
| `src/icons/brandMarks.ts` | 第三方官方品牌标记，保留 authored geometry |
| `src/icons/registry.ts` | 名字 → 渲染数据，`AwakenIconName` 类型 |
| `src/icons/concepts.ts` | 概念 → 图形的唯一映射 |
| `src/icons/sizes.ts` | 尺寸档，与 `tokens.css` 的 `--aw-icon-size-*` 同一套 |

### 不可违反的约束

- 16 单位网格，坐标一律是 stroke-center 坐标。
- 基本线只允许 0 / 45 / 90 度；其它角度必须由概念本身要求。
- 先画直线再倒圆角，不使用自由曲线；全套圆头端点与圆角接合。
- 描边固定 `1.25`，是几何常量而不是可换肤 token —— 它参与光学归一化，改它会同时改变每个图标的视觉尺寸。`AwakenIcon` 不接受调用点覆盖描边。既有的 `--aw-icon-stroke: 1.75` 只服务尚未迁移的 lucide 调用点。
- 每个图标声明一个光学形状，视觉包围盒归一化到该形状尺寸：Square `13`、Circle `14`、Horizontal `14×11`、Vertical `11×14`。圆形族用外接圆归一化（`fit: "radial"`），否则与方形族不等重量。
- 重叠元素之间的 stroke-center 净间隙不小于 3 个网格单位。
- 尺寸只接受 `12 / 14 / 16 / 20 / 24` 五档。16 是主用尺寸，12 是全套的可读下限。
- 内标密度高的图标声明更高的 `minSize`，调用点不得低于它。
- 闭合路径从顶点 0 的**出边**切点起笔。用入边切点起笔会让收尾的 `Z` 沿该顶点的弦闭合，把第一个角切掉；描边形状上表现为第一个角变平，实心形状上表现为缺一块。
- 圆形族的 `radial` 归一化以**圆本身的圆心**为锚点，不能用包围盒中心：缺口圆弧（`spinner`）的包围盒并不以圆心为中心。

### 状态族

工具轨迹与运行计划的状态共用同一个容器圆（`statusRing`）加不同内标，因为它们经常同屏逐行出现，容器一旦不同就会看出参差。迁移前这一族是散的：错误有 `AlertCircle` / `CircleAlert` / `AlertTriangle` / `TriangleAlert` 四个同义图标，`unsupported` 与 `error` 共用一个因此无法区分「工具失败」和「工具不可用」，loading 有 `LoaderCircle` / `Loader2` 两个。现在每个状态在 `concepts.ts` 里只有一个图形。

`slash-circle` 的斜杠按「禁止」的通用画法贯穿到接近圆周，是对「净间隙 ≥3 单位」的**有意例外**：缩短反而读不出禁止。这类例外必须写在图标定义的注释里。

### 实心字形

只给产品真正需要实心的概念做实心版。实心图标的视觉包围盒就是几何包围盒，不为描边预留半个描边宽度，因此按 `stroke 0` 归一化。当前唯一的实心字形是 `stop` —— 描边版在 16px 下读起来像一个空框，和「发送」的视觉重量不成对比。允许实心的清单在图标契约测试里显式登记，新增必须同时改测试。

### 六边形形状族

尖顶正六边形是 Awaken「可装配单元」的族形，全套里唯一，因此小尺寸下不会与方框族、文件族混淆。当前成员：`skill`（基型，可读下限 12px）、`skill-core`、`hook`、`connector`（带内标，可读下限 16px）。该外形与 Agent Skills 生态的事实标记一致，但几何由 Awaken 自有 `hexagon` 原语生成，不引用外部文件。

### 品牌标记

第三方官方标记不进入 16 网格构造体系，也不参与光学归一化，按既有规则保留 authored geometry；名字带 `-mark` 后缀与自绘图标区分，并各自声明来源、许可、商标归属与 `minSize`。

MCP 使用官方标记 `mcp-mark`，可读下限 20px —— 更小尺寸下互扣勾形的平行笔画会粘连，这是标记结构决定的，与缩放方式无关。因此 14–16px 的密集行改用 Awaken 自绘的 `mcp-compact` 伴随字形，不改动官方标记。

### 概念优先

产品代码优先用 `concept` 而不是 `name`：概念是稳定的语义 API，图形属于图标层，可以在不动调用点的前提下替换。`concepts.ts` 保证每个「X 用哪个图标」只有一个答案。

### 图标库的验证

图标层的契约测试在 `apps/desktop/src/renderer/awakenIconLibrary.test.ts`（仓库当前只有 desktop workspace 配置了 vitest，且它的 alias 直接指向本包源码）。覆盖几何快照、光学形状审计、网格边界、描边不变量、概念表完整性，以及「图标层不依赖 lucide-react」与「调用点只用尺寸档且不低于 minSize」两条守卫。几何一旦改动，快照会在 review 中显示 path 级别的差异。

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

“原子组件”只保留确实需要脱离业务场景单独观测的视觉单元。当前入口为产品 LOGO、开关、文件夹、头像、原始思考动效、思考球和矩阵方块。`AwakenThinkingOrb` 通过 `thinking-orbs` 的公开 API 组合六种 Canvas 视觉状态与 64px、20px 两档专用尺寸；`AwakenThinkingOrbStatus` 以透明行内布局组合思考球、状态文案与三点节奏动效，不提供胶囊表面；`思考中` 明确映射到 `composing`，不虚构第七种 state。Gallery 独立观测完整组件族，Desktop 只通过 `AwakenThoughtTrace` 在 active 状态注入当前主题和运行状态，完成态由 ThoughtTrace 切换为 Chevron 历史入口；Design System 不读取或推断 runtime。`AwakenDotmSquare3` 是固定的 5×5 方形点阵加载指示器；`AwakenSidebarThreadSpinner` 只组合它的 14px 尺寸，不另行维护一套动效。`AwakenSwitch` 是全产品布尔启停的唯一视觉原子，开启使用成功绿色、关闭使用中性灰色；Skills、连接器和设置语义外壳只做兼容映射，不再维护各自颜色与几何。`AwakenFolder` 是 Skills 与记忆项目入口共用的中性文件夹原子，只拥有名称、辅助信息和受控展开交互。图标按钮、弹出层、发送按钮、选择卡片、工具提示和记忆光流 primitive 仍保留生产源码和公开导出，但随真实消费组件族观测，不再用虚构状态重复占用目录。`AwakenMemoryLightFlow` 是适配浅色与深色主题的纯装饰 Canvas 光流，以铺满正常页头高度的独立细线构成；`AwakenMemoryLightFlowLayout` 负责把它与现有 `AwakenMemorySettings` 组合为正式记忆页布局，并统一在“记忆设置”入口观测。“图标库”入口对应 `AwakenIcon`，它是 Awaken 自有线性图标集的唯一渲染出口，`awaken-icon` 是它的正式 URL。它与早先被移除的同名组件不是同一件东西：那一版只包裹外部 SVG path，没有拥有几何，因此没有存在价值；当前这一版拥有 `src/icons` 下的完整图标层（几何内核、复用原语、光学形状归一化、概念表），因此图标层的 owner 回到 Design System。尚未迁移的界面图形仍直接使用 `lucide-react`；两套并存是迁移中的状态，新增图形不得再引入 lucide 字形。其中“产品 LOGO”入口对应 `AwakenProductLogo`，默认渲染 `product-mark.svg` 双柱标记，`icon` 变体才使用 `src/assets/product-logo.png` 这份官方 256×256 App Icon。横向 `AWAKEN` 字标仍由 `logo-wordmark.svg` 负责；三件套用途不同，不得互相裁切替代。

“左侧栏组件”收纳 `AwakenSidebar`、Skills、Account、Connectors 等工作台侧栏组件族；设置页不再作为左侧栏弹窗归入该目录。“中间交互区组件”只收纳 Desktop 当前采用的 `RunStage`、center/dock `ComposerBar`、执行叙事、提示与回答、工具轨迹和最终交付物组件；该分组的 Gallery 状态统一一行一列展示，便于按完整可用宽度复核中间区组件，但不改变 Desktop 对滚动、overlay、composer 和 runtime 编排的 ownership。其中“输入栏”组件族用真实 Workbench 缩略场景明确区分上方滚动输出与底部停靠输入栏，并从真实上下文圆环入口（hover 摘要、点击打开）打开 `RunStage.overlay` 中的 `AwakenContextUsagePanel`；`center-idle` 状态还观测由受控 `projectPicker` 提供的首页项目低栏，项目数据与创建逻辑仍由 consumer 拥有。随后集中展示 `AwakenComposerBar`、底层 `AwakenComposerInput`、由 `AwakenIconButton` 实现的添加上下文按钮、`AwakenSendButton` 的发送/停止状态、两者的 `AwakenTooltip`，以及 `AwakenUserPromptBlock` 的已提交提示及复制状态。相关组件保留独立源码和公开导出，但不重复占用 Gallery 目录；原 `awaken-composer-input`、`awaken-context-button`、`awaken-user-prompt-block`、`awaken-composer-dock`、`awaken-icon-button`、`awaken-send-button`、`awaken-tooltip` URL 均跳转到该组件族入口。“助手消息”组件族通过已完成回答底部操作区观测 `AwakenCopyButton` 的真实回答复制状态；复制组件保留独立源码和公开导出，原 `awaken-copy-button` URL 跳转到该组件族入口。“运行阶段”组件族先用完整运行缩略场景标出 `AwakenRunStage` 的滚动/停靠 ownership 与 `AwakenExecutionFeed` 的叙事边界，再补充 RunStage 自身状态；两个组件保留独立源码和公开导出，原 `awaken-execution-feed` URL 会跳转到该组件族入口。“工具轨迹”组件族先展示 Desktop 在最终回答后由 `AwakenDisclosure` 收纳“执行过程”的真实组合，再集中观测 `AwakenToolTraceGroup` 与 `AwakenToolTraceLine`；三者保留独立源码和公开导出，原 `awaken-disclosure`、`awaken-tool-trace-group`、`awaken-tool-trace-line` URL 均跳转到统一入口。“「最终交付物」组件库”集中展示 `AwakenDeliverableCard` 的单文件状态与 `AwakenDeliverableList` 的列表组合状态；两者同样保留独立源码和公开导出，原 `awaken-deliverable-card`、`awaken-deliverable-list` URL 会跳转到统一入口。

“布局 / 组合组件”当前只注册“「工作台框架」组件库”，统一展示 Desktop 正在消费的 `AwakenAppShell`、`AwakenWorkbenchChrome`、`AwakenWorkbenchShell` 和标准 `AwakenInspectorRail`。平台值由 Desktop runtime 传入，macOS 窗口按钮和拖拽区域仍由 Electron 原生窗口负责；设计系统不再提供重复的平台 Provider 或模拟窗口标题栏。旧的 `awaken-app-shell`、`awaken-inspector-rail`、`awaken-light-inspector-rail`、`awaken-workbench-chrome` 等 URL 会跳转到统一入口。

“「技能」组件库”入口同时观测完整 Skills 组合、Skill 卡片、项目窗口和 Markdown 预览窗口；项目窗口的紧凑文件夹 Popover 是实际筛选器，提供“全部文件夹”和每个项目文件夹选项，零 Skill 文件夹仍可选择并显示文件夹空态，且不展示路径。其中开关与项目文件夹直接消费原子 `AwakenSwitch`、`AwakenFolder`，并分别在原子组件目录单独观测。Skill/Connector 的旧命名组件只保留兼容外壳，不再拥有独立视觉实现。

“「左侧栏」组件库”同样先观测 Desktop 当前使用的 `AwakenWorkbenchSidebarFrame + AwakenSidebar` 完整组合，再集中展示 New Agent、主导航、项目树、独立对话列表、双分区展开/收起、共用窄滚动条、区块动作和线程运行指示，并补充项目/会话更多菜单、项目树空状态，以及创建项目、编辑项目、移除项目和重命名会话四个 App 级真实弹窗流程；账户入口只在完整组合中保留实际位置，不在该组件族内重复展开。`AwakenDropdown` 保留独立源码和公开导出，并在项目树的真实更多菜单中观测；它不再用虚构的模型、视图菜单重复占用原子组件目录，原 `awaken-dropdown` URL 跳转到“「左侧栏」组件库”。跨产品区域复用的 `AwakenDialog` 同样保留独立源码和公开导出，但不再重复占用原子组件目录；弹窗关闭动作继续使用 `AwakenIconButton`，创建项目流程在弹窗内观测 Desktop 当前唯一支持的本地 `AwakenSelectableCard`；`AwakenProjectFoldersEditor` 保留独立源码和公开导出，并在创建项目与编辑项目两个弹窗中观测项目的文件夹列表、主文件夹徽标和文件夹行更多菜单；`AwakenProjectHoverCard` 只能由项目行悬停触发，因此它的状态演示落在项目树的「悬停展示文件夹组成」里，用来观测多文件夹项目和零文件夹项目两种卡片。技能、设置和连接器仍在各自组件族中观测相应弹窗；原 `awaken-dialog` 与 `awaken-selectable-card` URL 均跳转到“「左侧栏」组件库”。

“「账户」组件库”集中展示 `AwakenAccount` 完整组合、`AwakenProfileButton` 个人入口、`AwakenProfileMenuPanel` 快捷面板和 `AwakenProfileUsageDialog` 双栏用量窗口；个人弹层依次左对齐展示“外观”“查询用量”“设置”，默认不显示卡片表面，仅在悬停时提供灰色反馈。“外观”在右侧小窗中展开三种主题选择；“查询用量”点击后左侧选择已配置 API / Plan，右侧展示对应余额或配额，详情头不重复显示“用量”标签。需要额外云账号管理授权或只能前往供应商平台查看的条目展示准确说明和官方入口，不把推理 Key 描述为管理凭据。`AwakenPopover` 只作为账户与项目菜单等真实组合的浮层能力观测，不再展示虚构的模型、Plan/Act 或权限菜单，原 `awaken-popover` URL 跳转到本组件族。运行时用量请求留在 Desktop，并且只在打开用量窗口后触发；Design System 只接收数据和回调。

“设置页组件”拥有独立顶层目录：`AwakenSettings` 是覆盖 Desktop 内容窗口的左右分栏框架，保留原生窗口 chrome，左侧提供“返回应用”以及模型、搜索、记忆、全局规则、外观、宠物、计算机操作、钩子和 ACP 导航，右侧由各页面组件独立拥有内容与滚动。模型、搜索、记忆、外观、宠物、计算机操作、ACP 分别注册 `awaken-model-settings`、`awaken-search-settings`、`awaken-memory-settings`、`awaken-appearance-settings`、`awaken-pets-settings`、`awaken-computer-settings`；全局 AGENTS.md 和 Hook 页面由 Desktop 拥有业务状态，Design System 只提供导航 item 与设置框架。旧 `awaken-memory-light-flow` URL 重定向到记忆设置。标准设置页继续保留全宽一级标题，宠物、计算机操作和 ACP 的实际配置主体共用 820px focused content width，在右侧工作区内水平居中并使用紧凑卡片密度；标准内容区滚动条使用透明轨道、低对比圆角 thumb，与 Workbench 的轻量滚动表面保持一致，窄窗口仍回退为可用宽度。设置框架本身不套 `AwakenDialog`，没有 backdrop、外层圆角卡片、右上角关闭按钮或底部关闭栏；ACP runner 的添加与编辑属于页面内任务，复用 `AwakenDialog` 在屏幕中央承载表单、内部滚动、底部操作、Escape/遮罩关闭和焦点恢复。Gallery 只注入 mock 数据；Desktop 的 `DesktopSettingsDialog` 保留运行时读取、保存和副作用，并直接消费这些 UI-only 组件。跨组件族复用的 `AwakenButton` 保留独立源码和公开导出，原 `awaken-button` URL 跳转到设置页框架。

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
- Desktop 与 Gallery 必须各自在根入口调用一次 `installAwakenFocusModality(document.documentElement)`，用 `data-aw-focus-modality` 区分 pointer 与 keyboard。鼠标、触控和由其触发的程序化恢复焦点不得显示键盘专用外置 outline；组件继续保留 Gallery、辅助技术和程序化聚焦所需的 `focus-visible` 反馈。Awaken Desktop 当前另由产品入口统一消费未修饰 `Tab` / `Shift+Tab`，不得为此逐组件删除组件语义或改写 `tabIndex`；Gallery 不继承该 Desktop 策略。当前 `Thought` 使用文字下划线而不生成外框。仅为承接程序化焦点而存在的 `tabIndex={-1}` 容器仍必须自行关闭 `:focus-visible` 焦点环。
- 官方产品标记只能通过 `AwakenProductLogo` 使用：应用内默认 `mark`，完整 App Icon PNG 仅用于 `icon`；Desktop 和其他 consumer 不保留或注入第二份 Awaken 图标。
- Desktop 应优先通过 `@awaken/design-system` 包出口使用组件，不要从 `Awaken Design System/src/...` 做源码路径 import。
- 独立组件必须能通过自己的 `#/components/<component-id>` URL 打开；归属于组件族的小组件必须在组件族 URL 中可查看和交互。

## 验证

修改组件或 tokens 后运行：

```bash
npm --workspace @awaken/design-system run typecheck
npm --workspace @awaken/design-system run smoke:render
```

`smoke:render` 会从 Gallery-only bundle server-render 所有 `Awaken*Demo` 组件，同时继续用生产 bundle 检查正式组件状态。如果 demo 缺失、渲染失败，或生产/Gallery 构建边界失效，命令会失败。
