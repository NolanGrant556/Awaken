# 前端设计系统与 Workbench 优化进展复核

> 更新日期：2026-07-19
>
> 当前范围：三步代码实现均已落地；第三步真实 Desktop 视觉复核由 Nolan 后续完成
>
> 文件定位：这是「前端设计系统与 Workbench 优化」的阶段交接文档，负责说明每一步的目标、边界、验证状态和涵盖提交，不替代 `docs/` 中的产品、架构与验收权威。

## 一、三步总览

| 步骤 | 状态 | 核心目标 | 涵盖提交 |
| --- | --- | --- | --- |
| 第一步：设计系统基础与 Workbench 稳定基线 | 已完成 | 建立 skin / skeleton 边界，完成第一轮视觉语言收敛，稳定 Workbench follow 与 Mermaid 真实 Electron 交互 | `f72ab9fd`、`910376a3`、`9b3bc07e`、`0135d7d6` |
| 第二步：skin / token 语义收敛 | 已完成 | 让连接器、扩展、模型设置和 pet overlay 的目标颜色回归语义 token，同时保持原有几何与交互 | `a874b665` |
| 第三步：前端视觉细节精修 | 代码实现完成，待视觉复核 | 保留现有交互结构与逻辑，系统调整颜色、字体、圆角、图标 / 图案、边框阴影、尺寸与状态细节 | `47319757`、`ba7bf38e`、`4d6c6463` |

前两步建立的稳定性、组件结构与 Workbench 交互逻辑是第三步的正确基线。第三步不重排内容、不重构组件形态、不改变交互流程；所有改动只服务于现有界面的颜色、字体、圆角、图案、尺寸、边框、阴影和视觉状态完成度。

下文提交表只统计实际实现和回归资产。`8676ffde`、`a7388a4e` 等纯交接文档维护提交不重复计入实现步骤。

## 二、第一步：设计系统基础与 Workbench 稳定基线（已完成）

### 2.1 设计系统的迭代边界已经建立

[Awaken Design System Guide](<Awaken Design System/DESIGN_SYSTEM_GUIDE.md>) 现在明确区分：

- skin token 负责颜色和阴影色值，不参与布局测量，但仍必须通过主题、真实视觉和无障碍验收；
- skeleton token 负责间距、字体度量、圆角、控件尺寸和布局宽度；修改它们等同于交互变更，必须进入 Workbench 中间区域复核与真实 Desktop 验收；
- `Awaken Design System` 继续保持 UI-only，产品状态、runtime、Electron 和滚动决策仍由 Desktop 拥有。

这些边界让后续优化可以清楚地回答「改的是视觉语言、组件结构，还是 Workbench 交互几何」。

### 2.2 第一轮视觉语言已经统一

当前已形成一套可继续迭代的 Awaken 视觉基底：

- 以暖调中性表面建立页面层级，减少无差别纯白、冷灰和临时透明色的混用；
- success / warning / error / running 各自回归单一基色家族，文字、背景、边框和 diff 状态不再互相脱节；
- 菜单、弹层、composer、控件边框和滚动条开始共享语义 token；
- light 是主设计目标，dark 保持对等语义和可读性，不另建一套无关的色板。

这一步完成的是「视觉语言的底座」，不代表所有界面已经完成最终视觉设计。

### 2.3 Workbench 已具备继续优化的稳定基础

本轮没有把 Workbench 当作普通聊天容器重排，而是保留了 [架构契约](docs/01_桌面工作台/中间交互区域/架构契约.md) 规定的：

- `RunStage` 唯一滚动、定位、overlay 和 composer ownership；
- `ExecutionFeed` 只承载 narrative / turn；
- composer 只保留 `center` / `dock` 两种形态；
- prompt anchor、follow / paused、return-to-latest 和 composer 测量共用同一坐标与滚动体系。

在这些不变量内，streaming 内容增长、被动高度变化和恢复 follow 的行为已有更确定的 controller 与测试基线。后续只在不改变这些行为的前提下精修视觉细节。

### 2.4 高风险图表交互已转为可持续回归

Mermaid 不再依赖临时个人会话或一次性脚本验证。当前仓库已同时保留：

- 组件与 jsdom 回归，覆盖多实例渲染、缓存身份和 pan 状态计算；
- 使用隔离 userData 的 Electron Gallery fixture；
- 通过原生鼠标事件验证 inline 滚动边界、100% / 400% 拖动、reset、close / reopen 的仓库内回归入口。

这部分现在是设计系统的稳定能力，不再是下一阶段的主任务。

### 2.5 第一步涵盖提交

| 提交 | 阶段作用 |
| --- | --- |
| `f72ab9fd` `feat(frontend): refine visuals and stabilize workbench` | 建立 skin / skeleton 边界，完成第一轮语义视觉收敛，稳定 Workbench follow 与 Mermaid 交互 |
| `910376a3` `fix(design-system): close visual accessibility gaps` | 收敛对比度、可访问名称、主题与作者规则 |
| `9b3bc07e` `fix(mermaid): isolate cached svg instances` | 建立 Mermaid 多实例唯一身份与对应回归 |
| `0135d7d6` `test(mermaid): add real Electron interaction regression` | 把 Electron 原生事件链验收收敛成隔离、可重复的仓库测试资产 |

## 三、第二步：skin / token 语义收敛（已完成）

> 实现完成并于 2026-07-19 提交。提交前曾通过正式入口两次打开同一份实现内容，由 Nolan 直接查看并在未报告阻塞问题后允许关闭。由于本轮没有保留截图，也没有逐项记录 light / dark 检查结果，本文只记录「用户已查看、未报告阻塞」，不把它扩写成完整视觉验收基线。
>
> 此前的「三黄金场景」follow-up 方案已被否决，不作为本阶段依据。

### 3.1 方向

继续收敛设计系统。主线是 skin / token 层面的语义收敛，不做 Workbench 场景重排，也不走三场景竖向切片。

### 3.2 已完成实现

本轮按 A → C → B 完成：

1. **状态色回归语义 token**
   - CLI 连接器详情的 connected / waiting 状态、扩展卡片 action-ready、模型启用开关与凭证不可读状态已统一消费 `--aw-status-success*`、`--aw-status-waiting*`、`--aw-status-danger*`；
   - 删除确认按钮新增 `--aw-status-danger-foreground` / `--aw-status-danger-hover`，light / dark 分别由 token 源决定前景与 hover，不再在 renderer 内混入 `#fff` / `#000`；
   - 状态色只改变 skin，不改变控件尺寸、布局或状态逻辑。
2. **基础色与 fallback 同步**
   - 连接器图标底色改用 `--aw-surface-raised`；
   - pet overlay 的 5 处文字 fallback 已与当前暖调文字 token 对齐：primary `#1c1917`、secondary `#44403c`、tertiary `#78716c`；
   - pet overlay 仍是独立透明浮窗，没有改变其窗口、主题或交互 ownership。
3. **品牌色与专用 accent 命名化**
   - 钉钉图标的固定品牌蓝收敛为 `--aw-brand-dingtalk`，明确不随 Awaken light / dark 主题改色；
   - 模型 context 进度条的 `#168fd2` / `#9fdbf6` 实际不是第三方品牌标识，分别收敛为 `--aw-context-meter-indicator` / `--aw-context-meter-fill`，避免把产品控件错误命名为钉钉品牌；
   - 这组命名化保持原色值，不在本轮扩大为视觉重设计。

### 3.3 修改 owner 与边界

- `Awaken Design System/src/tokens/tokens.css`：新增状态、钉钉品牌与 context meter skin token；
- `apps/desktop/src/renderer/styles.css`：场景消费者改用语义 token；
- `apps/desktop/src/renderer/pet-overlay/pet-overlay.css`：只同步 5 处文字 fallback；
- 未改 DOM / React 组件层级、overflow、position、间距、字体度量、圆角、控件尺寸或布局宽度；
- 未触碰 `RunStage`、`ExecutionFeed`、prompt anchor、follow、composer 测量和 Workbench 滚动不变量。

### 3.4 本轮代码级验证

- `npm run build:ui` 通过；
- `npm --workspace @awaken/design-system run typecheck` 通过；
- `npm --workspace @awaken/design-system run smoke:render` 通过，69 个组件完成 SSR smoke；
- `npm --workspace @awaken/desktop run typecheck` 通过；
- `npm --workspace @awaken/desktop run test` 通过，52 个测试文件、488 个测试通过；
- renderer 目标文件中的本章旧裸色值扫描无残留，`git diff --check` 通过。

这些结果只证明代码、类型、构建和组件渲染基线，没有替代真实 Desktop 视觉验收。

### 3.5 真实 Desktop 查看状态

- 本轮通过 `npm run open:desktop` 两次启动提交前同一份实现，窗口均成功打开；
- Nolan 完成直接查看后回复「可以了」，并要求通过 `npm run close:desktop` 关闭；
- 两次关闭均确认受管理 screen 与相关 Dev 进程停止，独立安装版未被处理；
- 未保存当前 HEAD 截图，未记录连接器、模型设置、删除确认、pet overlay 的逐项 light / dark 结果；这是第二步未完整留档的视觉证据，不再自动扩大为第三步的主线任务。

### 3.6 第二步涵盖提交

| 提交 | 阶段作用 |
| --- | --- |
| `a874b665` `refactor(frontend): finish skin token convergence` | 在 `tokens.css`、Desktop 场景样式和 pet overlay fallback 中完成状态色、基础色、钉钉品牌色与 context meter token 收敛；该提交只包含 3 个代码文件，没有混入其他线程的文档改动 |

## 四、第三步：前端视觉细节精修（代码实现完成，待真实 Desktop 视觉复核）

### 4.1 方向

当前 Workbench 的交互逻辑、内容归属、状态表达和结构组合是正确设计，不是第三步的优化对象。

本步所说的「优化前端」只指视觉细节精修：不改界面在做什么、内容出现在哪里、用户如何操作，只提升现有界面的颜色准确度、排版质感、形状一致性、图形完成度和尺寸精度。

### 4.2 已完成的视觉优化

| 视觉项 | 本轮实际调整 | 未改变的事 |
| --- | --- | --- |
| 颜色与主题 | light 改为更清楚的暖白画布、导航与 raised surface 层级；中性 fill / border 提高精度；dark 同步表面、边界与三级文字可见性；focus 使用统一 accent ring | 状态含义、状态映射和交互触发条件 |
| 字体 | assistant 正文建立 `14px / 22px` 阅读节奏和 `20 / 16 / 14px` 标题层级；代码块与表格提升到 `13px / 20px`；regular / medium / semibold / bold / heavy 字重角色明确为 `400 / 500 / 600 / 650 / 700` | 文本内容、信息顺序、折叠阈值和阅读逻辑 |
| 圆角与边缘 | 建立 compact / control / panel / composer 的 `6 / 8 / 12 / 16px` 层级；composer 从 18px 收到 16px；菜单、卡片、图表工具栏、模型选择器按组件角色消费对应圆角与边框 | 层级 ownership、overlay 定位、弹层打开 / 关闭方式 |
| 图标与图案 | 高频线性图标统一 `1.75` 光学线宽，常规图标为 16px、紧凑图标为 14px；覆盖 Workbench chrome、composer、项目树、Mermaid 工具栏与模型设置图标 | 图标功能、按钮语义、可访问名称和点击行为 |
| 阴影与状态 | 菜单、popover、toast、raised card、hover card、switch thumb 和 composer 使用统一暖黑环境光 / 接触阴影；Desktop 的冷灰硬编码阴影已移除；已有 hover / active / selected / focus / disabled 只改变视觉呈现 | 状态数量、触发条件、DOM 顺序、滚动逻辑和操作热区语义 |

这些改动针对的是原有界面层级偏平、长文正文偏小、标题层级不清、局部 `700 / 800` 过重、composer 光晕偏散、浮层阴影仍混用冷灰、焦点环不统一和线性图标光学规格分散等具体问题，不是单纯减少 CSS 字面量。

### 4.3 实际实现分层

1. `tokens.css` 负责 light / dark 色阶、语义阴影、focus ring、字重、阅读排版、圆角和图标光学规格；
2. `components.css` 与既有 `Awaken*/*.css` 负责 assistant 阅读层级、composer、Deliverable、项目树、Mermaid 与 sidebar action 的视觉消费；
3. Desktop `styles.css` / `styles/workbench.css` 只消费这些角色并收敛局部视觉值；
4. 本轮没有修改任何 `.tsx`、controller、projector、reducer、runtime、组件 API 或事件处理。

### 4.4 实际修改 Owner

- `Awaken Design System/src/tokens/tokens.css`、`components.css` 与 `DESIGN_SYSTEM_GUIDE.md`；
- `Awaken Design System/src/components/AwakenComposerBar/AwakenComposerBar.css`；
- `Awaken Design System/src/components/AwakenDeliverableCard/AwakenDeliverableCard.css`；
- `Awaken Design System/src/components/AwakenProjectTree/AwakenProjectTree.css`；
- `Awaken Design System/src/components/AwakenSidebarSectionAction/AwakenSidebarSectionAction.css`；
- `apps/desktop/src/renderer/styles.css` 与 `apps/desktop/src/renderer/styles/workbench.css`。

### 4.5 不可修改的边界

- 不改 `App.tsx`、`useRunStageScrollController.ts`、`workbenchPresentation.ts`、projector、reducer 或 runtime 代码；
- 不改 DOM / React 层级、内容顺序、组件组合、折叠方式、状态映射、组件 API 或事件处理；
- 不改 `RunStage` / `ExecutionFeed` ownership、唯一滚动容器、prompt anchor、follow / paused、marker、return-to-latest、overlay 和 composer 测量；
- 不新增 composer 形态、滚动容器、sticky 副本、中间定位层或新的界面状态；
- 不改焦点顺序、键盘语义、ARIA、点击热区语义和响应式折叠规则；
- 不以任何视觉精修理由突破以上边界。

### 4.6 本轮代码级验证

- `npm run build:ui` 通过；
- `npm run typecheck:ui` 通过；
- `npm run smoke:ui` 通过，69 个设计系统组件完成 SSR render smoke；
- `npm run typecheck:desktop` 通过；
- `npm --workspace @awaken/desktop run test` 通过，52 个测试文件、488 个测试通过；
- `git diff --check` 通过。

这些结果证明当前代码可构建、可通过类型检查、组件可渲染且 Desktop 现有自动化没有因视觉 CSS 改动回归，不等同于真实视觉验收。

### 4.7 真实 Desktop 视觉复核状态

按 Nolan 本轮要求，开发线程没有启动 Awaken，也没有生成当前 HEAD 的 light / dark、hover / focus 或前后对比截图。因此本章状态只能写为「代码实现完成，待真实 Desktop 视觉复核」，不能写成视觉验收完成。

后续复核仍应在同一窗口尺寸、主题和界面状态下检查：

- light / dark 的表面层级、正文可读性、状态语义、边界和图标可见性；
- hover、active、selected、focus、disabled 等已有视觉状态；
- assistant 长文、代码块、表格、composer、侧栏、Deliverable、Mermaid、扩展页和模型设置；
- 按 [复核清单](docs/01_桌面工作台/中间交互区域/复核清单.md) 确认 anchor、follow、composer、marker、return 和响应式行为没有因视觉尺寸变化而改变。

一旦发现交互行为、内容归属、状态逻辑或滚动测量变化，应撤回对应视觉值，不修改交互契约为视觉方案放行。

### 4.8 第三步涵盖提交

| 提交 | 阶段作用 |
| --- | --- | --- |
| `47319757` `style(design-system): refine color and typography` | 精修 light / dark surface、文字、边界、focus 与语义阴影；建立 assistant 阅读排版和字重角色 |
| `ba7bf38e` `style(design-system): refine shape iconography and sizing` | 建立 6 / 8 / 12 / 16px 圆角层级与 1.75 图标线宽，精修 composer、Deliverable、项目树和 Mermaid 视觉 |
| `4d6c6463` `style(desktop): apply visual polish without interaction changes` | 让 Desktop 扩展、设置、模型选择、toast、switch 和 Workbench 浮层消费统一字体、圆角、focus 与阴影角色 |
