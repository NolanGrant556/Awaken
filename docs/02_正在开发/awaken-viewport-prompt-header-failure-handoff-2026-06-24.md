# Awaken 视口当前轮消息气泡失败交接报告

日期：2026-06-24  
仓库：`/Users/nolangrant/Awaken`  
状态：本报告写作时 Awaken Desktop 已关闭；当前工作区存在未提交实验性改动。

## 0. 给 0 上下文 Agent 的结论

这不是一个单纯的 CSS 问题，也不是把某个气泡 `position: sticky` 就能解决的问题。连续多轮对话时，用户希望看到的是类似对标产品的“视口当前轮 prompt header”交互：

- 每一轮对话是一个完整的 turn：`用户消息 -> Thought/thinking -> assistant/tool/approval/file-change 输出`。
- 顶部用户消息气泡表示当前视口正在阅读的 turn，而不是永远表示最新一轮。
- 当用户向上/向下滚动跨过 turn 边界时，顶部气泡应自然切换成上一轮/下一轮用户消息。
- 更关键的是：顶部气泡显示哪一轮，气泡下面的正文第一屏就应是同一轮的 Thought/answer，而不能出现 header 是第二轮、正文还停在第一轮的脱节。
- `Thought / thinking` 必须保留，不隐藏、不移出主流。
- 用户消息气泡视觉应是“白色系但略深于背景”，不要米黄色；宽度应接近 composer rail；滚动条保留 thumb，但不要明显轨道。

当前实现仍然失败。用户最新截图显示：

- 顶部 header 是“再次告诉我”，但正文顶部紧接着仍显示历史 prompt“你好”和第一轮输出。
- 换句话说，当前轮 header 与正文阅读位置仍然脱节。
- 用户还给了一张空白 Electron 窗口截图，说明某次启动/热更新/窗口渲染状态也可能出现 blank window，需要下一位 Agent 在真实 Electron 中确认。

建议下一位 Agent 不要继续表层微调现有 header 实现，而是先重建“turn section + viewport header”的交互模型，再落回代码。

## 1. 用户真实需求

### 1.1 初始问题

用户提供了两个视频：

- Awaken 当前产品视频：`/Users/nolangrant/Library/Containers/5ZSL2CJU2T.com.dingtalk.mac/Data/Library/Application Support/DingTalkMac/ScreenCapture/钉钉录屏_2026-06-24 113925.mp4`
- 对标产品视频：`/Users/nolangrant/Library/Containers/5ZSL2CJU2T.com.dingtalk.mac/Data/Library/Application Support/DingTalkMac/ScreenCapture/钉钉录屏_2026-06-24 114034.mp4`

用户指出：Awaken 在同一个 thread 里发送第二条消息后，上一条消息在前端不可见，即使用滚轮也不能向上看到，所以多轮对话展示有问题。

### 1.2 后续明确需求

经过多轮反馈，需求从“历史消息不丢”进一步细化为：

1. 多轮对话历史必须保留。
2. 用户每条消息是一个 turn 的 header/context anchor。
3. 当前视口顶部应显示当前正在阅读的那一轮用户消息。
4. 当滚动跨过 turn 边界，顶部消息气泡要切换到对应 turn 的用户消息。
5. header 切换后，正文阅读位置也必须跟随到同一 turn 的输出区域。
6. 历史用户消息仍可见，但不能一直 sticky 或抢占顶部。
7. `Thought / thinking` 不能移除。
8. 气泡颜色要接近白色，但比背景略深一点；不要米黄色。
9. 气泡宽度可拉长，接近 composer 的 content rail。
10. 滚动条不希望看到轨道，thumb 可以保留。

### 1.3 最新失败截图

用户最新提供的失败截图：

- `/var/folders/d5/1yf2cz996yz7xzy0fn953wv40000gn/T/codex-clipboard-0a173b86-f01f-456f-8ac6-477e3f8dc943.png`
- `/var/folders/d5/1yf2cz996yz7xzy0fn953wv40000gn/T/codex-clipboard-16eda18d-f995-4d63-bc0f-6ba9adab39de.png`

第一张图的关键问题：

- 顶部气泡显示“再次告诉我”。
- 下面正文首先出现“你好”气泡和第一轮 Thought/answer。
- 第二轮 Thought/answer 在下面，但没有被定位到当前 header 下方。
- 这证明当前方案只是换了顶部 header 文案，并没有建立“header 与正文 turn body 同步”的交互。

第二张图的关键问题：

- Electron 窗口几乎空白，只显示 macOS 红黄绿窗口按钮。
- 需要下一位 Agent 确认这是热更新/renderer crash/路由空态/截图时机问题，还是另一个独立 bug。

## 2. 重要背景和入口

### 2.1 启动/关闭约束

根 `README.md` 规定：

- 正式打开 Awaken Desktop：`npm run open:desktop`
- 不要用普通浏览器打开 `127.0.0.1:5174` 作为 Desktop 成功标准，因为 renderer 依赖 Electron bridge。
- 组件 Gallery 入口是 `npm run open:ui`，只有在明确调组件库时使用。

本轮已按用户要求关闭 Awaken。关闭后状态检查：

```text
Awaken Desktop status
- screen: not running
- renderer: not reachable
- electron: not running
- awaken-core: not running
```

### 2.2 当前工作区状态

本报告写作时，工作区存在未提交改动，涉及：

```text
Awaken Design System/src/components/AwakenRunStage/AwakenRunStage.tsx
Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock.tsx
Awaken Design System/src/tokens/components.css
apps/desktop/src/main/index.ts
apps/desktop/src/preload/index.ts
apps/desktop/src/renderer/App.test.ts
apps/desktop/src/renderer/App.tsx
apps/desktop/src/renderer/runtime/client.test.ts
apps/desktop/src/renderer/runtime/client.ts
apps/desktop/src/renderer/runtime/reducer.test.ts
apps/desktop/src/renderer/runtime/reducer.ts
apps/desktop/src/shared/runtime.ts
apps/desktop/vite.config.ts
src/runtime/run_service.go
src/runtime/run_service_test.go
src/runtime/session.go
src/runtime/session_test.go
src/server/handlers.go
src/server/router.go
src/server/server_test.go
```

这些改动包含有价值的诊断尝试，但 UI 目标仍未达成。下一位 Agent 应先读 diff，不要默认它们都是正确方向。

## 3. 我尝试过的方案

### 3.1 第一轮：修复 narrative 被 `run/requested` 重置

目标：同一个 thread 里发送第二条消息时，第一轮 prompt/answer 不应从前端消失。

做法：

- 将 run narrative 从“当前 run 状态”调整为“thread transcript + active run tail”。
- `run/requested` 不再直接替换 `state.narrative`。
- 增加 `appendRunRequestedNarrative(previous, prompt, scopeId)`。
- prompt/planning/answer/tool/approval/fileChange ids 改成带 run/thread scope，避免 `prompt:current`、`planning:current` 覆盖历史。
- 保持 `threadViews` 机制，切换 thread 时保存/恢复 narrative。

效果：

- 自动化测试能证明历史 prompt/answer 不再被简单覆盖。
- 但这只解决了“历史 item 不丢”的数据层问题，不解决对标产品那种 viewport header 交互。

### 3.2 第二轮：把最新 prompt 做成 current sticky 气泡

目标：最新一轮用户消息置顶，看起来像对标产品。

做法：

- 将 turn 分成 `execution-turn-current` 和 `execution-turn-history`。
- `AwakenUserPromptBlock` 增加 `variant?: "current" | "history"`。
- 试图让 current prompt sticky，历史 prompt 不 sticky。
- 调整气泡宽度、滚动条轨道透明等。

失败原因：

- “最新轮”不等于“视口当前轮”。
- 用户滚动历史时，顶部气泡仍然可能锁在最新轮。
- sticky 气泡容易覆盖 `Thought`，出现 header 与正文重叠/遮挡。
- 这仍然是样式层修补，不是真正的 turn viewport 模型。

### 3.3 第三轮：新增独立 `promptHeader` + scroll spy

目标：顶部 header 独立显示 `viewportActivePromptId` 对应的用户消息，正文仍保留完整 turn 流。

做法：

- `AwakenRunStage` 新增 `promptHeader?: ReactNode`。
- `App.tsx` 新增：
  - `viewportActivePromptId`
  - `turnVariantForViewport(groupId, viewportActivePromptId)`
  - `activePromptIdForScrollPosition(turnMeasurements, anchorY)`
  - `shouldFollowLatestTurn(isUserDetached, activePromptId, latestPromptId)`
- 为每个 turn 注册 DOM ref。
- 在 scroll container 的 `onScroll` 中用 `requestAnimationFrame` 计算当前 turn。
- header 显示 active prompt。
- 旧的 `.execution-prompt-block-current { position: sticky }` 被移除。

效果：

- 单测能证明不同 scroll anchor 下会选择不同 prompt。
- 真实 Electron 第一轮/第二轮时可以看到顶部 header 改变。

失败原因：

- header 被放在滚动正文外部，变成“独立显示层”。
- 只改 header text，并不保证正文 scrollTop 同步到同一 turn body。
- 结果出现：header 是第二轮，正文仍显示第一轮内容。
- 这就是用户最新截图中的失败现象。

### 3.4 第四轮：发现旧 thread 恢复数据错配，补 `run.input` 和 thread transcript API

在真实 Electron 中发现一个更深层问题：

- 点击旧 thread 时，前端只有 `thread.label` 和 latest run events。
- `thread.label` 是第一轮输入，比如“你好”。
- latest run events 可能是第三轮输出，比如“你怎么知道的？”。
- 前端会把第三轮输出挂到第一轮 prompt 下面，造成 prompt-output 错配。

做法：

- Go runtime `Run` 增加 `Input string json:"input,omitempty"`。
- `NewSession(runID, threadID, input, cancel)` 保存 trimmed input。
- `Thread` 增加 `runIDs []string`，记录同一 thread 下 run 顺序。
- `RunService.GetThreadTranscript(threadID)` 返回：
  - thread snapshot
  - 每轮 run snapshot
  - 每轮 runtime event history
- server 增加：
  - `GET /v1/threads/{thread_id}/transcript`
  - 返回每轮 run + 映射后的 AG-UI events。
- desktop bridge 增加：
  - `getThreadTranscript(threadId)`
  - main/preload/shared/runtime/client 全链路类型。
- renderer 增加：
  - `actionsForThreadTranscript(transcript)`，把 transcript replay 成 reducer actions。
  - `run/threadReplayStarted`，避免 replay 叠加旧 `threadViews`。

效果：

- 这是有价值的治本数据改动。
- `go test ./src/runtime ./src/server` 通过。
- desktop tests/typecheck 通过。

局限：

- 它解决的是“恢复旧 thread 时 prompt-output 错配”。
- 它不解决当前正在滚动时 header/body 同步失败。
- 这部分可以考虑保留，但需要下一位 Agent 重新评估 API 形状是否应成为长期接口。

### 3.5 第五轮：隐藏当前轮 inline prompt，尝试用 turn body 锚点 + tail spacer

目标：header 已显示当前轮 prompt 后，正文里当前轮不要再重复一遍同文 prompt；同时滚动到当前轮 body 顶部。

做法：

- 新增 `shouldRenderInlinePrompt(groupId, viewportActivePromptId)`。
- 当前 active prompt 不在正文 inline 渲染，历史 prompt 保留。
- `scrollTurnToTop` 从 turn container 锚点改成 turn body 锚点：
  - 为 `.execution-turn-body` 注册 ref。
  - 优先滚到 turn body。
- 新增 `tailSpacerForAnchor(desiredScrollTop, maxScrollTop)`：
  - 当内容高度不足以把当前 turn body 滚到顶部时，在 feed 末尾补透明 spacer。

效果：

- 自动化测试通过。
- 但用户实测仍失败。

失败原因推测：

- `requestAnimationFrame` + React state 更新 spacer 后再设置 scrollTop 的时序不稳定。
- header 在 scroll container 外部，仍有可能与正文 scroll state 脱节。
- 当前实现仍然试图用外部 header 去追随内部 scroll，而不是让 prompt header 成为 scroll content 的一部分。
- 这证明“外部 header clone + scroll spy”方案风险很高。

### 3.6 颜色调整尝试

用户明确说：不要米黄色；要白色，但比背景深一点。

做法：

- 将 `.execution-prompt-block-current p` 从 `#fff8e8` 改为 `#fbfbf8`。
- 边框改为低对比 `rgba(31, 31, 31, 0.08)`。
- 阴影改弱。

状态：

- 颜色方向符合用户最新口径，但需真实 UI 验证。

## 4. 自动化验证记录

本轮执行过的主要命令：

```bash
go test ./src/runtime ./src/server
npm --workspace @awaken/desktop test
npm --workspace @awaken/desktop run typecheck
npm run smoke:ui
npm run build:desktop
git diff --check
```

结果：

- `go test ./src/runtime ./src/server` 通过。
- `npm --workspace @awaken/desktop test` 通过，最近一次为 94 tests passed。
- `npm --workspace @awaken/desktop run typecheck` 通过。
- `npm run smoke:ui` 通过。
- `npm run build:desktop` 通过。
- `git diff --check` 通过。

注意：这些通过不能代表用户验收通过，因为关键失败是实际 Electron 里的滚动/视口交互和视觉感受。

## 5. 本轮执行中遇到的问题

### 5.1 沙箱限制

多次遇到沙箱限制：

- `ps -axo ...` 被限制。
- `go test` 访问 `~/Library/Caches/go-build` 被限制。
- `vite --host 127.0.0.1 --port 5174` 监听端口被限制。
- `screencapture` 在沙箱中失败。

处理方式：

- 对需要 Go build cache、本地端口、截图、进程检查的命令使用了 `require_escalated`。

### 5.2 Awaken 启动状态问题

遇到过 stale `awaken-dev-desktop` screen：

- `npm run open:desktop` 显示 screen 存在，但 renderer/electron/core 都没起来。
- 使用 `screen -S awaken-dev-desktop -X quit` 清理后重新启动。

也遇到过用户最新截图中的 blank Electron window，需要下一位 Agent 重点复现。

### 5.3 Vite token import 问题

运行 `npm run smoke:ui` 后，desktop dev server 曾出现 Vite import overlay：

```text
Failed to resolve import "@awaken/design-system/tokens/tokens.css"
```

处理方式：

- 在 `apps/desktop/vite.config.ts` 增加 alias，让 desktop dev 直接使用 design-system source tokens：
  - `@awaken/design-system/tokens/tokens.css`
  - `@awaken/design-system/tokens/components.css`

这部分看起来是合理修复，避免 desktop dev 依赖 transient dist 状态。

### 5.4 Computer Use / 截图问题

真实 Electron 验证中遇到：

- Computer Use 有时无法输入中文，但 ASCII 输入正常。
- Computer Use 截屏偶发 `ScreenCaptureKit.SCStreamErrorDomain Code=-3812`。
- `screencapture` 有时截到 Codex 而不是 Electron，因为 Electron 没成功成为前台。

因此下一位 Agent 不要只依赖截图；更好的方式是：

- 用 Electron/Playwright/CDP 或可注入调试 hook 读取 DOM。
- 或者在 Gallery/fixture 中构造 deterministic narrative，而不是每次走真实 LLM。

## 6. 当前失败的核心原因分析

### 6.1 不变量没有被建模

正确的不变量应是：

```text
activePromptId === 当前顶部 header prompt
firstVisibleTurnBody.promptId === activePromptId
```

当前实现只保证了第一条：

```text
activePromptId === header prompt
```

但没有稳定保证：

```text
scrollTop 已经把对应 turn body 放到 header 下方
```

因此出现 header 和正文脱节。

### 6.2 外部 header clone 是高风险方案

当前方案把 header 放在 scroll container 外部：

```text
RunStage
  promptHeader  <-- 独立于 scroll
  scroll body   <-- narrative turns
  composer
```

这使得 header 的视觉状态和 scroll body 的真实位置必须用 JS 同步。一旦时序、内容高度、streaming、用户滚动、tail spacer 有一点偏差，就会错位。

用户看到的失败就是这种错位。

### 6.3 更可能正确的交互模型

下一位 Agent 应优先验证这个方向：

```text
scroll container
  turn section 1
    prompt header 1  // 在 scroll flow 内
    turn body 1
  turn section 2
    prompt header 2
    turn body 2
  turn section 3
    prompt header 3
    turn body 3
```

然后让 prompt header 在同一个 scroll container 内做 section-sticky 或 equivalent layout。

这样 header 与正文天然属于同一个 turn section，不需要外部 clone 去追 scroll。

可选实现路径：

1. 原生 CSS sticky section header：
   - 每个 turn 的 prompt 在自己的 section 内 `position: sticky; top: 0`。
   - 利用浏览器 sticky 的 push-off 行为实现滚过边界切换。
   - 必须解决 Thought 被遮挡的问题：让 sticky prompt 在流内占位，turn body 与 prompt 间有明确 spacing；不要把 prompt 绝对定位。
2. IntersectionObserver + 外部 header：
   - 只有在能写出强测试证明 `activePromptId` 与 `firstVisibleTurnBody` 一致时才考虑。
   - 必须有真实 DOM/e2e 测试，不要只写纯函数单测。

我的建议：优先做方案 1。它更接近对标产品的“滚动穿越 section header”交互，也减少 JS 状态机复杂度。

## 7. 下一位 Agent 的建议执行计划

### Step 1：冻结当前失败状态

1. 不要先继续改代码。
2. 先读：
   - `README.md`
   - 本报告
   - 当前 `git diff`
3. 用用户最新截图确认失败现象：
   - header 是当前/最新 prompt。
   - body 仍从历史 prompt/历史 output 开始。
   - Electron 可能 blank。

### Step 2：建立 deterministic UI fixture

不要一开始走真实 LLM。先创建一个可重复的三轮 narrative fixture，最好在 desktop renderer test 或 design-system demo 中能固定渲染：

```text
Turn 1:
  prompt: 你好
  Thought: first thought
  answer: first answer
Turn 2:
  prompt: 再次告诉我
  Thought: second thought
  answer: second answer
Turn 3:
  prompt: 你怎么知道的
  Thought: third thought
  answer: third answer
```

验收这个 fixture：

- 打开时定位 Turn 3，顶部 prompt 应是 Turn 3。
- Turn 3 prompt 下方第一屏应是 Turn 3 Thought/answer。
- 向上滚到 Turn 2 区域，顶部 prompt 切 Turn 2，正文是 Turn 2。
- 再向上滚到 Turn 1 区域，顶部 prompt 切 Turn 1，正文是 Turn 1。
- 向下反向同理。

### Step 3：替换外部 header clone 方案

建议重构方向：

- `AwakenRunStage.promptHeader` 很可能应该撤掉，或只作为实验分支，不作为最终方案。
- 在 `ExecutionFeed` 内按 turn section 渲染 prompt header。
- prompt header 作为 scroll content 的一部分，而不是 scroll body 外面的独立行。
- 如果使用 sticky：
  - sticky top 应贴近顶栏/scroll viewport 顶部。
  - prompt block 背景为 near-white。
  - 保留文档流占位，避免 Thought 被 prompt 覆盖。

### Step 4：保留或重新评估 thread transcript API

本轮补的 thread transcript API 是为了解决旧 thread 恢复错配，建议不要直接删除，但要重新评估：

- `Run.Input` 是否应该长期进入 `/v1/runs/{run_id}` 响应。
- `GET /v1/threads/{thread_id}/transcript` 是否是正确 API 名称和 shape。
- Desktop 是否应该在 thread switch 时 replay 整个 transcript。

这部分解决的是另一类真实 bug：

```text
旧 thread label = 第一轮 prompt
latest run events = 最新轮输出
=> 前端把最新输出挂到第一轮 prompt 下
```

如果没有 transcript，旧线程恢复永远会有 prompt-output 错配风险。

### Step 5：必须做真实 Electron 验收

不要只跑单测。最终至少要：

1. `npm run open:desktop`
2. 发送三轮：
   - `你好`
   - `你是谁？`
   - `你怎么知道的？`
3. 在第三轮结束后确认：
   - 顶部 prompt 是第三轮。
   - prompt 下方正文是第三轮 Thought/answer。
   - 第一轮/第二轮内容只在向上滚时出现。
4. 向上滚：
   - 进入第二轮 body，header 切第二轮。
   - 进入第一轮 body，header 切第一轮。
5. 向下滚：
   - header 和 body 同步回第二轮/第三轮。
6. 截图检查：
   - 气泡颜色是 near-white，不偏黄。
   - 气泡宽度与 composer rail 对齐。
   - 滚动条 track 不明显。
   - Thought 没有被移除或遮挡。

## 8. 当前改动中可能有价值的部分

可以考虑保留：

- `appendRunRequestedNarrative` 和 scoped narrative ids。
- `Run.Input`。
- thread run order / transcript replay 思路。
- `apps/desktop/vite.config.ts` token alias。
- prompt 气泡 near-white 方向。
- 滚动条 track transparent。

需要谨慎甚至重做：

- `AwakenRunStage.promptHeader` 外部 header。
- `viewportActivePromptId` + scroll-spy 外部同步模型。
- `tailSpacerForAnchor` 作为补丁式滚动修复。
- 当前 `scrollTurnToTop` / `requestAnimationFrame` 时序。

## 9. 当前代码涉及的关键文件说明

### 9.1 Desktop renderer

- `apps/desktop/src/renderer/App.tsx`
  - 当前大部分 UI 逻辑都在这里。
  - 包含 turn grouping、viewport active prompt、scroll spy、thread transcript replay。
  - 也是失败最集中处。

- `apps/desktop/src/renderer/runtime/reducer.ts`
  - 管理 run/thread/narrative 状态。
  - 已增加 `run/threadReplayStarted`。
  - 旧 `run/recovered` 仍存在，但 thread switch 新路径尝试走 transcript replay。

- `apps/desktop/src/renderer/run-narrative/projector.ts`
  - 投影 AG-UI events 到 `RunNarrativeItem`。
  - 已支持 scoped ids。

### 9.2 Design System

- `Awaken Design System/src/components/AwakenRunStage/AwakenRunStage.tsx`
  - 当前新增了 `promptHeader`，但这个方向可能需要撤销。

- `Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock.tsx`
  - 当前有 `variant?: "current" | "history"`。

- `Awaken Design System/src/tokens/components.css`
  - prompt block 样式、run stage grid、scrollbar 样式都在这里。

### 9.3 Runtime / Server / Bridge

- `src/runtime/session.go`
  - `Run` 增加 `Input`。

- `src/runtime/run_service.go`
  - `Thread` 增加 `runIDs`。
  - 增加 `GetThreadTranscript`。

- `src/server/router.go`
  - 增加 `/v1/threads/{thread_id}/transcript`。

- `src/server/handlers.go`
  - transcript endpoint 映射 runtime events 到 AG-UI events。

- `apps/desktop/src/shared/runtime.ts`
  - 增加 transcript wire/runtime 类型。

- `apps/desktop/src/main/index.ts`
  - 增加 `awaken:get-thread-transcript` IPC handler。

- `apps/desktop/src/preload/index.ts`
  - bridge 暴露 `getThreadTranscript`。

- `apps/desktop/src/renderer/runtime/client.ts`
  - client 暴露 `getThreadTranscript`。

## 10. 不要重复的错误

1. 不要只说“测试通过”。用户验收是视觉/交互，不是纯函数。
2. 不要把“最新轮”当成“视口当前轮”。
3. 不要让 header 只换文字，正文却不移动。
4. 不要移除 Thought。
5. 不要把气泡调成明显米黄色。
6. 不要只用浏览器 `127.0.0.1:5174` 验证 Desktop。
7. 不要忽略旧 thread 恢复路径。旧 thread 没 transcript 会发生 prompt-output 错配。
8. 不要继续在 `scrollIntoView` / tail spacer 上堆补丁，除非先有稳定 DOM/e2e 验收。

## 11. 建议最终验收标准

最终修复必须同时满足：

- 新开 thread 连续三轮后，三轮内容都保留。
- 第三轮结束时，顶部 prompt 是第三轮，正文第一屏是第三轮 Thought/answer。
- 向上滚入第二轮输出区域，顶部 prompt 切换成第二轮，正文第一屏是第二轮输出。
- 向上滚入第一轮输出区域，顶部 prompt 切换成第一轮，正文第一屏是第一轮输出。
- 向下滚回第三轮同理。
- 切换 thread 后，不发生 first prompt + latest output 的错配。
- Thought 始终存在。
- 用户 prompt 气泡 near-white，非米黄色，宽度接近 composer。
- 滚动条无明显轨道。
- Electron 不出现 blank window。

## 12. 当前报告生成时的最后状态

- Awaken Desktop 已关闭。
- 当前工作区有未提交实验性改动。
- 自动化测试多数通过，但用户实际验收失败。
- 下一位 Agent 应从本报告、用户截图、当前 diff 和真实 Electron fixture 重新建立交互模型。

## 13. 2026-06-24 后续 Agent 代码修改追加记录：仍失败

### 13.1 记录口径

用户在本报告生成后，又让另一个 Coding Agent 继续处理；该 Agent 做完并启动 Awaken 后，用户实际测试仍然反馈失败。因此本节继续记录当前代码修改，以免下一位 0 上下文 Agent 重复同一条路。

重要限制：

- 当前工作区没有把“上一轮失败实现”和“后续 Agent 失败实现”分成两个提交。
- 因此本节不能保证逐行区分是谁改的，只能按当前 dirty diff 记录“现在仓库里已经存在的失败方案”。
- 本节记录时的当前 diff 规模为 20 个文件，约 `1002 insertions / 189 deletions`。
- 前文第 8、9 章曾描述过 `promptHeader` 外置 header 方向；但当前代码里 `AwakenRunStage` 已没有 `promptHeader` prop，实际落地方案已经变成“正文 prompt 全局 sticky + 底部自动跟随”。

### 13.2 当前改动文件范围

当前 `git diff --stat` 覆盖这些文件：

- `Awaken Design System/src/components/AwakenRunStage/AwakenRunStage.tsx`
- `Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock.tsx`
- `Awaken Design System/src/tokens/components.css`
- `apps/desktop/src/main/index.ts`
- `apps/desktop/src/preload/index.ts`
- `apps/desktop/src/renderer/App.test.ts`
- `apps/desktop/src/renderer/App.tsx`
- `apps/desktop/src/renderer/runtime/client.test.ts`
- `apps/desktop/src/renderer/runtime/client.ts`
- `apps/desktop/src/renderer/runtime/reducer.test.ts`
- `apps/desktop/src/renderer/runtime/reducer.ts`
- `apps/desktop/src/shared/runtime.ts`
- `apps/desktop/vite.config.ts`
- `src/runtime/run_service.go`
- `src/runtime/run_service_test.go`
- `src/runtime/session.go`
- `src/runtime/session_test.go`
- `src/server/handlers.go`
- `src/server/router.go`
- `src/server/server_test.go`

### 13.3 后续方案实际做了什么

#### A. 后端增加 thread transcript 能力

代码路径：

- `src/runtime/session.go`
- `src/runtime/run_service.go`
- `src/server/router.go`
- `src/server/handlers.go`
- `src/runtime/run_service_test.go`
- `src/server/server_test.go`

具体变化：

- `Run` 增加 `Input string`，用于保存本轮用户输入。
- `NewSession(runID, threadID, input, cancel)` 开始把输入写入 session snapshot。
- `Thread` 增加 `runIDs []string`，每次同 thread 的新 run 都 append。
- `RunService.GetThreadTranscript(threadID)` 返回 thread snapshot 和每个 run 的事件历史。
- HTTP 新增 `GET /v1/threads/{thread_id}/transcript`。
- server handler 把 runtime events 映射成 AG-UI events 后返回。

这部分的价值：

- 方向上是对的，它解决的是“切换 thread 后如何恢复多轮 transcript”的数据来源问题。
- 它比只用 `latest run + thread label` 更接近真实多轮 conversation。

但它不是本次 UI 失败的核心修复：

- 用户当前验收失败集中在“当前视口 prompt 气泡如何随阅读位置切换”，不是后端没有 transcript。
- `runIDs` 仍是内存态 thread 结构的一部分；刷新、重启、持久化后的完整历史恢复仍需要另一个阶段确认。
- 旧 thread 或旧 session 如果没有 `Run.Input`，仍可能回退到 label 或出现恢复语义不完整。

#### B. Desktop bridge / client 增加 transcript 调用

代码路径：

- `apps/desktop/src/shared/runtime.ts`
- `apps/desktop/src/main/index.ts`
- `apps/desktop/src/preload/index.ts`
- `apps/desktop/src/renderer/runtime/client.ts`
- `apps/desktop/src/renderer/runtime/client.test.ts`

具体变化：

- shared runtime 类型新增：
  - `RuntimeRun.input`
  - `RuntimeThreadTranscriptWire`
  - `RuntimeThreadTranscript`
  - `normalizeRuntimeThreadTranscript`
- Electron main 新增 IPC handler：
  - `awaken:get-thread-transcript`
- preload bridge 新增：
  - `getThreadTranscript(threadId)`
- renderer runtime client 新增：
  - `client.getThreadTranscript(threadId)`

这部分本身没有明显问题，但它只提供数据通道，不决定 UI 如何滚动、锚定和切换 prompt。

#### C. Reducer 增加 thread replay 路径

代码路径：

- `apps/desktop/src/renderer/runtime/reducer.ts`
- `apps/desktop/src/renderer/runtime/reducer.test.ts`

具体变化：

- `RuntimeAction` 新增 `run/threadReplayStarted`。
- `run/threadReplayStarted` 会重置 view state 的 narrative，并保留 supervisor/threadViews/threads，然后设置 active thread。
- `run/recovered` 的 prompt 恢复逻辑改为优先使用 `run.input`，再回退到 thread label。

注意点：

- 这解决的是“恢复/切换 thread 时 prompt 文案来源错”的一部分。
- 但它仍然不是 viewport 当前轮 header 的实现。
- 如果下一位 Agent 继续改 UI，不建议先删除这块；应该先通过 e2e 确认它是否确实修复了 thread switch transcript 错配，再决定保留或收敛。

#### D. App.tsx 把 narrative 改成按 prompt 分组

代码路径：

- `apps/desktop/src/renderer/App.tsx`
- `apps/desktop/src/renderer/App.test.ts`

新增导出的函数和类型：

- `stripNarrativeScope(value)`
- `rawToolId(selectionId)`
- `rawApprovalId(selectionId)`
- `rawFileChangeId(selectionId)`
- `NarrativeTurnGroup`
- `groupNarrativeItemsByPrompt(items)`
- `latestPromptGroupId(groups)`
- `executionTurnClassName(group)`
- `actionsForThreadTranscript(transcript)`

渲染变化：

- 之前是 `visibleNarrativeItems.map(...)` 直接渲染。
- 现在先 `groupNarrativeItemsByPrompt(visibleNarrativeItems)`。
- 每个 group 渲染为：
  - 外层 `.execution-turn`
  - prompt 仍然在正文里用 `<UserPromptBlock text={group.prompt.text} />`
  - 其余 Thought / answer / tool / approval / fileChange 放进 `.execution-turn-body`
- 原来内联在 map 里的 item 渲染逻辑被抽成 `renderNarrativeItem`。

这一步有部分价值：

- prompt 和后续输出被分成 turn group，是正确的基础数据结构。
- `rawToolId/rawApprovalId/rawFileChangeId` 会剥掉 scoped narrative id，能避免 inspector 查不到 runtime 原始记录。

但关键问题是：

- `latestPromptGroupId` 只在测试里使用，当前生产渲染没有真正建立 `viewportActivePromptId`。
- 没有 scroll-spy。
- 没有根据滚动 anchor 选择当前阅读轮次。
- 没有独立 header 区。
- prompt 仍在正文流里承担 sticky header 职责，导致布局语义和视觉语义混在一起。

#### E. 当前滚动逻辑退化为“底部跟随”

代码路径：

- `apps/desktop/src/renderer/App.tsx`
- `Awaken Design System/src/components/AwakenRunStage/AwakenRunStage.tsx`

具体变化：

- 移除了 `useAwakenAutoScrollTail`。
- `App.tsx` 新增：
  - `runStageScrollElementRef`
  - `userScrolledRef`
  - `setRunStageScrollRef`
  - `handleRunStageScroll`
- `AwakenRunStage` 新增事件透传：
  - `onWheel`
  - `onPointerDown`
  - `onTouchStart`
- `RunStage` 使用这些事件来判断用户是否离开底部。
- 有一个 effect 在 `!showCenterComposer && !userScrolledRef.current` 时执行：
  - `scrollTop = scrollHeight`

失败点：

- 这仍然是传统 chat 的 tail-follow 模型，不是“视口当前轮 prompt header”模型。
- 它只关心用户是否在底部附近，不关心用户当前正在读第几轮。
- streaming 增长时仍可能按底部逻辑影响阅读位置。
- 没有“滚过第二轮输出区域后 header 切换为第二轮 prompt”的机制。

#### F. Design System 当前变成“每个正文 prompt 都 sticky”

代码路径：

- `Awaken Design System/src/components/AwakenRunStage/AwakenRunStage.tsx`
- `Awaken Design System/src/components/AwakenUserPromptBlock/AwakenUserPromptBlock.tsx`
- `Awaken Design System/src/tokens/components.css`

`AwakenRunStage` 当前状态：

- 没有 `promptHeader` prop。
- 只是在 scroll container 上透传滚动/指针事件。
- 布局仍是：
  - `.aw-run-stage-scroll`
  - composer

`AwakenUserPromptBlock` 当前状态：

- 没有 `variant?: "current" | "history"`。
- 没有 header/current/history 语义。
- 始终渲染：

```tsx
<article className="execution-prompt-block">
  <p>{text}</p>
</article>
```

`components.css` 当前关键样式：

```css
.execution-prompt-block {
  position: sticky;
  top: 0;
  z-index: 1;
  min-width: 0;
  padding: 4px 0 12px;
  background: var(--aw-surface-work);
}

.execution-prompt-block p {
  width: 100%;
  min-height: 34px;
  border: 1px solid rgba(31, 31, 31, 0.08);
  border-radius: 9px;
  background: #fbfbf8;
  box-shadow: 0 1px 2px rgba(23, 23, 23, 0.05);
}
```

这正是当前失败的核心：

- sticky 被放在所有正文 prompt 上，而不是一个外置、单例、由 scroll-spy 驱动的 header。
- 多轮对话时，多个 sticky prompt 会在各自 parent boundary 内参与 sticky 计算。
- 当滚动到 turn 边界附近，旧 prompt、新 prompt、正文 Thought 会出现互相挤压、堆叠或视觉错位。
- 用户截图中出现的“顶部一个 prompt，下面又一个 prompt，然后正文读到旧/新输出混在一起”，与这个实现高度吻合。
- 这不是 CSS 微调能解决的问题，因为结构模型错了：正文元素不应该同时承担 transcript item 和 viewport header 两个职责。

#### G. 视觉调整已经做了，但仍不达标

当前视觉相关变化：

- `.aw-run-stage` 新增 `--aw-run-content-width: 780px`。
- `.execution-feed`、composer 使用同一 content width。
- prompt 气泡从 fit-content 变成 `width: 100%`。
- prompt 背景为 `#fbfbf8`。
- scrollbar track 设为 transparent，thumb 为低透明灰。

用户后续明确反馈：

- 颜色不要米黄色。
- 要“白色，但是比背景深色一点，但就是白色”。

因此后续不应继续用明显 warm/yellow palette。建议下一轮改成更接近：

- 页面背景：保持当前 surface。
- prompt 气泡：`#fbfbfb` / `#fafafa` / `rgba(255,255,255,0.72)` 这一类 near-white。
- 边框：极浅灰，如 `rgba(24,24,27,0.08)`。
- 阴影要极轻，不能卡片化。

### 13.4 当前测试为什么没有挡住失败

新增测试集中在：

- prompt grouping 纯函数。
- latest prompt group id。
- execution turn class。
- transcript replay into narrative。
- scoped id stripping。
- bridge/client transcript proxy。
- Go transcript endpoint。

缺失的测试是这次失败的关键：

- 没有 DOM 滚动验收。
- 没有断言“正文 prompt 不应全局 sticky”。
- 没有 `viewportActivePromptId`。
- 没有 `activePromptIdForScrollPosition`。
- 没有模拟三轮 turn 的 scroll anchor。
- 没有验证“向上滚入第二轮输出区域时，顶部 header 切成第二轮 prompt”。
- 没有验证“顶部 header 是独立单例，不占用/覆盖正文 Thought”。
- 没有真实 Electron 截图 diff 或 Playwright 截图检查。

所以单测即使通过，也只能说明数据被分组了，不能证明交互达标。

### 13.5 当前失败现象与代码原因对应

用户给出的现状截图里能看到：

- 顶部出现一个用户气泡“再次告诉我”。
- 下面又出现另一个用户气泡“你好”。
- 正文 Thought 和 answer 混在这些气泡下面。
- 整体不像“当前视口 header + 当前 turn body”，更像多个正文 prompt 在滚动过程中一起 sticky。

对应代码原因：

1. `.execution-prompt-block { position: sticky; top: 0 }` 作用在每一轮正文 prompt 上。
2. `AwakenUserPromptBlock` 没有 current/history/header variant，无法区分职责。
3. `RunStage` 没有外置 header slot，prompt 只能留在正文里抢顶部。
4. `App.tsx` 没有 scroll-spy，也没有 viewport active prompt state。
5. 当前 auto-scroll 仍以 bottom 为目标，不以“当前阅读轮次”为目标。

### 13.6 给下一位 Agent 的建议：不要继续补这个方案

建议不要继续在当前 sticky-inline-prompt 方案上微调：

- 不要只调 `top`、`z-index`、`padding`。
- 不要试图用每个 turn 内部 sticky 伪装成全局 header。
- 不要再把所有 prompt 都 `position: sticky`。
- 不要只用 `scrollTop = scrollHeight` 或 `scrollIntoView` 解决阅读焦点。

更靠谱的结构应该是：

1. 正文 transcript 保持完整流式内容，但正文 prompt 不 sticky。
2. RunStage 顶部增加一个独立的、单例的 header rail。
3. header 显示 `viewportActivePromptId` 对应 prompt。
4. `viewportActivePromptId` 由 scroll container 内各 turn 的测量结果计算。
5. 每个 turn 需要有稳定 ref/data attribute，例如 `data-turn-prompt-id`。
6. scroll handler 用 requestAnimationFrame 节流，按 anchorY 选择最后一个 `turnTop <= anchorY` 的 turn。
7. 新 prompt 发送时可以临时跟随最新 turn；用户滚动后由 scroll-spy 接管。
8. header 不应该遮挡 Thought，也不应该是正文 item 的 sticky 副本。

### 13.7 最小复现与验收建议

下一位 Agent 应先写一个 deterministic UI fixture，而不是直接连真实 LLM 猜：

- 构造 3 个 turn：
  - prompt: `你好`
  - prompt: `你是谁？`
  - prompt: `你怎么知道的？`
- 每个 turn 都包含：
  - prompt
  - Thought
  - answer
- 内容高度要足够触发滚动。

需要自动化验证：

- scrollTop 在第一轮 body 区域时，header 文案为 `你好`。
- scrollTop 在第二轮 body 区域时，header 文案为 `你是谁？`。
- scrollTop 在第三轮 body 区域时，header 文案为 `你怎么知道的？`。
- DOM 中正文 prompt 不带 `position: sticky` 语义 class。
- header 是单例，不能在正文中复制多个 sticky header。
- Thought 文本在 header 下方可读，没有被遮挡。

真实 Electron 验收再跑：

- 连续发送 `你好 / 你是谁？ / 你怎么知道的？ / 再次告诉我`。
- 任意滚动位置只应有一个顶部上下文气泡。
- 向上滚动跨过输出区域时，顶部气泡跟随阅读轮次切换。
- 气泡颜色 near-white，不能偏黄。
- 滚动条轨道透明，thumb 可见。

### 13.8 当前本节记录时的状态

- Awaken Desktop 此前由 `npm run open:desktop` 成功启动过。
- 用户随后手动验收失败。
- 本节只记录当前失败实现，没有修改业务代码。
- 下一位 Agent 应先确认是否保留 transcript 后端改动，再重做 prompt header 的前端结构。
