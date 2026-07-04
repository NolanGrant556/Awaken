# Awaken Desktop Workbench Interaction PRD

## 1. 背景

Awaken Desktop 的中间工作区（Workbench Main Area）已经形成明确的产品方向：用户在这里把任务交给 Agent，并持续阅读 Agent 的执行过程、工具使用、审批请求、最终回答和上下文状态。这个区域不是普通聊天窗口，也不是日志面板，而是一个“执行中的任务叙事空间”。

过去多轮修复暴露出一个核心问题：中间区域的视觉与交互已经被用户确认是想要的方向，但其滚动、锚定、sticky prompt、composer 避让、latest-output follow 等算法仍不够系统。单点修复容易出现“修一个功能，破坏另一个已确认行为”的情况。

本 PRD 的目标是把用户对 Workbench 中间区域的需求系统化，作为后续重构 `useRunStageScrollController`、turn/feed 渲染层、验收护栏的产品依据。

## 2. 目标

1. 固化 Workbench 中间区域的产品行为，而不是只描述当前代码结构。
2. 明确 prompt bubble、turn、ExecutionFeed、dock composer、turn marker、scroll/follow 之间的交互关系。
3. 为后续算法重构提供可验收的需求边界。
4. 明确哪些行为必须通过真实 Awaken Desktop + Computer Use 验证。
5. 降低后续重构破坏现有设计的概率。

## 3. 非目标

- 本 PRD 不要求立刻重写 Workbench 前端。
- 本 PRD 不改变 `RunStage / ExecutionFeed / ComposerDock` 的 DOM 契约。
- 本 PRD 不新增第三种 composer variant。
- 本 PRD 不把 Desktop renderer 的 scroll/follow 策略下沉到 `@awaken/design-system`。
- 本 PRD 不引入 Playwright 或浏览器自动化作为动态交互验收手段。
- 本 PRD 不解决 markdown/assistant message 渲染质量问题，除非它直接影响中间区域布局和滚动。

## 4. 用户与使用场景

### 4.1 主要用户

- 本地使用 Awaken Desktop 的用户。
- 需要连续向 Agent 发起多轮任务的人。
- 会在执行中阅读历史、暂停自动 follow、回到底部继续观察的人。
- 会让 Agent 执行长任务、读文件、跑命令、申请审批、生成最终回答的人。

### 4.2 核心使用场景

- 用户在空态从 center composer 发起第一条任务。
- Agent 进入运行态后，composer 变为 dock 形态，ExecutionFeed 展示 turn。
- 用户连续发送多轮 prompt，每轮 prompt bubble 应成为该 turn 的视觉锚点。
- 用户在输出过程中向上滚动阅读历史，系统不应强行拉回底部。
- 用户向下滚回最新输出区域后，latest-output follow 应能恢复。
- 多 turn 后，turn marker rail 提供跳转和当前 turn 位置反馈。
- approval prompt、runtime panel、composer 高度变化时，输出区域不应被 dock composer 覆盖。

## 5. 产品原则

### 5.1 中间区现有设计是产品契约

空态 center composer、运行态 dock composer、ExecutionFeed turn 分组、sticky prompt header、turn marker、latest-output follow、用户滚动暂停/恢复，都是产品意图，不是临时实现。

### 5.2 Prompt 属于 Turn

用户 prompt bubble 必须位于对应 turn section 内，并与该 turn 的 Thought、工具轨迹、审批卡、文件变更、assistant answer 形成一个完整叙事单元。不能把 prompt header 拆成外部同步层或独立悬浮副本。

### 5.3 RunStage 是唯一中间区域容器

中间区域的滚动、定位、overlay、composer 都必须通过 `RunStage` 承载。`ExecutionFeed` 只展示 narrative / turn；composer 只通过 `RunStage` 的 composer slot 挂载。

### 5.4 用户意图优先于被动布局变化

用户主动滚动、触摸、点击 marker 时，系统必须尊重用户正在阅读历史的意图。ThoughtTrace 增高、markdown 渲染完成、composer 高度变化等被动布局变化，不能被误判为用户想恢复 auto-follow。

### 5.5 真实运行优先于静态等价

DOM 等价、静态截图、纯函数测试都只是辅助。凡是涉及 scroll、anchor、sticky、follow、composer 避让的改动，必须通过真实 Awaken Desktop + Computer Use 交互验收。

## 6. 信息架构与结构契约

### 6.1 中间区域主链路

```text
AppShell
  -> WorkbenchShell
    -> RunStage
      -> ExecutionFeed
        -> execution-turn-section
          -> execution-turn-header
            -> UserPromptBlock
          -> execution-turn-body
            -> narrative items
      -> ComposerDock
      -> turn marker rail overlay
```

### 6.2 Ownership

| 层级 | 代表 | 责任 |
|------|------|------|
| Design System | `AwakenRunStage`、`ComposerBar`、`UserPromptBlock` | 结构壳、视觉 token、可复用 UI |
| Desktop Renderer | `App.tsx`、`useRunStageScrollController.ts` | runtime 编排、DOM scroll、anchor、follow、用户意图、composer 生命周期 |
| Pure Decisions | `workbenchPresentation.ts` | 可测试的纯判断函数 |
| Acceptance | `capture-workbench-*`、Computer Use | 真实窗口截图和动态交互证据 |

## 7. 功能需求

### P0-1 空态 Center Composer

**用户意图**：打开 Awaken 或新建任务后，中间区域应聚焦在清晰、安静的任务输入入口。

**行为要求**：
- 无 narrative 且无 active run 时，显示 center composer。
- center composer 位于视觉中心偏上的位置，并与 `TaskWelcome` 一起构成空态。
- 此状态不应显示 dock composer、ExecutionFeed、turn marker rail。

**验收标准**：
- `npm run open:desktop` 后窗口 ready。
- center composer 可输入、可发送。
- `npm run capture:workbench -- --state center --no-open` 产出非白屏截图。

**状态**：已确认契约。

### P0-2 第一轮 Prompt 锚定

**用户意图**：发送第一条任务后，用户应立即看到自己刚发出的 prompt，且它成为当前 turn 的顶部视觉锚点。

**行为要求**：
- 第一条 prompt 提交后，center composer 切换为 dock composer。
- 最新 turn 的 prompt bubble 锚定到 RunStage 顶部。
- prompt bubble 下方显示该 turn 的 Thought / narrative / answer。
- dock composer 不遮挡 prompt 或 answer。

**验收标准**：
- 使用 Computer Use 输入并发送第一条短 prompt。
- 发送后截图中 prompt bubble 位于中间区顶部。
- 输入框进入运行态 stop mode，结束后恢复可编辑/发送。

**状态**：已确认契约。

### P0-3 多轮 Prompt 锚定与 Sticky 交接

**用户意图**：连续发送第二条、第三条任务时，最新 prompt 应成为新的顶部锚点；旧 turn 不应继续抢占 sticky 顶部。

**行为要求**：
- 新 run 开始时，sticky ownership 必须从旧 active turn 交接到 latest turn。
- 第二条 prompt 提交后，第二 turn 的 prompt bubble 应锚定到顶部。
- 第一 turn 的 prompt header 不得覆盖第一 turn 自己的 answer，也不得遮挡第二 turn 的内容。
- prompt bubble 为多行、长文本、可折叠状态时也必须成立。

**已知失败**：
- 连续发送两条相同多行 prompt 后，旧 prompt bubble 仍 sticky 在顶部，导致新 turn 置顶失效。
- 多行 prompt 使旧 sticky header 更高，遮挡更明显。

**验收标准**：
- 使用 Computer Use 连续发送两条多行 prompt，例如：
  ```text
  回复
  1
  2
  ...
  11
  ```
- 第二条发送后，最新 prompt bubble 必须可见并位于顶部锚点。
- 旧 turn header 不得覆盖旧 turn answer。
- 上下滚动后 active marker 与 sticky header 应一致。

**状态**：已确认需求，当前实现存在 bug。

### P0-4 Latest Output Follow

**用户意图**：当用户没有主动阅读历史时，Agent 输出增长应自动保持可见，最终回答不应停在不可见区域。

**行为要求**：
- latest turn 输出持续增长时，视图应跟随最新输出。
- follow 的目标应与真实 scroll bottom / visible tail 一致。
- ThoughtTrace、工具输出、assistant answer footer 增高后，follow 不应出现最终间距漂移。
- run 完成后，最终输出与 dock composer 之间保持稳定可读间距。

**验收标准**：
- 运行会产生多段 Thought / 工具输出 / final answer 的任务。
- 输出过程中，latest output 保持在可视区域。
- 完成后，final answer 不被 dock composer 覆盖。

**状态**：已确认契约。

### P0-5 用户滚动暂停 Follow

**用户意图**：用户向上滚动阅读历史时，Awaken 不应强行把视图拉回最新输出。

**行为要求**：
- wheel 向上、touch 向历史方向、pointer 操作、点击旧 marker 都视为用户主动阅读历史。
- 用户暂停 follow 后，后续 ThoughtTrace / markdown / tool output 增高不得自动恢复 follow。
- passive layout scroll 不得被误判为用户恢复 follow。

**验收标准**：
- run 过程中使用 Computer Use 向上滚动。
- 输出继续增长时，视图停留在用户阅读位置。
- 只有用户明确向下滚回 tail 或产生恢复意图后，follow 才恢复。

**状态**：已确认契约。

### P0-6 Dock Composer 避让

**用户意图**：dock composer 是输入区，不应覆盖 ExecutionFeed 的最终输出，也不应让滚动条视觉穿入输入框。

**行为要求**：
- RunStage 内容底部必须为 dock composer 预留稳定 safe area。
- composer 高度变化时，RunStage inset 同步更新。
- runtime panel、approval prompt、model pill、输入框高度变化都不能破坏输出避让。
- output tail、scroll bottom、composer inset 必须使用同一个布局事实，不能各算各的。

**已知失败**：
- final output 卡进输入框区域，无法通过滚动恢复正常间距。
- 滚动条视觉突破 composer dock。

**验收标准**：
- 完成一条长输出任务。
- final answer 与 composer dock 之间有稳定可读间距。
- 右侧滚动条停在 composer 上方，不穿过输入框区域。

**状态**：已确认需求，已有过修复，但仍需纳入系统性回归。

### P0-7 Turn Marker Rail

**用户意图**：多轮任务后，用户需要快速理解当前在第几轮，并能跳转到某一轮。

**行为要求**：
- turn 数达到阈值后显示 marker rail。
- marker rail 位于 RunStage overlay 内，不影响 ExecutionFeed 布局。
- active marker 与当前 sticky / active turn 一致。
- 点击 marker 后跳转到对应 prompt anchor，并暂停 latest follow。
- marker 在 turn 较多时可进入 compact / summary 模式。

**验收标准**：
- 连续发送至少三轮 prompt。
- marker rail 出现，且 active marker 对应当前 turn。
- 点击旧 marker 后跳转到对应 turn，sticky header 与 marker 同步。

**状态**：已确认契约。

### P1-1 Prompt Bubble 长文本与折叠

**用户意图**：长 prompt 或多行 prompt 不应破坏布局，用户可展开查看完整内容。

**行为要求**：
- 超过行数或长度阈值的 prompt bubble 默认折叠。
- 折叠高度必须参与 sticky/anchor 算法。
- 展开后不能遮挡自己的 answer 或下一个 turn。
- 点击外部后可恢复折叠。

**验收标准**：
- 发送多行 prompt。
- 折叠态、展开态、再次折叠后，sticky/anchor/follow 都正常。

**状态**：已确认需求，当前多行场景暴露 P0 bug。

### P1-2 Runtime Panel 与 Approval Prompt

**用户意图**：runtime 状态、错误、approval prompt 可以出现在 composer 附近，但不能改变中间区基本布局契约。

**行为要求**：
- runtime panels / approval prompt 位于 composer stack 内。
- 出现或消失时，composer bottom inset 应稳定同步。
- approval card 不应出现在 ExecutionFeed 中作为非 narrative 控件；历史 approval narrative 可作为 narrative item 展示。

**验收标准**：
- 触发 approval / runtime panel 场景。
- composer 变高后，final output 仍在 composer 上方。

**状态**：已确认契约。

### P1-3 Tool / Work Packet 叙事

**用户意图**：长任务读取大量文件或运行工具时，用户应看到“正在做什么”，但不被低层日志淹没。

**行为要求**：
- ExecutionFeed 展示 tool / Thought / file changes 等 narrative。
- 连续 Read / Search / Bash / MCP 操作未来应考虑聚合为 collapsed work packets。
- 判断 run 是否“卡住”时，应区分真实 runtime fault 和长时间 read-heavy research。

**验收标准**：
- 长任务中 stop button、context 增长、tool trace 增长可作为仍在运行的信号。
- UI 不把大量低层工具事件散乱堆满 feed。

**状态**：部分确认，聚合形态待设计。

### P1-4 响应式 Workbench

**用户意图**：窗口宽度变化、sidebar/inspector 折叠展开时，中间区域仍可稳定使用。

**行为要求**：
- sidebar / inspector 折叠或展开不应让 RunStage 出现异常空白。
- composer model pill、按钮、输入框不应错位。
- prompt bubble、turn body、dock composer 在窄窗口保持可读。

**验收标准**：
- 在正常宽度和窄宽度下各完成 center -> dock -> multi-turn 验收。

**状态**：已确认风险，需系统纳入验收。

### P2-1 自动化 VRT

**用户意图**：后续 PR 能稳定产出对比截图，减少人工漏看。

**行为要求**：
- 保留 center / dock / marker baseline。
- VRT 可产出 manifest。
- 未来可引入轻量像素 diff，但不得取代真实交互验收。

**验收标准**：
- `npm run capture:workbench:vrt` 能完成三态捕获。
- PR 附新截图与 baseline 差异说明。

**状态**：已部分完成。

### P2-2 结构性重构

**用户意图**：`App.tsx` 不再无限膨胀，但每一刀都必须可回滚、可验收。

**行为要求**：
- 重构顺序优先治理 scroll/anchor 算法，再做渲染层拆分。
- 拆 narrative/feed 前必须有动态交互护栏。
- 抽 hook / view 时不得改变 DOM key、wrapper、approval handler、ExecutionFeed 边界。

**验收标准**：
- 每次重构都有静态测试 + 真实 Desktop 交互证据。
- 出现交互破坏时能精确撤回该刀。

**状态**：已确认需求；narrative item 拆分已因真实验收失败撤回。

## 8. 关键状态模型需求

后续算法设计应至少显式建模以下状态，而不是散落在多个 ref / CSS 变量 / effect 中：

1. `currentTurn`：当前视觉上被 sticky / active marker 代表的 turn。
2. `latestTurn`：最新提交的 prompt 所属 turn。
3. `pendingPromptAnchor`：新 prompt 等待锚定到顶部。
4. `followLatestOutput`：系统是否正在自动跟随 latest output。
5. `pausedByUser`：用户是否主动退出 follow。
6. `resumeIntent`：用户是否明确表达恢复 follow。
7. `composerInset`：dock composer 实际占用和 safe area。
8. `promptBubbleMode`：prompt bubble 折叠 / 展开 / 多行高度状态。
9. `programmaticScroll`：系统主动滚动，避免被误判为用户滚动。

## 9. 验收策略

### 9.1 必须使用真实 Desktop

涉及中间区域布局、滚动、锚定、follow、composer、turn marker 的改动，必须使用：

```bash
npm run open:desktop
```

并确认窗口 ready、可见、尺寸足够。

### 9.2 必须使用 Computer Use

动态交互验收指定使用 Computer Use。不得用 Playwright 替代。浏览器打开 `127.0.0.1:5174` 也不能代表 Desktop 验收，因为 renderer 依赖 Electron bridge。

### 9.3 最小 P0 验收链路

1. 打开 Awaken Desktop，确认 center composer。
2. 发送第一条短 prompt，确认 prompt 置顶、dock composer 出现。
3. 发送第二条短 prompt，确认最新 prompt 置顶，旧 turn 不遮挡 answer。
4. 发送两条多行 prompt，确认多行 bubble 不破坏 sticky/anchor。
5. 发送第三条 prompt，确认 turn marker 出现。
6. 点击旧 marker，确认跳转与 active marker 同步，且 latest follow 暂停。
7. 运行长输出任务，向上滚动暂停 follow。
8. 输出继续增长时不自动拉回。
9. 向下滚回 tail，follow 可恢复。
10. run 完成后 final output 与 dock composer 保持稳定避让。

### 9.4 截图与 manifest

至少保留：

```bash
npm run capture:workbench -- --state center --no-open
npm run capture:workbench -- --state dock --no-open
npm run capture:workbench -- --state marker --no-open
```

涉及视觉变更时使用：

```bash
npm run capture:workbench:vrt
```

### 9.5 验收矩阵

| ID | 优先级 | 旅程 | 前置状态 | 操作步骤 | 预期行为 | 证据 |
|----|--------|------|----------|----------|----------|------|
| AC-001 | P0 | 空态启动 | Awaken Desktop 未运行或已关闭 | `npm run open:desktop` | 窗口 ready、center composer 可见、无 ExecutionFeed / dock composer | open:desktop 输出 + center 截图 |
| AC-002 | P0 | 第一轮短 prompt | center composer 可见 | Computer Use 输入短 prompt 并发送 | prompt bubble 锚定顶部，dock composer 出现，stop mode 生效 | Computer Use 截图 / 录屏 |
| AC-003 | P0 | 第一轮结束 | AC-002 run 结束 | 等待输出完成 | 输入框恢复可编辑，final answer 不被 composer 遮挡 | Computer Use 截图 |
| AC-004 | P0 | 第二轮短 prompt | 第一轮已完成 | 在 dock composer 输入第二条短 prompt 并发送 | 第二 turn prompt 置顶；第一 turn header 不压住自己的 answer | Computer Use 截图 |
| AC-005 | P0 | 多行 prompt 锚定 | 至少一轮已完成 | 连续发送两条多行 prompt | 最新多行 prompt 置顶；旧 sticky header 不抢占顶部 | Computer Use 录屏或关键帧 |
| AC-006 | P0 | Prompt 展开/折叠 | 已存在多行 prompt turn | 点击多行 prompt 展开，再点击外部折叠 | 展开/折叠不破坏 turn body 间距、sticky、anchor | Computer Use 录屏 |
| AC-007 | P0 | 用户上滚暂停 follow | run 正在输出 | 输出中向上滚动 | 视图停在历史位置，新增输出不强拉回底部 | Computer Use 录屏 |
| AC-008 | P0 | 用户恢复 follow | AC-007 后 | 向下滚回 latest output tail | follow 恢复，后续输出保持可见 | Computer Use 录屏 |
| AC-009 | P0 | 长输出 composer 避让 | dock composer 可见 | 运行长输出任务直到完成 | final output 与 dock composer 保持间距，滚动条不穿入输入框 | dock 截图 + Computer Use 截图 |
| AC-010 | P0 | Turn marker 出现 | 至少三轮 turn | 发送第三条 prompt | marker rail 出现，active marker 对应当前 turn | marker 截图 |
| AC-011 | P0 | Turn marker 跳转 | marker rail 可见 | 点击第一/第二 marker | 跳转到对应 prompt，sticky header 与 marker 同步，latest follow 暂停 | Computer Use 录屏 |
| AC-012 | P1 | runtime panel / approval prompt 高度变化 | 可触发 runtime panel 或 approval | 触发 panel/card 出现和消失 | composer inset 同步，输出不被覆盖 | Computer Use 录屏 |
| AC-013 | P1 | 响应式布局 | 正常窗口已通过 | 折叠 sidebar / inspector，或缩窄窗口 | center/dock/prompt/marker 不漂移，不出现异常空白 | 前后截图 |
| AC-014 | P1 | 重构行为保持 | 涉及 narrative/feed/composer/scroll 重构 | 跑静态测试后执行 AC-002 至 AC-011 | DOM/key/handler 变化不得破坏真实交互 | 测试输出 + Computer Use 证据 |
| AC-015 | P2 | VRT bundle | 已准备三态 | `npm run capture:workbench:vrt` | 产出 center/dock/marker 截图和汇总 manifest | manifest + 截图路径 |

### 9.6 当前覆盖与缺口

| 覆盖层 | 已有能力 | 缺口 |
|--------|----------|------|
| 纯函数单测 | `workbenchPresentation.ts` 多项决策函数已可测 | 缺少多行 prompt / active sticky 交接状态机测试 |
| 单状态截图 | `capture-workbench-acceptance.mjs` 可捕获 center / dock / marker | 只能拍当前窗口状态，不会自动操作 UI |
| VRT bundle | `capture-workbench-vrt.mjs` 可产出三态 manifest | 仍依赖人工准备状态，不验证动态行为 |
| 真实交互 | Computer Use 可操作真实 Electron 窗口 | checklist 仍需硬性要求具体链路和阻塞条件 |
| 静态 DOM 等价 | 可辅助检查渲染输出 | 无法证明 scroll / anchor / sticky / follow 正确 |

## 10. 测试覆盖要求

### 10.1 单元测试

必须覆盖：
- prompt anchor scrollTop 计算。
- latest turn min-height reserve。
- user wheel / touch pause follow。
- explicit resume intent。
- active turn 与 marker 采样。
- composer bottom inset。
- 多行 prompt bubble 高度参与 anchor 的纯决策边界。

### 10.2 交互级测试缺口

当前静态 DOM 等价测试无法证明：
- prompt bubble 是否真正置顶。
- sticky header 是否属于正确 turn。
- passive layout change 是否误触发 follow。
- composer 高度变化是否与 scroll tail 同步。

这些必须用 Computer Use 验收补齐。

## 11. 已知问题清单

| 编号 | 问题 | 严重级别 | 当前判断 |
|------|------|----------|----------|
| WBI-001 | 连续发送多行 prompt 后，最新 prompt 置顶失效 | P0 | 原始 sticky/active turn 交接 bug |
| WBI-002 | 旧 turn header 覆盖自己的 answer | P0 | active sticky ownership 与新 run anchor 脱节 |
| WBI-003 | final output 卡进 dock composer | P0 | composer inset / latest tail follow 同步问题 |
| WBI-004 | 用户暂停 follow 后被 passive layout 拉回 | P0 | 意图识别需显式化 |
| WBI-005 | DOM 等价测试无法发现动态锚定破坏 | P1 | 验收策略缺口 |
| WBI-006 | narrative/feed 拆分可能改变 React key / commit 时序 | P1 | 重构风险 |
| WBI-007 | 多行 prompt 折叠/展开高度未被系统性建模 | P1 | 算法模型缺口 |

## 12. 后续交付建议

### Phase 1：修复 P0 sticky/anchor 状态模型

- 目标：解决 WBI-001 / WBI-002。
- 验收：连续多行 prompt、两轮以上 run、旧 header 不遮挡、新 prompt 置顶。

### Phase 2：统一 composer inset 与 latest tail follow

- 目标：解决 WBI-003。
- 验收：长输出、runtime panel、approval prompt、composer 高度变化。

### Phase 3：补强 user intent / follow 状态机

- 目标：解决 WBI-004。
- 验收：用户向上滚动暂停、被动内容增长不恢复、明确向下回 tail 才恢复。

### Phase 4：建立 Computer Use 交互护栏

- 目标：解决 WBI-005。
- 验收：PR checklist 中明确动态验收；保留截图证据。

### Phase 5：恢复低风险结构重构

- 目标：继续治理 `App.tsx`，但必须在交互护栏之后。
- 验收：每一刀保持 DOM / key / behavior 不变。

## 13. 来源索引

- `docs/frontend/workbench-contract.md`
- `docs/frontend/review-checklist.md`
- `AGENTS.md`
- `README.md`
- `apps/desktop/src/renderer/useRunStageScrollController.ts`
- `apps/desktop/src/renderer/workbenchPresentation.ts`
- `apps/desktop/src/renderer/styles/workbench.css`
- `Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock.tsx`
- 当前 Codex 线程：`中间区域架构优化`
- Memory Task Group：`/Users/nolangrant/Awaken desktop turn-interaction handoff, assistant markdown rendering, and mid-turn scroll behavior`

## 14. 待确认问题

1. 多行 prompt 默认折叠后的目标高度是否固定为当前视觉设计，还是需要重新定义最大行数？
2. latest turn 自动 follow 时，是否永远以 final output bottom 为 tail，还是以 turn section bottom 为 tail？
3. 点击旧 marker 后，如果最新 run 仍在输出，是否永久暂停 follow，还是显示“回到最新输出”的显式 affordance？
4. 长任务的 tool trace 是否进入 collapsed work packet，是本轮 Workbench PRD 的 Phase 2，还是独立 PRD？
5. PRD 是否需要新增“录屏必须保存到固定 docs/acceptance 路径”的硬性要求？
