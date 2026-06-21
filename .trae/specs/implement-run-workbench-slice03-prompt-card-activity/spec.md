# Slice 3：Prompt Card 与运行节奏 Spec

## Why

Slice 2 已经建立 Run Narrative projection，但 prompt 和 planning 仍只是普通 narrative article。用户发起任务后，需要在中心顶部稳定看到“本次 run 要做什么”，并且看到明确的 planning 节奏。长 prompt、中文 prompt、cancel/error/reconnect 都不应让目标丢失或撑破布局。

本切片把 prompt/planning 的 UI 表达抽为 `@awaken/ui` 的 UI-only 组件，再由 Desktop 传入 narrative 数据。组件不接触 AG-UI、A2UI、Electron、runtime client、SSE 或 `window.awaken`。

## What Changes

- UI：新增 `PromptCard`。
- UI：新增 `ActivityLine`。
- UI：导出新增组件，并在 playground 提供短 prompt、长 prompt、中文 prompt mock 场景。
- Desktop：Run Stage 中 prompt item 使用 `PromptCard` 渲染。
- Desktop：Run Stage 中 planning item 使用 `ActivityLine` 渲染。
- Desktop：保留 Slice 2 narrative state，不改变 runtime/server/tool policy。

## Impact

- 中心叙事顶部会更稳定地展示用户 prompt。
- planning active item 会用轻量活动行表达，而不是普通内容卡。
- `packages/ui` 增加真实产品验证过的 narrative primitives。

## Requirements

### Requirement: Prompt card component

The system SHALL expose a UI-only prompt card.

#### Scenario: Render short prompt

- **WHEN** `PromptCard` receives a short prompt
- **THEN** it renders the prompt with stable spacing and readable status metadata

#### Scenario: Render long prompt

- **WHEN** `PromptCard` receives a long prompt
- **THEN** it wraps or scrolls without expanding beyond its container

#### Scenario: Render Chinese prompt

- **WHEN** `PromptCard` receives Chinese text
- **THEN** it preserves readability and does not clip glyphs

### Requirement: Activity line component

The system SHALL expose a UI-only activity line for planning state.

#### Scenario: Planning is active

- **WHEN** `ActivityLine` receives `status="active"`
- **THEN** it shows a compact active state for `Planning next moves`

#### Scenario: Planning is done

- **WHEN** `ActivityLine` receives `status="done"`
- **THEN** it shows a completed state without layout shift

### Requirement: Desktop uses prompt/planning components

The Desktop Run Stage SHALL render prompt and planning narrative items with the new UI components.

#### Scenario: User submits run prompt

- **WHEN** reducer receives `run/requested`
- **THEN** Run Stage renders the prompt item through `PromptCard`
- **AND** Run Stage renders the planning item through `ActivityLine`

#### Scenario: Run reaches terminal states

- **WHEN** run is cancelled, errors, reconnects, or finishes
- **THEN** the prompt card remains visible if the narrative state still contains the prompt item

## Modified Requirements

- Prompt and planning narrative items are no longer rendered by the generic narrative article branch.

## Removed Requirements

- None.
