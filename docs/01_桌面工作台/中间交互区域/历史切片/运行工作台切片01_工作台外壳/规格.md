# Slice 1：Run Workbench Shell Spec

## Why

Awaken Desktop 当前主界面仍然以 Phase 4 调试面板形态呈现：中心区域包含 hero、assistant stream、reasoning stream、tool result、artifact 面板等工程化内容。新的 Desktop 第一屏应是 Run Workbench，而不是 Dashboard、卡片墙或开发者日志面板。

本切片先建立真实产品壳：左侧 Sidebar，中间 Run Stage，右侧 Inspector Rail，底部 Composer。runtime、workspace、thread、provider status、approval、artifact、cancel 能力继续复用，复杂 narrative 投影留到后续切片。

## What Changes

- Desktop：重构 `apps/desktop/src/renderer/App.tsx` 主布局为 Run Workbench。
- Desktop：移除主路径里的 hero、assistant/reasoning debug stream、tool result debug 面板。
- Desktop：保留 workspace picker、runtime 状态、provider/model 状态、thread 列表、approval 弹窗、artifact 数据、cancel run。
- Desktop：右侧 Inspector Rail 展示 runtime、workspace、approvals、artifacts、latest tool/result 的可读摘要。
- Desktop：底部 Composer 固定在 workbench 底部，可以继续发起真实 run。
- UI：优先复用 `@awaken/ui` 已有 `AppShell`、`Sidebar`、`TopBar`、`Composer`、`ApprovalCard`。
- UI：Artifact 摘要先在 Desktop Inspector 内用紧凑列表展示，后续设计系统切片再抽象 reusable inspector components。

## Impact

- Desktop 用户打开应用后第一屏进入 Run Workbench。
- `@awaken/ui` 仍保持 UI-only，不 import AG-UI、A2UI、Electron、runtime client、SSE 或 `window.awaken`。
- Runtime policy 仍在 Go runtime/tool 层，不移入 renderer。

## Requirements

### Requirement: Desktop first screen is Run Workbench

The system SHALL render the Desktop first screen as a Run Workbench shell.

#### Scenario: Launch Desktop

- **WHEN** the user opens Awaken Desktop
- **THEN** the main product path shows a left Sidebar
- **AND** the center shows a Run Stage
- **AND** the right side shows an Inspector Rail
- **AND** the bottom shows a Composer
- **AND** the first screen does not show Dashboard, card wall, developer log, or Phase 4 debug panels

### Requirement: Runtime lifecycle remains visible

The system SHALL keep readable runtime lifecycle states after the shell rewrite.

#### Scenario: Runtime is starting

- **WHEN** the Desktop-managed runtime is starting
- **THEN** the Run Stage shows a readable starting state
- **AND** Composer and workspace actions are disabled

#### Scenario: Runtime crashed

- **WHEN** the Desktop-managed runtime has crashed
- **THEN** the Run Stage shows `Awaken needs restart`
- **AND** the UI does not expose process logs, runtime URL, port, or exit details

### Requirement: Existing run entry still works

The system SHALL preserve the existing start-run path from the Composer.

#### Scenario: User submits prompt

- **WHEN** the runtime is ready, workspace path is valid, and the user submits a non-empty prompt
- **THEN** Desktop validates the workspace
- **AND** starts a run with the current thread/workspace/client request data
- **AND** subscribes to the run stream
- **AND** clears the Composer after the run is created

### Requirement: Existing operational data moves to Inspector

The system SHALL move operational/debug data out of the center stage and into the Inspector Rail.

#### Scenario: Run has artifacts or approvals

- **WHEN** runtime state includes approvals, artifacts, errors, or tool results
- **THEN** the Inspector Rail shows a compact readable summary
- **AND** the center Run Stage remains focused on the run state and current answer

## Modified Requirements

- The previous Phase 4 center debug panels are no longer a product-surface requirement.
- The center stage can temporarily show current answer/reasoning summaries until Slice 2 introduces the narrative projector.

## Removed Requirements

- The Desktop main path SHALL NOT render a Dashboard view.
- The Desktop main path SHALL NOT render a card-wall view.
- The Desktop main path SHALL NOT render a developer-log panel.
- The Desktop main path SHALL NOT render raw assistant stream, raw reasoning stream, tool result lists, or artifact grids as the central primary experience.
