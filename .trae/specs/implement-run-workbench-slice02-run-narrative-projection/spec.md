# Slice 2：Run Narrative Projection Spec

## Why

Slice 1 已经把 Desktop 第一屏改成 Run Workbench 壳，但中心仍主要依赖 runtime timeline 和临时 answer/thinking 摘要。用户真正需要看到的是 Agent 执行叙事：prompt、planning、thinking、tool、approval、file change、answer，而不是原始 AG-UI event 或分散 debug stream。

本切片新增 renderer-only 投影层，把现有 AG-UI-compatible events 映射为 `RunNarrativeState`。投影层只消费 renderer 已经收到的事件，不改变 Go runtime、server、tool policy 或 `@awaken/ui` 边界。

## What Changes

- Desktop：新增 `apps/desktop/src/renderer/run-narrative/`。
- Desktop：定义 `RunNarrativeItem`、`RunNarrativeState`、`InspectorSelection`、`RunAction`、`ChangedFile`。
- Desktop：新增 `projectAguiEventToNarrative()`，把 AG-UI events 投影为 committed/active narrative item。
- Desktop：`RuntimeViewState` 持有 narrative state；`run/requested` 生成 prompt item 和 planning active item。
- Desktop：Run Stage 用 narrative list 渲染 committed items 和 active item。
- Tests：覆盖 run start、reasoning delta、text delta、tool call、approval、artifact、run finish、replay/idempotency。

## Impact

- Desktop 中心区域开始由 narrative projection 驱动。
- `packages/ui` 不新增协议依赖。
- Runtime policy 仍在 Go runtime/tool 层。
- 现有 assistantText、reasoningText、steps、approvals、artifacts 继续保留，作为后续迁移和 Inspector 数据源。

## Requirements

### Requirement: Narrative state model

The system SHALL define a renderer narrative model for run execution.

#### Scenario: Import narrative types

- **WHEN** renderer code imports narrative types
- **THEN** it can represent prompt, planning, thinking, exploring, tool, approval, fileChange, and answer items
- **AND** it can represent committed items, active item, and inspector selection

### Requirement: Request starts prompt and planning narrative

The system SHALL initialize narrative state when a run is requested.

#### Scenario: User submits prompt

- **WHEN** reducer receives `run/requested`
- **THEN** narrative committed items include a prompt item
- **AND** narrative active item is a planning item

### Requirement: AG-UI events project to narrative items

The system SHALL map existing AG-UI events into narrative items.

#### Scenario: Reasoning stream arrives

- **WHEN** `REASONING_MESSAGE_CONTENT` events arrive
- **THEN** narrative active item is a thinking item with accumulated text

#### Scenario: Text stream arrives

- **WHEN** `TEXT_MESSAGE_CONTENT` events arrive
- **THEN** narrative active item is an answer item with accumulated markdown

#### Scenario: Tool call arrives

- **WHEN** `TOOL_CALL_START`, `TOOL_CALL_ARGS`, `TOOL_CALL_END`, or `TOOL_CALL_RESULT` events arrive
- **THEN** narrative shows a tool or exploring item

#### Scenario: Approval arrives

- **WHEN** `AWAKEN_APPROVAL_REQUESTED` arrives
- **THEN** narrative shows an approval item
- **AND** inspector selection points to that approval

#### Scenario: Artifact arrives

- **WHEN** `AWAKEN_ARTIFACT_CREATED` or `AWAKEN_ARTIFACT_UPDATED` arrives
- **THEN** narrative shows a fileChange item or updates inspector data

### Requirement: Replay is item-idempotent

The system SHALL avoid duplicate narrative item insertion when the same event batch is replayed.

#### Scenario: Same event batch is reduced twice

- **WHEN** a reducer receives the same sequenced AG-UI events twice
- **THEN** committed item IDs remain unique
- **AND** text deltas with already-seen event sequence keys are not appended again

## Modified Requirements

- The center Run Stage now renders from narrative projection instead of standalone answer/thinking cards.

## Removed Requirements

- None.
