# Slice 4：Thinking 与 Final Answer 流式展示 Spec

## Why

Slice 2 已经把 reasoning/text events 投影为 thinking/answer item，Slice 3 已经固定 prompt/planning 节奏。本切片让普通问答的主叙事真正可读：reasoning summary 用 thinking block 呈现，最终 answer 用基础 markdown 呈现，并在 streaming 时保持自然跟随 live tail。

本切片不展示隐藏 chain-of-thought；只展示 runtime 已经通过 `REASONING_*` event 明确发出的 summary 文本。

## What Changes

- UI：新增 `ThinkingBlock`。
- UI：新增 `MarkdownAnswer`，支持段落、列表、inline code 的基础渲染。
- Desktop：thinking narrative item 使用 `ThinkingBlock`。
- Desktop：answer narrative item 使用 `MarkdownAnswer`。
- Desktop：Run Stage 在靠近底部时自动滚动到 live tail；用户手动向上滚动后不强制抢回。
- Desktop：provider 缺失或 stream error 时显示 error narrative item。
- Tests：覆盖 markdown 基础渲染、replay 不重复、RUN_ERROR 投影为 error answer。

## Impact

- 普通问答路径不再把 answer 作为纯文本 `<p>` 渲染。
- Run Stage 会更像连续执行叙事，而不是静态卡片列表。
- `packages/ui` 继续保持 UI-only。

## Requirements

### Requirement: Thinking block component

The system SHALL expose a UI-only thinking block.

#### Scenario: Thinking summary arrives

- **WHEN** `ThinkingBlock` receives summary text
- **THEN** it renders readable thinking content without exposing hidden reasoning

### Requirement: Markdown answer component

The system SHALL expose a UI-only markdown answer component for basic final answers.

#### Scenario: Paragraph markdown

- **WHEN** `MarkdownAnswer` receives paragraph text
- **THEN** it renders readable paragraphs

#### Scenario: List markdown

- **WHEN** `MarkdownAnswer` receives simple bullet or ordered lists
- **THEN** it renders list items instead of one flattened paragraph

#### Scenario: Inline code markdown

- **WHEN** `MarkdownAnswer` receives inline code spans
- **THEN** it renders the code span with code styling

### Requirement: Desktop streaming answer display

The Desktop Run Stage SHALL render thinking and answer narrative items with the new UI components.

#### Scenario: Reasoning delta arrives

- **WHEN** reducer receives `REASONING_MESSAGE_CONTENT`
- **THEN** Run Stage renders the item with `ThinkingBlock`

#### Scenario: Text delta arrives

- **WHEN** reducer receives `TEXT_MESSAGE_CONTENT`
- **THEN** Run Stage renders the item with `MarkdownAnswer`

#### Scenario: Run finishes

- **WHEN** reducer receives `TEXT_MESSAGE_END` or `RUN_FINISHED`
- **THEN** the answer item is committed and remains visible

### Requirement: Live tail behavior

The Desktop Run Stage SHALL follow streaming content only when the user is near the bottom.

#### Scenario: User is near live tail

- **WHEN** new narrative content arrives
- **THEN** the stage scrolls to the live tail

#### Scenario: User has scrolled upward

- **WHEN** new narrative content arrives
- **THEN** the stage does not force-scroll back to the bottom

### Requirement: Error narrative

The system SHALL show provider or stream errors in narrative.

#### Scenario: Provider auth missing

- **WHEN** `RUN_ERROR` has code `PROVIDER_AUTH_MISSING`
- **THEN** narrative includes an error answer item with the sanitized API key message

#### Scenario: Stream error

- **WHEN** `RUN_ERROR` has another non-cancel code
- **THEN** narrative includes an error answer item with the sanitized runtime error

## Modified Requirements

- Thinking and answer items are no longer rendered by the generic narrative article branch.

## Removed Requirements

- None.
