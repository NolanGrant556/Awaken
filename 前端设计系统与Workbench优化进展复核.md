# 前端设计系统与 Workbench 优化进展复核

> 复核日期：2026-07-19
>
> 复核对象：`c462d63d` 之后尚未提交的当前工作树
>
> 文件定位：这是本次任务的进展审计报告，不替代 `docs/` 中的产品、架构和验收权威。

## 一、结论

当前已经从“设计系统第一步”推进到了“提交前收口”阶段：

- 皮肤变量与骨架变量已经分层，暖调中性色、状态色、阴影和滚动条的第一轮视觉收敛基本完成；骨架 token 的数值没有在这一轮被修改。
- Workbench 流式输出跳动的两条主因已经在正确 owner 内修复，并补了状态机文档与自动化测试。
- Mermaid 的展开、缩放、自由拖动和双滚动条问题已经完成实现；用户已确认主要故障消失，上一轮也留下了真实 Electron/CDP 通过记录。
- 本次重新执行的 UI smoke、UI/Desktop typecheck 和 Desktop 全量测试均通过。

但这批改动现在还不应被描述为“全部闭环”，也不建议马上继续扩大视觉改造范围。提交前至少要处理一个确定性正确性问题：**按 Mermaid 源码复用完整 SVG 会让重复图表产生重复 DOM ID**。另外，真实 Electron 回归脚本、当前 HEAD 的 Workbench 动态证据和三态视觉 baseline 尚未进入可持续的仓库资产。

因此，下一步不是继续调字号、间距或结构，而是先把当前批次收口。

## 二、现在进展到哪里

| 范围 | 当前状态 | 判断依据 |
| --- | --- | --- |
| Design token 分层 | 基本完成 | [DESIGN_SYSTEM_GUIDE.md](<Awaken Design System/DESIGN_SYSTEM_GUIDE.md>) 和 [tokens.css](<Awaken Design System/src/tokens/tokens.css>) 已明确 skin/skeleton；本次只改了颜色、透明度和阴影色基，没有改 spacing、radius、font metrics、layout width 数值 |
| 浅色/深色视觉色板 | 第一轮完成，产品确认未完全闭环 | 暖调 neutral、状态色族、surface、border、shadow 已同时覆盖 light/dark；当前只完成了有限真实窗口观察，尚无 light/dark/system 的完整对照证据 |
| 组件视觉消费 | 已完成一批，尚未全仓收口 | Composer、ProjectTree、UserPrompt、共享滚动条和多处边框/阴影已改用语义 token；产品层仍有文件类型色、连接器品牌色、宠物状态色等硬编码或受控例外待分类 |
| Workbench 流式稳定性 | 实现与自动化完成，真实 AC 待补齐 | [useRunStageScrollController.ts](apps/desktop/src/renderer/useRunStageScrollController.ts) 已处理异步高度变化 re-pin 和被动 scrollHeight 收缩；[交互状态机与滚动布局.md](docs/01_桌面工作台/中间交互区域/交互状态机与滚动布局.md) 已同步；新增 4 条相关回归覆盖 |
| Mermaid 拖拽/缩放 | 主故障已解决，仍有缓存缺陷 | 原生 `pointerdown` 绕过真实 Electron 中失效的 React 合成事件；100% 与放大后的拖拽已由用户和上一轮 CDP 记录确认，但 SVG 跨实例缓存存在重复 ID 风险 |
| 自动化 | 本次通过 | 69 个 Design System 组件 smoke；Desktop 50 个测试文件、482 个测试全部通过；UI/Desktop typecheck 和 `git diff --check` 通过 |
| 真实 Desktop | 可正常启动，完整验收未完成 | 本次按正式入口启动了 `Awaken Desktop`，窗口为 `1442×900`，只读观察到 center 空态和新色板正常渲染，随后恢复为关闭状态；本次没有重新执行完整 streaming/Mermaid 动态旅程 |

复核前的工作树包含 15 个已跟踪修改文件和 1 个未跟踪测试文件，tracked diff 为 506 行新增、196 行删除。它实际混合了三种不同性质的工作：

1. 纯视觉 token/组件收敛；
2. Workbench 滚动状态行为修复；
3. Mermaid 渲染、缓存和 pointer 交互修复。

这说明最初“先换皮肤”的方向是对的，但当前工作树已经不再是单纯的 skin 批次，后续评审、验收和提交需要按三类风险分别处理。

## 三、现在还存在什么问题

### P1：Mermaid SVG 缓存会复用内部 ID

[AwakenMermaidDiagram.tsx](<Awaken Design System/src/components/AwakenMermaidDiagram/AwakenMermaidDiagram.tsx>) 第 49–89、318–352 行用 `chart` 文本作为 key，缓存并复用最终 SVG 字符串。Mermaid 会把每次调用的 render ID 写进 SVG 的 wrapper、selector、marker、`defs` 和 `url(#...)` 引用；同一张图在同一页面出现两次时，第二个组件会直接拿到第一份 SVG，因而出现重复 DOM ID 和跨图引用风险。

这不是纯理论问题：本次代码审计确认 Mermaid 的输出包含实例 render ID。当前新增测试中的“remount 不再次 render”还把这个不安全策略固化成了预期行为，却没有覆盖“两张相同图同时存在”。

最小处置方向：缓存必须具有图实例身份，或者每个挂载实例仍生成唯一 SVG/内部 ID；同时新增“两张相同 Mermaid 同屏”的回归测试。不要只把 `chart + theme` 拼进 key，因为那仍不能区分两个同时存在的实例。

### P1：真正能捕获故障的 Electron 回归仍是临时文件

真实 Electron 事件分发差异正是这次 Mermaid 连续误判的根因，但能复现它的 CDP 脚本仍位于 `/tmp/mermaid-pan-e2e.mjs`。它不受版本控制、不会随仓库分发，也不会成为提交前门禁。

仓库内的 [AwakenMermaidDiagramPan.test.tsx](apps/desktop/src/renderer/AwakenMermaidDiagramPan.test.tsx) 当前还是未跟踪文件，而且运行在 jsdom 中。它能验证 pan 状态计算，却不能证明真实鼠标 `pointerdown` 会经过 Electron 的原生事件链。

因此当前状态是“这次修好了”，还不是“以后改坏会自动发现”。

### P1：Workbench 当前 HEAD 的验收证据仍未闭环

[验收规范](docs/01_桌面工作台/中间交互区域/验收规范.md) 要求 scroll/follow/ResizeObserver 等动态行为提供真实 Desktop 录屏或连续关键帧，并要求视觉变更提供当前 HEAD 截图与 manifest。现有 [baseline manifest](docs/01_桌面工作台/中间交互区域/验收基线/manifest.json) 仍明确标记为 `legacy-unverified`、`requiresRecapture: true`。

本轮至少还需要补：

- AC-007：异步 Mermaid/SVG 高度变化时，用户上滚后的 paused 视图不被拉回；
- AC-008：明确恢复后回到唯一 native tail，后续继续 follow；
- AC-010：Mermaid 增长靠近底部时，输出与多行 composer 的安全间距稳定；
- center/dock/marker 三态的当前 HEAD 截图与完整 manifest；
- light/dark，必要时 follow-system 的真实视觉检查。

用户对 Mermaid 主故障的确认是有效验收信号，但不能替代上述 Workbench 合同规定的整套动态证据。

### P1：浅色 warning 小字对比度不足

[tokens.css](<Awaken Design System/src/tokens/tokens.css>) 当前 `--aw-status-waiting-text: #b05e00` 叠在 `rgba(226, 121, 0, 0.12)` 的浅色背景上，按当前浅色 surface 计算约为 `4.17:1`。它实际用于 12px runtime pill 和 11px A2UI warning badge，低于普通文本常用的 `4.5:1` 目标。

这也说明“skin token 不改变滚动几何”是正确的，但不等于“任意颜色变化都天然安全”。皮肤层仍必须做对比度、主题和真实视觉复核。

### P2：规则文档还有两处边界不够准确

- [DESIGN_SYSTEM_GUIDE.md](<Awaken Design System/DESIGN_SYSTEM_GUIDE.md>) 把 skin token 写成“may be iterated freely”。更准确的含义应是“不改变布局几何”，而不是免除对比度、可读性、overlay 和主题验收。
- 同一文件和 [Awaken Design System README](<Awaken Design System/README.md>) 仍把 `npm run dev:desktop` 写成正式 Desktop 入口；根 README 的当前正式入口是 `npm run open:desktop`，文档之间存在入口漂移。

### P2：`moduleResolution` 改动缺少本任务内理由

[Awaken Design System/tsconfig.json](<Awaken Design System/tsconfig.json>) 把 `moduleResolution` 从 `Node` 改为 `Bundler`。`Bundler` 对 Vite 项目本身合理，但它不属于视觉或 Mermaid 行为修复的必然部分；本次额外用 `Node` override 运行开发 typecheck 和声明构建也都通过。

提交前应二选一：给出明确的构建兼容理由并作为独立 tooling 变更提交，或从本批次移除。不要让无解释的工程配置变更混在 UI 修复里。

### P2：React 合成 `pointerdown` 失效的根因仍未知

当前原生 listener 是合理且已验证的窄修复，但仓库里 sidebar/inspector resize 和 pet overlay 仍使用 React `onPointerDown`。目前没有证据说明它们也坏了，也没有证据说明问题只会发生在 Mermaid portal/overlay 链路。

后续应做一次有限 blast-radius 复核，不应立即全仓改成原生事件。

### P2：流式 rAF 节流仍只是性能候选，不是已确认缺陷

上一轮提到的 O(n²) 或 rAF throttling 尚无本次 Performance trace、长会话基准或丢帧数据。现在不应直接实现；先测量 chunk 频率、commit 次数和长消息帧时间，再决定是否需要批处理。

## 四、下一步应该做什么

### 第一步：收掉当前正确性风险

1. 修正 Mermaid 缓存的实例唯一性，并增加“两张相同图同屏”的测试。
2. 把 `AwakenMermaidDiagramPan.test.tsx` 纳入版本控制。
3. 修正浅色 warning 文本对比度，并复核 dark 对应值。
4. 明确 `tsconfig.json` 的 `moduleResolution` 是否属于本任务；不属于就移出本批次。
5. 把 skin token 的文档表述改为“几何安全但仍需视觉/无障碍验收”，并统一正式启动入口。

### 第二步：把真实 Electron 验收变成可重复资产

将 `/tmp/mermaid-pan-e2e.mjs` 收敛成仓库内维护的 Electron/CDP 回归入口，使用可删除 fixture thread，不依赖个人真实业务会话。至少覆盖：

- 展开、100% 四向拖动；
- 400% 缩放后反向拖动；
- reset、关闭、重新展开；
- 两张相同 Mermaid 同屏；
- inline 不出现双滚动条；
- 切 thread 后回到图表不出现明显 loading/高度跳变。

这条真实运行测试和 jsdom 单元测试应同时保留：前者防事件链回归，后者快速验证状态计算。

### 第三步：完成 Workbench 当前 HEAD 验收

按 [验收规范](docs/01_桌面工作台/中间交互区域/验收规范.md) 执行 AC-007、AC-008、AC-010 以及受视觉变更影响的 center/dock/marker 检查，保存动态证据，并原子更新三张 baseline 与 manifest。任何一项不符合契约时停止替换 baseline，报告差异。

### 第四步：按风险拆分交付

建议至少拆为三个可独立审查和回滚的提交：

1. Design System 纯视觉 token 与组件色值收敛；
2. Workbench 流式 follow/passive-layout 修复、测试和状态机文档；
3. Mermaid 展开/pan/cache 修复、单元测试和真实 Electron 回归。

`moduleResolution` 如确需保留，单独放入 tooling 提交并写清原因。

### 第五步：再继续下一轮纯视觉优化

当前批次闭环后，再做剩余硬编码色的分类收敛：文件类型色、连接器品牌色、宠物 level/focus 色应升级为明确语义 token，或登记为受控品牌/功能例外。仍然只动颜色、边框色、阴影色和状态视觉，不动字号、行高、padding、margin、overflow ownership、DOM 层级和 Workbench 定位结构。

## 五、本次实际验证记录

本次复核实际执行：

```text
npm run smoke:ui
  通过：69 components

npm run typecheck:ui
  通过

npm --workspace @awaken/desktop run test
  通过：50 test files / 482 tests

npm run typecheck:desktop
  通过

git diff --check
  通过

npx tsc --noEmit --moduleResolution Node
npx tsc -p tsconfig.build.json --moduleResolution Node --noEmit
  均通过，用于判断 tsconfig 改动是否为当前代码的必要条件

npm run open:desktop
  通过：Awaken Desktop 1442×900，center 空态正常渲染

npm run close:desktop
  通过：screen 与相关进程已停止，恢复复核前状态
```

本次没有重新运行 `/tmp/mermaid-pan-e2e.mjs`，也没有把 center 空态截图写成正式 baseline；因此本报告把 Mermaid 的用户确认、上一轮 CDP PASS、本次自动化和本次只读 Desktop 观察分开陈述，没有把它们合并成“完整 Desktop 验收已通过”。
