# Awaken Desktop Run Workbench 垂直切片开发计划

生成时间：2026-06-20

本文档用于指导 Awaken Desktop 从当前聊天界面、Dashboard 看板、卡片墙、开发者日志面板，转向录屏所示的 Agent Execution UI / Run Workbench。格式参考 `docs/desktop-v1-vertical-slices.md`，但产品目标已经改变：第一版 Desktop 的主界面不再是 Dashboard 或调试壳，而是围绕一个 Run / Task / Thread 展开的执行工作台。

## 目标判断

用户已经明确接受：

- 删除当前 Dashboard。
- 删除当前卡片墙。
- 删除当前开发者日志面板。
- 保留并复用已有 runtime、workspace、approval、artifact、cancel、reconnect、thread、provider status 等基础能力。
- 同时把 UI 组件、样式令牌和重复交互抽象为可复用、可维护的设计系统。

本轮计划的核心不是“把现有界面美化一下”，而是把 Desktop 主产品形态重构为：

```text
Awaken Desktop
  -> Run Workbench
  -> Run Narrative
  -> Inspector Rail
  -> Bottom Composer
  -> reusable @awaken/ui design system
```

## 总原则

### 1. 先产品形态，后组件库

设计系统必须从真实 Run Workbench 中抽象，不先做一个脱离产品的“大而全组件库”。组件库只收纳已经被产品验证过的 tokens、primitives、layout、narrative、inspector、composer pattern。

### 2. 原始事件不直接成为 UI

后端事件、AG-UI-compatible events、SSE frames 都不是最终 UI 模型。Desktop renderer 需要增加一层投影：

```text
AwakenEvent
  -> AG-UI-compatible event
  -> renderer runtime reducer
  -> RunNarrative projector
  -> committedItems + activeItem
  -> Run Workbench UI
```

### 3. 中心区域只展示执行叙事

中心区域回答 5 个问题：

```text
用户要做什么
Agent 正在想什么
Agent 正在看哪里
Agent 正在执行什么
Agent 已经交付什么
```

原始 JSON、tool result 全文、debug event list、runtime frame 细节进入右侧 Inspector，不进入中心主叙事。

### 4. 先用现有事件跑通体验，再补后端事件精度

第一阶段必须先用现有 runtime 事件完成视频式交互骨架。不要等后端事件全部完美后才改 UI。

第二阶段再补：

- tool args delta
- tool progress
- command output delta
- patch updated
- turn diff
- structured file/search/read action

### 5. 验收允许并鼓励真实 LLM

凡是触及 run、streaming、tool、approval、artifact、cancel、reconnect 的切片，最终验收必须至少包含一次真实 LLM run。只用 mock 的验收不算完成。

### 6. `packages/ui` 保持 UI-only

设计系统组件不知道 AG-UI、A2UI、SSE、Electron、runtime URL、API client，也不读取 `window.awaken`。所有数据通过 props 进入，所有行为通过 callbacks 传出。

## 目标界面

```text
┌────────────────────────────────────────────────────────────────────┐
│ Sidebar          Center Run Stage                         Inspector │
│                                                                    │
│ New Agent        Prompt card                              Changes   │
│ Automations      Planning / Thinking / Exploring          Browser   │
│ Repositories     Read file / Run command / Approval       Terminal  │
│ Runs             Final answer                             Files     │
│                                                                    │
│                  Bottom Composer                                    │
└────────────────────────────────────────────────────────────────────┘
```

## 核心状态模型

```ts
type RunNarrativeState = {
  committedItems: RunNarrativeItem[];
  activeItem?: RunNarrativeItem;
  inspectorSelection?: InspectorSelection;
};

type RunNarrativeItem =
  | { kind: "prompt"; id: string; text: string }
  | { kind: "planning"; id: string; text: string; status: "active" | "done" }
  | { kind: "thinking"; id: string; text: string; status: "active" | "done" }
  | { kind: "exploring"; id: string; actions: RunAction[]; status: "active" | "done" }
  | { kind: "tool"; id: string; title: string; detail?: string; status: "active" | "done" | "error" }
  | { kind: "approval"; id: string; title: string; status: "waiting" | "approved" | "rejected" }
  | { kind: "fileChange"; id: string; files: ChangedFile[]; status: "active" | "done" }
  | { kind: "answer"; id: string; markdown: string; status: "active" | "done" | "error" };
```

## Slice 1：删除旧主界面，建立 Run Workbench 壳

### 用户目标

用户打开 Desktop 后看到的是 Run Workbench，而不是 Dashboard、卡片墙或开发者日志面板。

### 用户流程

1. 用户启动 Awaken Desktop。
2. 左侧看到 workspace、runs、thread 入口。
3. 中间看到当前 run stage。
4. 右侧看到 Inspector rail。
5. 底部看到 composer。

### 开发范围

- Desktop：重构 `apps/desktop/src/renderer/App.tsx` 主布局。
- Desktop：删除 Dashboard、卡片墙、开发者日志面板在主路径里的渲染入口。
- Desktop：保留 runtime supervisor、workspace、thread、provider status 的必要入口。
- UI：先用当前 `@awaken/ui` layout/primitives 拼出三栏加底部 composer。
- UI：空状态也必须是 Workbench 空状态，不做 landing page。

### 完成定义

- 启动 Desktop 后第一屏是 Run Workbench。
- 主界面不再出现 Dashboard、卡片墙、开发者日志面板。
- runtime unavailable / starting / crashed 仍有可读状态提示。
- 当前 composer 可以发起真实 run。
- `npm run build:desktop` 通过。
- `npm run typecheck:ui` 或等价 UI typecheck 通过。

### 暂不做

- 不补后端新事件。
- 不做完整视觉 polish。
- 不做 Browser pane 的真实网页控制。

## Slice 2：Run Narrative 类型与事件投影

### 用户目标

用户看到的是“Agent 正在做什么”的叙事，而不是分散的 assistant stream、reasoning stream、tool results 面板。

### 用户流程

1. 用户发起 run。
2. 顶部出现 prompt card。
3. 中间出现 active item。
4. 事件继续到来时 active item 就地更新。
5. item 完成后进入 committed history。

### 开发范围

- Desktop：新增 `apps/desktop/src/renderer/run-narrative/`。
- Desktop：定义 `RunNarrativeItem`、`RunNarrativeState`、`InspectorSelection`。
- Desktop：新增 `projectAguiEventToNarrative()` 或等价 reducer 层。
- Desktop：把现有 AG-UI events 映射为 narrative item。
- Tests：覆盖 run start、reasoning delta、text delta、tool call、approval、artifact、run finish。

### 完成定义

- `run/requested` 生成 prompt item 和 planning active item。
- reasoning delta 进入 thinking item。
- text delta 进入 answer item。
- tool events 进入 tool 或 exploring item。
- approval events 进入 approval item。
- artifact events 进入 fileChange 或 inspector data。
- reducer replay 同一批事件时不会重复插入 item。
- 有 reducer 单元测试覆盖事件顺序和 replay。

### 暂不做

- 不要求 tool args delta。
- 不要求 command output delta。
- 不要求复杂 diff 视图。

## Slice 3：Prompt Card 与运行节奏

### 用户目标

用户发出任务后，输入内容被固定为顶部 prompt card，明确知道当前 run 的目标。

### 用户流程

1. 用户输入长 prompt。
2. 点击发送。
3. prompt 收束到中心顶部。
4. 下方显示 `Planning next moves`。
5. 底部 composer 进入 follow-up / stop 状态。

### 开发范围

- UI：新增 `PromptCard`。
- UI：新增 `ActivityLine` 用于 `Planning next moves`。
- Desktop：发送后冻结当前 prompt 到 narrative。
- Desktop：run 结束后仍保留 prompt card。
- Desktop：cancel / error 时 prompt card 不消失。

### 完成定义

- 长 prompt 不撑破布局。
- prompt card 在 desktop 宽屏和较窄窗口中都可读。
- run 过程中 prompt card 不被 streaming answer 挤掉。
- cancel、error、reconnect 后 prompt card 仍可恢复。
- 有组件级 mock 场景覆盖短 prompt、长 prompt、中文 prompt。

### 暂不做

- 不做 prompt 编辑历史。
- 不做 prompt 折叠搜索。

## Slice 4：Thinking 与 Final Answer 流式展示

### 用户目标

用户能看到 agent 从 thinking 到 final answer 的自然流式过程。

### 用户流程

1. 用户发起普通问答。
2. 界面显示 planning。
3. 如果 runtime 有 reasoning event，显示 thinking 摘要。
4. 最终回答以 markdown 流式出现。
5. run 完成后 answer 固化。

### 开发范围

- UI：新增 `ThinkingBlock`。
- UI：新增 `MarkdownAnswer`。
- Desktop：reasoning delta 映射为 thinking active item。
- Desktop：text delta 映射为 answer active item。
- Desktop：message end / run finish 时 commit active answer。
- Desktop：保留自动滚动到 live tail，但用户手动向上滚动时不强制抢回。

### 完成定义

- 真实 LLM 输入“请只回复 pong”，最终 answer 只显示 `pong`。
- 普通问答不出现 tool、approval、artifact。
- markdown 段落、列表、inline code 基础渲染正确。
- replay 后不会重复显示同一段 answer。
- provider 缺失或 stream error 时显示 error narrative item。

### 暂不做

- 不展示未授权的隐藏 chain-of-thought。
- 不做完整 GitHub-flavored markdown 全能力。
- 不做 citation system。

## Slice 5：Exploring 与工具动作归并

### 用户目标

用户看到的是 agent 正在探索项目，而不是裸露的 tool result/debug log。

### 用户流程

1. 用户输入“看一下这个项目并总结结构”。
2. agent 自主调用只读工具。
3. 中心区域显示 `Exploring`。
4. 下方逐条显示 `Searching files`、`Read README.md`、`Read App.tsx L1-L80`。
5. 工具细节可以在右侧 Inspector 查看。

### 开发范围

- Desktop：把 `list_files`、`grep_search`、`read_file` 等只读工具归并成 exploring item。
- Desktop：为 tool call 生成稳定 action id。
- Desktop：主叙事只显示动作摘要。
- Inspector：保存 tool args/result 全文供右侧查看。
- Tests：覆盖连续 read/search/list 合并成一个 exploring group。

### 完成定义

- 真实 LLM read-only 项目总结任务中，中心区域出现 `Exploring`。
- read/list/search 不渲染为互不相关的 debug log。
- 工具失败时对应 action 显示 error 状态。
- 主叙事不显示大段 tool result JSON。
- Inspector 可查看选中 action 的原始 args/result。

### 暂不做

- 不做并发工具可视化。
- 不做项目索引。
- 不做搜索结果全文高亮。

## Slice 6：Approval 与 File Change 进入叙事

### 用户目标

当 agent 需要修改文件时，用户在主叙事中看到审批请求，批准或拒绝后看到最终文件变更。

### 用户流程

1. 用户要求修改文件。
2. agent 触发 `apply_patch`。
3. 中心区域出现 approval item。
4. 用户 Approve 或 Reject。
5. Approve 后出现 file change item。
6. Reject 后显示 rejected，并且文件不变。

### 开发范围

- UI：把现有 `ApprovalCard` 适配到 narrative。
- UI：新增 `FileChangeSummary`。
- Desktop：approval request/resolved 进入 narrative state。
- Desktop：artifact/file-change events 进入 narrative 和 Changes inspector。
- Tests：覆盖 approve、reject、stream error 时 approval cleanup。

### 完成定义

- 审批前文件不变。
- Approve 后文件改变，并显示 changed files。
- Reject 后文件不变，approval item 显示 rejected。
- run cancel 时 pending approval 不残留为可点击状态。
- artifact path 与 approval affected paths 保持一致。
- 有真实 LLM smoke 覆盖 approve 或 reject 至少一个路径。

### 暂不做

- 不做完整 diff editor。
- 不做“以后总是允许/拒绝”的持久规则。

## Slice 7：Right Inspector Rail

### 用户目标

用户可以在右侧查看细节，但中心主叙事保持干净。

### 用户流程

1. 用户点击中心的 tool action。
2. 右侧 Files 或 Terminal 显示详细信息。
3. 用户点击 changed files。
4. 右侧 Changes 显示文件变更摘要。
5. Browser tab 在没有 browser runtime 时显示明确空状态。

### 开发范围

- UI：新增 `InspectorRail`。
- UI：新增 `InspectorTabs`，至少包含 `Changes`、`Browser`、`Terminal`、`Files`。
- Desktop：实现 `inspectorSelection`。
- Desktop：tool action、file change、approval 可设置 inspector selection。
- Desktop：Browser pane 先做 disabled/empty state，不伪造功能。

### 完成定义

- 右侧 tab 切换不影响 run streaming。
- 选中 tool action 后 Inspector 显示 args/result。
- 选中文件变更后 Inspector 显示 path、change type、summary。
- Terminal pane 能显示已有 shell/tool output；没有 command output delta 时显示完成后的 result 摘要。
- Browser pane 没有真实 runtime 时不出现假页面。
- Inspector 可折叠或在较窄窗口中保持不遮挡中心内容。

### 暂不做

- 不做真实 browser automation。
- 不做内嵌文件编辑器。
- 不做 terminal 交互输入。

## Slice 8：Sidebar Runs 与 Thread 状态

### 用户目标

用户能从左侧知道有哪些 run/thread，以及当前 run 是 running、needs input、ready for review 还是 done。

### 用户流程

1. 用户发起新 run。
2. 左侧当前 run 显示 running。
3. 需要审批时显示 needs input。
4. 文件修改待查看时显示 ready for review。
5. 完成后显示 done。
6. 切换 thread 后中心 narrative 可恢复。

### 开发范围

- UI：调整 `Sidebar` / `ConversationList` 为 run/thread 导航。
- Desktop：把 runtime thread list 映射为 sidebar item。
- Desktop：把 run status 映射为用户可理解状态。
- Desktop：切换 thread 时保存/恢复 narrative view。
- Tests：覆盖 thread switch 后 active/comitted narrative 不错乱。

### 完成定义

- 新 run 出现在左侧列表。
- awaiting approval 映射为 needs input。
- file change 或 finished with artifact 可以显示 ready for review 或 done。
- terminal/recovered run 切换后不丢 answer、approval、artifact。
- 左侧不再承担 Dashboard 或卡片墙职责。

### 暂不做

- 不做 scheduled runs。
- 不做云端历史。
- 不做跨 workspace thread 合并。

## Slice 9：Bottom Composer、Stop 与 Follow-up

### 用户目标

用户能随时看到底部 composer。run 中可 stop，run 后可 follow-up。

### 用户流程

1. 用户输入任务并发送。
2. run 中发送按钮变成 stop。
3. 用户可 cancel 当前 run。
4. run 完成后 composer 变成 follow-up。
5. follow-up 进入同一 thread。

### 开发范围

- UI：复用并收紧 `Composer`、`SendButton`、`ModeToggle`、`ModelSelector`。
- Desktop：busy 状态下主按钮触发 cancel。
- Desktop：done 状态下 submit 携带当前 thread id。
- Desktop：空输入不能提交。
- Tests：覆盖 submit、cancel、follow-up。

### 完成定义

- run 中点击 stop 调用 runtime cancel。
- cancel 后 UI 显示 cancelled，不显示 failed。
- run 完成后再次输入可以作为同一 thread follow-up。
- composer 不被中心内容遮挡。
- Cmd/Ctrl+Enter 行为正确。

### 暂不做

- 不做 run 中排队 follow-up。
- 不做语音输入。
- 不做富文本 prompt 编辑器。

## Slice 10：设计令牌 v2

### 用户目标

Awaken Desktop 的视觉语言稳定，不靠散落硬编码颜色和尺寸维持。

### 用户流程

用户不会直接操作 token，但会感受到 sidebar、stage、inspector、composer、narrative item 的间距、颜色、状态一致。

### 开发范围

- UI：升级 `packages/ui/src/styles/tokens.css`。
- UI：新增 Workbench / Narrative / Inspector / Terminal / Diff 语义 token。
- UI：迁移新组件到语义 token。
- Docs：新增或更新设计系统文档，说明 v2 token 来源是 Run Workbench，不再是旧 HTML demo。

### 建议 token

```css
--aw-surface-canvas
--aw-surface-sidebar
--aw-surface-stage
--aw-surface-inspector
--aw-surface-raised
--aw-text-muted
--aw-text-subtle
--aw-text-code
--aw-status-running
--aw-status-waiting
--aw-status-success
--aw-status-danger
--aw-diff-added
--aw-diff-removed
--aw-terminal-bg
--aw-terminal-text
```

### 完成定义

- 新 Workbench 组件不写散落颜色值。
- 新 Workbench 组件不写散落 shadow/radius magic number。
- token 命名表达语义，不表达具体页面位置。
- `packages/ui` build 通过。
- 文档明确 `docs/component-library.md` 是历史档案，v2 以 Run Workbench 为视觉源。

### 暂不做

- 不做多主题系统。
- 不做用户自定义主题。
- 不做 dark mode。

## Slice 11：Workbench Layout 组件

### 用户目标

Workbench 布局可以复用、维护，不把一堆 grid/flex 细节塞在 Desktop App 里。

### 用户流程

用户不会直接感知组件名，但会看到稳定的三栏、底部 composer、可折叠 Inspector。

### 开发范围

- UI：新增 `WorkbenchShell`。
- UI：新增 `WorkbenchSidebarFrame`。
- UI：新增 `RunStage`。
- UI：新增 `InspectorRail`。
- UI：新增 `ComposerDock`。
- Desktop：从 `App.tsx` 中移除布局细节，改为组合这些 UI-only 组件。
- Playground：添加 workbench mock 场景。

### 完成定义

- `packages/ui` 组件不 import runtime/AG-UI/Electron。
- Desktop 只传 props 和 callbacks。
- 组件支持至少 1280px 宽度下稳定展示。
- 较窄窗口下 Inspector 不遮挡 composer。
- 有 mock playground 展示 idle、running、approval、done 状态。

### 暂不做

- 不做移动端网页适配。
- 不做可拖拽面板宽度。

## Slice 12：Narrative 组件库

### 用户目标

Run Narrative 的各类状态可复用，后续新增工具或事件时不需要重写主界面。

### 用户流程

用户看到统一的 prompt、thinking、exploring、tool、approval、file change、answer 展示。

### 开发范围

- UI：新增 `PromptCard`。
- UI：新增 `NarrativeFeed`。
- UI：新增 `ActivityLine`。
- UI：新增 `ThinkingBlock`。
- UI：新增 `ExploringGroup`。
- UI：新增 `ToolActionRow`。
- UI：新增 `FileChangeSummary`。
- UI：新增 `MarkdownAnswer`。
- UI：所有组件 controlled props。

### 完成定义

- 所有 narrative 组件可在 playground 独立预览。
- 所有状态至少支持 active、done、error 或 waiting 中适用状态。
- 长文件名、长中文、长 code span 不撑破布局。
- active item 动画不造成 layout shift。
- 组件可通过 keyboard focus 选中并触发 Inspector callback。

### 暂不做

- 不做复杂 markdown 插件生态。
- 不做虚拟列表。
- 不做 canvas/graph 叙事组件。

## Slice 13：重复交互抽象

### 用户目标

Awaken 的常见交互一致：tab、popover、tooltip、copy、collapse、auto-scroll 都有统一行为。

### 用户流程

1. 用户 hover 图标看到 tooltip。
2. 用户切换 Inspector tab。
3. 用户展开 tool details。
4. 用户复制 command/path。
5. streaming 时 feed 自动跟随底部。
6. 用户向上滚动后不被强制拉回。

### 开发范围

- UI：补齐或收敛 `Tabs`。
- UI：补齐或收敛 `Tooltip`。
- UI：补齐或收敛 `Disclosure`。
- UI：新增 `CopyButton`。
- UI：新增 `AutoScrollTail` 或 hook。
- Desktop：Run Workbench 使用这些交互组件。

### 完成定义

- 所有 icon-only button 有 label/title 或 tooltip。
- tab 支持键盘切换。
- disclosure 支持 aria-expanded。
- copy 成功有轻量反馈。
- auto-scroll 不打断用户手动阅读历史。

### 暂不做

- 不做复杂 command palette。
- 不做全局快捷键管理器。

## Slice 14：Design Gallery 与视觉验收

### 用户目标

开发者能在不启动完整 runtime 的情况下检查设计系统组件状态，同时最终仍用真实 Desktop 验收。

### 用户流程

1. 开发者打开 UI playground。
2. 查看 Workbench idle/running/approval/done/error mock。
3. 修改组件。
4. 回到 Desktop 跑真实 LLM 验收。

### 开发范围

- UI：更新 `packages/ui/playground`。
- UI：新增 Run Workbench mock data。
- Tests：组件级 render smoke。
- QA：Playwright 或 Electron screenshot 验证关键状态。

### 完成定义

- playground 能展示所有 narrative item 类型。
- playground 能展示 Inspector tabs。
- playground 能展示 composer busy/done 状态。
- screenshot 中没有文本重叠、按钮文字溢出、嵌套卡片感。
- mock 只存在 playground，不进入 runtime reducer。

### 暂不做

- 不做 Storybook 引入，除非后续明确需要。
- 不把 playground 当产品入口。

## Slice 15：Tool Args Delta 与 Tool Progress

### 用户目标

用户看到工具调用不是突然出现，而是能看到 agent 正在准备和执行工具。

### 用户流程

1. 模型开始生成 tool call。
2. UI 显示工具输入正在形成。
3. 工具开始执行。
4. 工具执行过程中有 progress。
5. 工具完成后 action 固化。

### 开发范围

- LLM：OpenAI / Anthropic stream parser 暴露 tool args delta。
- Agent Core：新增或细化 `ToolCallArgsDelta`。
- Tool Runtime：tool interface 增加 progress callback。
- Runtime Event：新增 `ToolProgress`。
- AG-UI adapter：映射 args delta / progress。
- Desktop：projector 更新 active tool/exploring action。

### 完成定义

- Anthropic `input_json_delta` 或 OpenAI tool call delta 能进入内部事件。
- 没有 provider delta 时仍保持现有完整 args fallback。
- 工具 progress 可以被 reducer 接收并显示。
- 工具取消时不会留下 orphan active item。
- 有 provider parser 单元测试。
- 有真实 LLM tool task smoke。

### 暂不做

- 不做 StreamingToolExecutor 并发执行。
- 不做跨工具调度优化。

## Slice 16：Command Output、Patch Updated 与 Turn Diff

### 用户目标

当 agent 运行命令或修改文件时，用户能看到真实输出和最终变更，而不是等待结束后才出现一块摘要。

### 用户流程

1. agent 运行 shell 命令。
2. Terminal inspector 流式显示 command output。
3. agent 生成 patch。
4. Changes inspector 显示 patch updated。
5. run 完成后显示 turn diff。

### 开发范围

- Tools：shell 支持 output delta。
- Tools：apply_patch 支持 patch updated event。
- Runtime：新增 command output / patch updated / turn diff 事件。
- AG-UI adapter：映射为 desktop 可消费事件。
- Desktop：Terminal pane 展示 output delta。
- Desktop：Changes pane 展示 patch / diff。

### 完成定义

- 长 shell 输出不会等结束才显示。
- output delta 有截断策略，不把巨大输出塞爆 renderer。
- apply_patch 审批前可看到将修改哪些文件。
- approve 后 turn diff 与实际 git diff 一致。
- reject 后没有 file change artifact。
- 有 shell/apply_patch 单元测试和至少一次真实 mutation smoke。

### 暂不做

- 不做完整终端模拟器。
- 不做交互式 stdin。
- 不做大型 diff editor。

## Slice 17：Narrative Replay 与恢复

### 用户目标

刷新、断线、切换 thread 后，用户看到的 Run Narrative 不重复、不丢失、不乱序。

### 用户流程

1. 用户启动 run。
2. run 中产生 thinking/tool/answer。
3. renderer 刷新或 SSE 断开。
4. Desktop 通过 replay 恢复 narrative。
5. live tail 继续更新 active item。

### 开发范围

- Desktop：RunNarrative projector 必须 idempotent。
- Desktop：每个 narrative item 有稳定 id。
- Desktop：replay terminal run 后能重建 committed items。
- Runtime：继续使用 event seq / after_seq。
- Tests：覆盖 replay + live tail。

### 完成定义

- replay 后 prompt、thinking、tool、approval、artifact、answer 不重复。
- pending approval 能恢复为 waiting。
- terminal answer 能恢复为 done。
- disconnected stream 不会把 run 错误渲染为普通 failed。
- 有 reducer replay 测试和 Desktop recovery smoke。

### 暂不做

- 不做跨 runtime 重启持久化。
- 不做长期数据库。

## Slice 18：删除旧主界面的最终收口与真实验收

### 用户目标

用户拿到的是完整 Run Workbench，而不是旧 UI 和新 UI 并存的半成品。

### 用户流程

1. 用户打开 Desktop。
2. 发起普通问答。
3. 发起只读项目探索。
4. 发起文件修改并审批。
5. cancel 或 reconnect 一次。
6. 全程只使用 Run Workbench。

### 开发范围

- Desktop：删除不再使用的 Dashboard/card wall/dev log 组件、样式、mock 数据。
- UI：删除只服务旧主界面的组件或降级为 playground-only。
- Docs：更新根 README 和 Desktop README。
- QA：真实 LLM smoke + visual QA。

### 完成定义

- `rg -n "Dashboard|card wall|developer log|开发者日志|卡片墙" apps/desktop packages/ui` 不再命中产品主路径。
- 普通问答真实 LLM smoke 通过。
- 只读项目探索真实 LLM smoke 通过。
- approval approve/reject 至少一个真实 LLM smoke 通过。
- cancel 或 reconnect smoke 通过。
- Desktop screenshot 与目标视频的 IA 相符：左 sidebar、中间 run narrative、右 inspector、底部 composer。
- 没有 mock run 被误当真实验收。

### 暂不做

- 不做云同步。
- 不做 marketplace。
- 不做多 agent 编排 UI。

## 推荐执行顺序

```text
Phase 1：先让视频式体验在真实 Desktop 中出现
1. 删除旧主界面，建立 Run Workbench 壳
2. Run Narrative 类型与事件投影
3. Prompt Card 与运行节奏
4. Thinking 与 Final Answer 流式展示
5. Exploring 与工具动作归并
6. Approval 与 File Change 进入叙事
7. Right Inspector Rail
8. Sidebar Runs 与 Thread 状态
9. Bottom Composer、Stop 与 Follow-up

Phase 2：把真实产品组件抽成设计系统
10. 设计令牌 v2
11. Workbench Layout 组件
12. Narrative 组件库
13. 重复交互抽象
14. Design Gallery 与视觉验收

Phase 3：补齐接近 Codex / Claude Code 的事件精度
15. Tool Args Delta 与 Tool Progress
16. Command Output、Patch Updated 与 Turn Diff
17. Narrative Replay 与恢复

Phase 4：收口
18. 删除旧主界面的最终收口与真实验收
```

## 第一批建议交付

第一批只做 Slice 1 到 Slice 6，加上 Slice 10 的最小 token 补充。原因：

- 这能最快证明 Awaken Desktop 是否真的从 Dashboard 壳转成 Run Workbench。
- 这批可以主要用现有 runtime events 完成，不被后端事件精度卡住。
- 这批完成后，设计系统才有真实组件可以抽象。

第一批完成后的最小验收脚本：

```text
1. 打开 Desktop。
2. 发起“请只回复 pong”。
3. 发起“看一下本项目并总结结构”。
4. 发起一个需要 apply_patch 的小修改，并 approve 或 reject。
5. 确认主界面没有 Dashboard、卡片墙、开发者日志面板。
6. 确认中心区域是 prompt -> planning/thinking/exploring/tool/approval/file-change/answer 的执行叙事。
```

## 全局暂不做

- 不做 Dashboard。
- 不做卡片墙。
- 不做开发者日志主面板。
- 不做通用 A2UI renderer 扩张。
- 不做 Browser runtime 假功能。
- 不做多主题系统。
- 不做 cloud task / remote agent。
- 不做 subagent/worktree UI。

## 给 Coding Agent 的执行约束

- 每个切片开始前先读本文件、根 README、`docs/01_项目情况说明/AG-UI&A2UI说明/agui-a2ui-architecture.md`、相关 Desktop reducer/UI 文件。
- 修改范围要小，优先让一个真实用户流程闭环。
- `packages/ui` 只放 UI-only 组件。
- Runtime policy 只能在 Go runtime/tool 层，不能移到 renderer。
- 触及 runtime 的切片，必须跑真实 LLM smoke。
- 触及 UI 的切片，必须有至少一次视觉检查，确认没有文本重叠、布局抖动、旧主界面残留。
