# 前端设计系统与 Workbench 优化进展复核

> 复核日期：2026-07-19
>
> 复核范围：`f72ab9fd`、`910376a3`、`9b3bc07e`、`0135d7d6` 四个已落地提交
>
> 文件定位：这是「前端设计系统与 Workbench 优化」的阶段交接文档，只说明已完成的第一步和下一步执行方向，不替代 `docs/` 中的产品、架构与验收权威。

## 一、阶段结论

第一步已经完成。这一步不是重做 Workbench，而是先建立可继续迭代的视觉基础和交互稳定性基线：

- Awaken 已拥有清晰的 skin / skeleton token 边界，视觉改动与会影响测量、滚动和定位的结构改动不再混在一起。
- 浅色与深色的中性表面、文字、边框、阴影和状态色已完成第一轮语义收敛，关键小字、状态和控件的可读性也已纳入设计系统约束。
- Composer、项目树、新建任务、用户 prompt、滚动条和图表等现有组件已开始统一消费语义 token，不再依靠各自的临时色值和阴影。
- Workbench 的 streaming follow、被动布局变化和 native tail 处理已在 scroll controller / 纯决策 owner 内稳定，为后续视觉与几何优化提供了可信的交互底座。
- Mermaid 的 inline、展开、缩放、拖动、复位、重开与多实例渲染已有组件测试和独立的真实 Electron 原生指针回归。

因此，当前任务不再是「继续收掉第一步的修复尾项」，而是正式进入第二步：**从基础一致性转向 Workbench 核心场景的整体体验优化。**

## 二、第一步完成了什么

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

在这些不变量内，streaming 内容增长、被动高度变化和恢复 follow 的行为已有更确定的 controller 与测试基线。后续才能安全地处理信息层级、密度、间距和状态表达。

### 2.4 高风险图表交互已转为可持续回归

Mermaid 不再依赖临时个人会话或一次性脚本验证。当前仓库已同时保留：

- 组件与 jsdom 回归，覆盖多实例渲染、缓存身份和 pan 状态计算；
- 使用隔离 userData 的 Electron Gallery fixture；
- 通过原生鼠标事件验证 inline 滚动边界、100% / 400% 拖动、reset、close / reopen 的仓库内回归入口。

这部分现在是设计系统的稳定能力，不再是下一阶段的主任务。

### 2.5 四个提交的阶段分工

| 提交 | 阶段作用 |
| --- | --- |
| `f72ab9fd` `feat(frontend): refine visuals and stabilize workbench` | 建立 skin / skeleton 边界，完成第一轮语义视觉收敛，稳定 Workbench follow 与 Mermaid 交互 |
| `910376a3` `fix(design-system): close visual accessibility gaps` | 收敛对比度、可访问名称、主题与作者规则 |
| `9b3bc07e` `fix(mermaid): isolate cached svg instances` | 建立 Mermaid 多实例唯一身份与对应回归 |
| `0135d7d6` `test(mermaid): add real Electron interaction regression` | 把 Electron 原生事件链验收收敛成隔离、可重复的仓库测试资产 |

## 三、下一步：完成三个 Workbench 黄金场景

下一步不应以「继续找全仓零散色值」为主线，也不应启动新的 Workbench 架构重写。主线应改为：**用现有产品能力做好新建任务、任务执行中、任务完成与交付物三个高频真实场景。**

### 3.1 黄金场景 A：新建任务

目标是让用户一进入 Workbench 就知道「我现在可以做什么、在哪里输入、当前将使用什么上下文」。

本场景优先优化：

- `TaskWelcome` 与 center composer 的主次关系、垂直重心和空间平衡；
- 文本输入、附件、Skill token、模型与 workspace 信息的层级；
- 默认、hover、focus、有草稿、有附件和 token-only 等已支持状态；
- 左侧导航与中间空态的视觉层级，但不在本阶段新增项目筛选或 Context picker 能力。

这是第二步的第一个实施切片：状态较少、交互风险最低，适合先确立后两个场景共用的排版、间距、表面和控件语言。

### 3.2 黄金场景 B：任务执行中

目标是让用户能迅速分辨「我的要求、Agent 正在做的事、当前状态和已经产生的内容」，而不是看到平铺的日志。

本场景优先优化：

- prompt header、thinking / exploration、tool trace、streaming answer 之间的层级和阅读密度；
- running、waiting、error、cancelled 等已支持状态的一致表达；
- 多行 composer、return-to-latest、marker rail 与运行内容的安全间距；
- 在保留 follow / paused、prompt anchor 和单一 native tail 的前提下，改善长任务的节奏和可扫读性。

该场景是 Workbench 的核心，任何 skeleton token、DOM 层级、overflow、position 或 composer 几何改动都必须单独审查，不得作为「顺手调样式」混入。

### 3.3 黄金场景 C：任务完成与交付物

目标是让任务结果成为视觉重心，并让用户清楚地进入「阅读结论、打开交付物、继续追问」。

本场景优先优化：

- final answer 与已折叠「执行过程」的主次关系；
- 交付物标题、文件类型、数量、打开 / 展开动作和终态提示的信息密度；
- 无交付物、单交付物和多交付物等已支持组合；
- final answer、deliverables 和 dock composer 之间的结束感与后续行动关系。

本阶段只优化已实现的交付物展示与安全打开入口，不把尚未接线的右侧 Inspector 当作现有能力。

## 四、第二步的实施顺序

### 4.1 用真实 Desktop 建立三场景当前基准

在修改样式前，先用可删除的测试 thread / workspace 准备三个真实状态，记录当前 HEAD、主题、窗口尺寸和截图。观察结果放在任务上下文或临时验收目录，不新建第二套长期产品规范。

这一轮先回答三个问题：

1. 每个场景的第一视觉焦点是否正确；
2. 用户是否能在不阅读所有内容的情况下理解当前阶段；
3. 现有组件差距应由 token、局部组件，还是 Desktop 场景编排 owner 解决。

### 4.2 按 A → B → C 做竖向切片

每个场景都完成一个闭环再进入下一个：

1. 在现有 Design System 组件和 states 中定义或补齐可观测的 UI 状态；
2. 优先修改语义 token 和现有 `Awaken*` 组件，确实需要几何变更时再单独进入 skeleton / Workbench 契约复核；
3. Desktop 只负责组合 props、callbacks、slots 和场景级样式，不把 runtime 或滚动策略下沉到 Design System；
4. 在 Gallery 检查组件状态，再在真实 Awaken Desktop 完成场景验收；
5. 如果改变产品行为、owner、状态或验收基线，同一任务更新对应唯一权威文档。

### 4.3 三场景通过后再做横向扩展

三个黄金场景共用的视觉语言稳定后，再把已验证的 token 和组件模式扩展到 Settings、连接器、Skills 和其他页面。不在黄金场景尚未定型时进行全仓扫描式重绘。

## 五、第二步的范围边界

### 本阶段包含

- 已实现三场景的信息层级、排版、表面、状态、密度和局部几何优化；
- 相关 `Awaken*` 组件、states、demo、语义 token 与 Desktop 场景样式；
- light 主目标、dark 对等性与必要的响应式复核；
- 适用的自动化、真实 Desktop 动态验收与当前 HEAD 截图。

### 本阶段不包含

- 重新打开已闭环的 Mermaid、对比度或回归脚本修复，除非下一阶段实际产生新回归；
- 用第三种 composer variant、第二个滚动容器或新 overlay 坐标系重做 Workbench；
- 新增 Inspector、Context picker、run 中队列或新 runtime / protocol 状态；
- 用外部 UI 库替换 Awaken Design System 的组件所有权；
- 脱离三个黄金场景的全仓硬编码色值清理或大规模 CSS 整理。

## 六、第二步的完成标准

只有同时满足以下条件，才能宣称三个黄金场景完成：

1. 新建任务、执行中、完成与交付物都已在真实 Awaken Desktop 中准备和人工复核，不用 Gallery 冒充产品验收。
2. 三个场景的主视觉焦点、状态层级和下一行动清晰，且共用同一套 token 和组件语言。
3. Design System 仍保持 UI-only，Desktop 仍拥有业务编排、滚动、runtime 和 Electron 边界。
4. Workbench 的 `RunStage`、`ExecutionFeed`、`center` / `dock`、prompt anchor、follow / paused、overlay 和 composer 测量不变量没有被绕过。
5. 适用的自动化、UI smoke、Desktop typecheck / test 和 [Workbench 验收规范](docs/01_桌面工作台/中间交互区域/验收规范.md) AC 在本次工作区实际通过。
6. 三态当前 HEAD 截图与 manifest 完整；涉及滚动、follow、sticky 或 composer 测量的改动同时有录屏或连续关键帧。
7. 如果产品行为、owner、状态机或验收基线发生变化，已同步更新对应唯一权威，没有在本文档中建立第二套规范。

建议下一个实施批次从「黄金场景 A：新建任务」开始，完成它的当前 Desktop 基准、Design System 状态和真实 Desktop 验收后，再进入执行中场景。
