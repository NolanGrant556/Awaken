# Wave 1 产品化补丁包 Spec

## Why

Wave 1 的会话理解、上下文可见、分支三项能力在后端、HTTP、Electron bridge、renderer client 全链路都已接通，但 `App.tsx` 没有调用这些能力：侧栏还在用 ProjectTree 子项展示会话且只显示 `label`，状态文案是英文，标题实际只是 prompt 清洗（`cmd/awaken-core` 未注入 LLM title generator，落到 `promptTitleGenerator` 兜底），run 完成后侧栏不刷新；Inspector "Files" 只展示选中 tool/artifact，从不调用 `getThreadContext`；`branchThread` 整条 bridge 接了但没有按钮入口；分支后新 thread 没有 `runIDs`，切过去是空白会话。本补丁包把这些"已接未通"的能力在 UI 上贯通，让用户真正看见 Wave 1 的价值。

## What Changes

- **侧栏会话体验**
  - thread item 标题改用 `metadata.displayTitle`（依次 fallback `aiTitle` → `customTitle` → `label`），并在标题下展示 `taskSummary` 摘要行。
  - 状态文案改为中文：运行中 / 待输入 / 已取消 / 已完成 / 出错 / 待复核 / 空闲。
  - run 完成 / 取消 / 出错后自动 `refreshThreads()`，让 LLM 生成的标题和摘要刷新回侧栏。
  - 后端：在 `cmd/awaken-core` 接入真实 LLM title generator（复用 `llm.LoadConfig()`），通过 `RunServiceConfig.TitleGenerator` 注入，使 `aiTitle` 由 LLM 生成而非 prompt 清洗。
- **Inspector Context**
  - Inspector "Files" tab 改为 "Context"。
  - 切换 thread 或当前 thread 的 run 完成后，调用 `getThreadContext(threadId)`，展示上下文文件列表（path、source、tokens、last_read_time）和 usage 摘要（model、total_tokens / max_tokens、percentage、message_count）。
  - 保留选中 tool / artifact 的展示能力，作为 Context 面板内的子视图（选中态优先级高于 thread context 总览）。
- **Branch 按钮**
  - 当前会话在 thread header 或 Composer 工具区提供"分支"按钮，调用 `runtimeClient.branchThread(threadId, { customTitle? })`。
  - 分支成功后切换到新 thread，并在侧栏 thread item 和 Inspector Context 展示 `metadata.branch`（来源 thread label、forked-from message）。
- **修 branch transcript 坑**
  - 切换到没有 `runIDs` 的 branch thread 时，不再显示空白会话，而是展示"从 {parent label} 分支"来源卡片 + forked-from message 预览 + "查看来源会话"入口（跳回父 thread）。

## Impact

- 受影响能力：会话标题、会话状态展示、会话摘要、上下文文件可见、上下文用量可见、会话分支、分支后体验。
- 受影响代码：
  - `apps/desktop/src/renderer/App.tsx`（侧栏 thread item 渲染、Inspector tab 与面板、branch 按钮、branch 来源卡片）
  - `apps/desktop/src/renderer/threadPresentation.ts`（中文状态文案）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（run 终态后触发 refresh、branch 结果落库）
  - `apps/desktop/src/renderer/runtime/client.ts`（已存在 `getThreadContext` / `branchThread`，仅需调用）
  - `cmd/awaken-core/main.go`（构造并注入 LLM title generator）
  - `src/runtime/run_service.go` / `src/runtime/wave1.go`（确认 `titleGenerator` 注入路径，无需改行为）
  - `src/titlegen`（新增 LLM-backed `Generator` 实现或由 main.go 内联实现）
- 不做：不做大重构，不改变 transcript run-centric 架构，不引入云端历史，不改变 AG-UI / A2UI 协议边界。
- 不做：不让 `Awaken Design System` 依赖 runtime / Electron；新增 UI 仍走 props + callbacks。

## ADDED Requirements

### Requirement: 侧栏会话标题与摘要展示

The system SHALL render sidebar thread items using the thread's best available title (`metadata.displayTitle` → `metadata.aiTitle` → `metadata.customTitle` → `label`), and SHALL display the `taskSummary` text as a secondary line under the title when present.

#### Scenario: Thread with AI title

- **WHEN** a thread has `metadata.aiTitle` set
- **THEN** the sidebar item title shows the AI title
- **AND** the raw `label` is not shown unless no AI/custom title exists

#### Scenario: Thread with task summary

- **WHEN** a thread has `metadata.taskSummary` set
- **THEN** the sidebar item displays the summary as a muted secondary line
- **AND** the summary is truncated to a single line

#### Scenario: Thread without AI title or summary

- **WHEN** a thread has no `aiTitle`, `customTitle`, or `taskSummary`
- **THEN** the sidebar item falls back to `label` and shows no secondary line

### Requirement: 侧栏会话状态中文文案

The system SHALL present thread status in Chinese in the sidebar: 运行中 / 待输入 / 已取消 / 已完成 / 出错 / 待复核 / 空闲.

#### Scenario: Status mapping

- **WHEN** a thread is in `running` status with no artifacts
- **THEN** the sidebar shows "运行中"
- **WHEN** a thread has pending approvals
- **THEN** the sidebar shows "待输入"
- **WHEN** a thread is `cancelled` / `finished` / `error`
- **THEN** the sidebar shows "已取消" / "已完成" / "出错"
- **WHEN** a thread is `running` with artifacts
- **THEN** the sidebar shows "待复核"
- **WHEN** a thread is idle
- **THEN** the sidebar shows "空闲"

### Requirement: run 终态后侧栏自动刷新

The system SHALL refresh the thread list from the runtime after the active run reaches a terminal state (finished / cancelled / error), so that LLM-generated titles and summaries appear in the sidebar without manual refresh.

#### Scenario: Run finished refreshes sidebar

- **WHEN** the active run emits a terminal `RUN_FINISHED` / `RUN_ERROR` / cancelled status
- **THEN** the renderer calls `listThreads()` to refresh `runtimeState.threads`
- **AND** the sidebar re-renders with the latest `aiTitle` and `taskSummary`

### Requirement: LLM 生成会话标题

The system SHALL generate thread titles via the LLM provider configured in `llm.LoadConfig()`, injected through `RunServiceConfig.TitleGenerator`, so that `aiTitle` is a real LLM summary rather than a cleaned prompt.

#### Scenario: LLM title generator wired

- **WHEN** `awaken-core` starts
- **THEN** a real LLM-backed `titlegen.Generator` is passed into `RunServiceConfig.TitleGenerator`
- **AND** `promptTitleGenerator` is no longer used as the active generator

#### Scenario: Title generated after first exchange

- **WHEN** a run completes on a thread that has no `aiTitle` and no `customTitle`
- **AND** the thread has a usable first user/assistant exchange
- **THEN** the LLM generator is invoked
- **AND** the resulting title is stored as `metadata.aiTitle`

### Requirement: Inspector Context 面板

The system SHALL provide an Inspector "Context" panel that calls `getThreadContext(threadId)` and displays the thread's context files and usage, while preserving the existing tool/artifact selection sub-view.

#### Scenario: Context panel shows thread context

- **WHEN** the user switches to a thread or the active run finishes
- **THEN** the renderer calls `runtimeClient.getThreadContext(threadId)`
- **AND** the Context panel lists context files (path, source, tokens, last_read_time)
- **AND** the Context panel shows usage summary (model, total_tokens / max_tokens, percentage, message_count)

#### Scenario: Tool or artifact selection overrides context overview

- **WHEN** the user selects a tool result or artifact from the narrative
- **THEN** the Context panel shows the selected tool/artifact detail
- **AND** the thread context overview is hidden until the selection is cleared

#### Scenario: Empty context

- **WHEN** `getThreadContext` returns no files and no usage
- **THEN** the Context panel shows an empty state explaining there is no context yet

### Requirement: 当前会话分支按钮

The system SHALL provide a "分支" action on the current thread that calls `runtimeClient.branchThread(threadId, { customTitle? })` and switches to the new branch thread.

#### Scenario: Create branch

- **WHEN** the user clicks the "分支" button on a thread that has content
- **THEN** `runtimeClient.branchThread(threadId, request)` is invoked
- **AND** on success the renderer switches to the new branch thread
- **AND** the new branch thread appears in the sidebar

#### Scenario: Branch metadata shown

- **WHEN** the active thread has `metadata.branch`
- **THEN** the sidebar item and/or Inspector shows "从 {parent label} 分支"
- **AND** the forked-from message preview is available

#### Scenario: Branch on empty thread

- **WHEN** the user clicks "分支" on a thread with no messages
- **THEN** the action is disabled or returns a structured error
- **AND** no empty branch is created

### Requirement: 分支来源展示与空白会话修复

The system SHALL NOT show an empty conversation when switching to a branch thread that has no runs; instead it SHALL show a branch source card describing the parent thread and forked-from message.

#### Scenario: Switch to branch with no runs

- **WHEN** the user switches to a branch thread whose `runIDs` is empty
- **THEN** the conversation area shows a "从 {parent label} 分支" card
- **AND** the card shows the forked-from message preview
- **AND** a "查看来源会话" action navigates back to the parent thread

#### Scenario: Branch with runs

- **WHEN** the user switches to a branch thread that already has runs (e.g. after sending a follow-up)
- **THEN** the normal transcript replay is shown
- **AND** the branch source card is not displayed (or is collapsed)

## MODIFIED Requirements

无。本补丁包只新增 UI 调用与展示，不改变已有 slice 8 / slice 10 的契约。

## REMOVED Requirements

无。
