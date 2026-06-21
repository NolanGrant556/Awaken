# Slice 6：Approval 与 File Change 进入叙事 Spec

## Why

Slice 5 已经让只读探索进入 `Exploring`。当 agent 要修改文件时，用户需要在主叙事中看到审批请求，并在 approve/reject 后看到清晰的状态与文件变更，而不是只靠右侧 Inspector 或弹窗。

本切片不改变 runtime permission policy，不新增持久“总是允许/拒绝”规则，只把现有 approval/artifact events 投影到 Run Narrative。

## What Changes

- UI：`ApprovalCard` 增加 inline narrative 展示模式，默认 modal 行为不变。
- UI：新增 `FileChangeSummary`。
- Desktop：approval item 在中心使用 inline `ApprovalCard` 渲染。
- Desktop：fileChange item 使用 `FileChangeSummary` 渲染 changed files。
- Desktop：artifact/file-change 继续进入 Inspector artifact list。
- Tests：覆盖 approve/reject/cancel/stream error 的 approval cleanup 与 file change projection。

## Impact

- 用户可直接在中心叙事里处理 pending approval。
- Approve 后主叙事显示文件变更摘要。
- Reject 或 cancel 后不保留可点击的 pending approval 控件。

## Requirements

### Requirement: Inline approval narrative

The system SHALL render pending approval in the center narrative.

#### Scenario: Approval requested

- **WHEN** `AWAKEN_APPROVAL_REQUESTED` arrives
- **THEN** Run Stage shows an approval item
- **AND** the item can approve or reject the active runtime approval

#### Scenario: Approval resolved

- **WHEN** `AWAKEN_APPROVAL_RESOLVED` arrives
- **THEN** the approval item is committed as approved or rejected
- **AND** it is no longer clickable

### Requirement: File change summary

The system SHALL render file change artifacts in the center narrative.

#### Scenario: Artifact created or updated

- **WHEN** `AWAKEN_ARTIFACT_CREATED` or `AWAKEN_ARTIFACT_UPDATED` arrives
- **THEN** Run Stage shows changed file path, title, and status

### Requirement: Approval cleanup

The system SHALL not leave interactive approval UI after terminal states.

#### Scenario: Run cancelled

- **WHEN** a run is cancelled while approval is pending
- **THEN** runtime approvals are cleared
- **AND** center narrative does not expose active approve/reject controls

#### Scenario: Stream error

- **WHEN** stream error occurs while approval is pending
- **THEN** runtime approvals are cleared
- **AND** center narrative does not expose active approve/reject controls

### Requirement: Path consistency

The system SHALL keep approval affected paths and artifact paths consistent.

#### Scenario: Apply patch approved

- **WHEN** apply_patch emits approval affected paths and artifact path
- **THEN** artifact path is visible in file change summary
- **AND** server smoke confirms it appears in approval affected paths

## Modified Requirements

- Approval action controls move into the center narrative item instead of relying on the global modal overlay.

## Removed Requirements

- None.
