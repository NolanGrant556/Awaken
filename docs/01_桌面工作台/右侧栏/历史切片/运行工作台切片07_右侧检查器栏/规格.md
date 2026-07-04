# Slice 7：Right Inspector Rail Spec

## Why

Slice 6 已经把 approval 与 file change 放入中心叙事。中心叙事应该保持干净，只展示执行故事；工具参数、完整结果、文件变更细节和空的 Browser 能力需要放到右侧 Inspector，避免把主线内容撑得过重。

本切片不新增 browser runtime，不改变 tool/runtime policy，只把已有 selection 与已有 runtime records 接到右侧 Inspector Rail。

## What Changes

- UI：新增 `InspectorRail`。
- UI：新增 `InspectorTabs`，包含 `Changes`、`Browser`、`Terminal`、`Files`。
- Desktop：右侧 Inspector 使用 `InspectorRail`/`InspectorTabs`。
- Desktop：tool action、fileChange、approval narrative item 可设置 `inspectorSelection`。
- Desktop：Inspector 根据 selection 展示 tool args/result、file change path/status/summary、approval details。
- Desktop：Browser pane 显示 disabled/empty state，不伪造页面。
- Tests：覆盖 inspector selection projection 与 selected detail 数据。

## Impact

- 用户点击中心叙事项后，右侧显示对应细节。
- 右侧 tab 切换不影响 run streaming 或 reducer state。
- Browser pane 在没有 runtime 时不会误导用户。

## Requirements

### Requirement: Inspector tabs

The system SHALL render a right Inspector Rail with stable tabs.

#### Scenario: Default inspector

- **WHEN** Desktop renders without a selected item
- **THEN** Inspector shows `Changes`、`Browser`、`Terminal`、`Files` tabs
- **AND** Browser tab shows an explicit disabled/empty state

### Requirement: Tool detail selection

The system SHALL show tool detail in the Inspector when a tool narrative item is selected.

#### Scenario: Tool action clicked

- **WHEN** the user clicks a tool or exploring action in the center narrative
- **THEN** `inspectorSelection` points to that tool
- **AND** Terminal/Files detail can show the captured args/result

### Requirement: File change detail selection

The system SHALL show file change details in the Inspector when a file change item is selected.

#### Scenario: File change clicked

- **WHEN** the user clicks a file change item in the center narrative
- **THEN** `inspectorSelection` points to that artifact/file change
- **AND** Changes shows path, status/change type, and summary

### Requirement: Approval detail selection

The system SHALL show approval details in the Inspector when an approval item is selected.

#### Scenario: Approval clicked

- **WHEN** the user clicks an approval item in the center narrative
- **THEN** `inspectorSelection` points to that approval
- **AND** Inspector shows title, affected paths, and patch summary when available

### Requirement: Streaming safety

The system SHALL keep Inspector interactions local to UI selection.

#### Scenario: Switching tabs while streaming

- **WHEN** the user switches Inspector tabs during a run
- **THEN** run streaming state and narrative projection continue unchanged

## Modified Requirements

- Right Inspector content is driven by active tab plus `inspectorSelection`, not only by latest artifact/tool.

## Removed Requirements

- None.
