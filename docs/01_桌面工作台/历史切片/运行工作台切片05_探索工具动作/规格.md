# Slice 5：Exploring 与工具动作归并 Spec

## Why

Slice 4 已经让普通问答以 thinking/answer 展示。下一步是只读项目探索：用户不应该在中心看到一堆裸露 tool result/debug log，而应该看到一个 `Exploring` 叙事组，里面是可扫读的动作摘要。原始 args/result 进入右侧 Inspector。

本切片只处理现有只读工具：`read_file`、`list_files`、`grep_search`。不改变 Go runtime、tool permission、SSE 协议或工具执行策略。

## What Changes

- Desktop：`RunNarrativeProjector` 将连续只读工具事件归并为 `exploring` item。
- Desktop：每个只读 tool call 生成稳定 action id。
- Desktop：Run Stage 为 exploring item 渲染 action 摘要列表。
- Desktop：Inspector 保存 latest tool 的 raw args 和 raw result。
- Tests：覆盖 read/list/search 连续事件归并成一个 exploring group，且 replay 不重复。

## Impact

- `read_file`、`list_files`、`grep_search` 不再作为普通 tool card 进入中心主叙事。
- 非只读工具仍保留当前 tool item 行为，后续 Slice 6 处理 approval/file change。

## Requirements

### Requirement: Read-only tools become exploring group

The system SHALL group read-only tool calls into one exploring narrative item.

#### Scenario: Consecutive read-only tools arrive

- **WHEN** `list_files`、`grep_search`、`read_file` tool events arrive consecutively
- **THEN** narrative active item is one `exploring` item
- **AND** the item contains one action per tool call

### Requirement: Stable action ids

The system SHALL generate stable action ids from tool call ids.

#### Scenario: Same read-only event batch is replayed

- **WHEN** the same sequenced tool events are reduced twice
- **THEN** exploring action ids remain unique
- **AND** no duplicate actions are inserted

### Requirement: Main narrative stays summarized

The system SHALL avoid showing raw tool result JSON or long output in center narrative.

#### Scenario: Tool result arrives

- **WHEN** `TOOL_CALL_RESULT` contains long content
- **THEN** the exploring action shows a short status/title summary
- **AND** the full result remains outside the center narrative

### Requirement: Inspector keeps args and result

The Desktop Inspector SHALL retain the latest tool args and result.

#### Scenario: Tool args and result arrive

- **WHEN** `TOOL_CALL_ARGS` and `TOOL_CALL_RESULT` arrive
- **THEN** the latest tool inspector record contains raw args
- **AND** it contains raw result content

## Modified Requirements

- Read-only tool calls no longer render as standalone `tool` narrative items.

## Removed Requirements

- None.
