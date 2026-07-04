# Slice 16: Command Output、Patch Updated 与 Turn Diff Spec

## Why

Run Workbench 已经能展示工具准备、执行和完成，但 shell 输出、apply_patch 预览、最终变更仍主要在工具完成后才以摘要形式出现。Slice 16 让命令输出、patch 预览和 run 结束后的 diff 进入事件流，用户可以在 Inspector 中实时看到正在发生的事。

## What Changes

- Tool runtime 新增 command output、patch updated、turn diff 内部事件。
- `shell` 支持 output delta，且 delta 有截断策略。
- `apply_patch` 在审批前发出 patch updated event，包含 patch、summary 与 affected paths。
- run 完成时发出 workspace turn diff event，内容与当前 `git diff` 一致并受大小限制。
- AG-UI mapper 映射 command output / patch updated / turn diff。
- Desktop reducer/shared types 接收新事件。
- Terminal inspector 展示 command output delta。
- Changes inspector 展示 patch updated 与 turn diff。
- 增加 shell/apply_patch/runtime/mapper/reducer 测试和真实 mutation smoke。

## Impact

- 触及 `src/tool`、`src/tools/shell`、`src/tools/apply_patch`、`src/agentcore`、`src/protocol/agui`、Desktop reducer/App/shared types。
- 不实现完整终端模拟器。
- 不改变 approval 安全边界。
- 不改变 apply_patch 的执行语义。

## Requirements

### Requirement: Command output delta

#### Scenario: Shell command streams output

- **WHEN** shell command produces stdout or stderr
- **THEN** runtime emits command output delta events before final tool result
- **AND** each event includes tool call id, tool name, stream name and delta

#### Scenario: Long shell output is capped

- **WHEN** shell output exceeds the configured delta budget
- **THEN** command output delta is truncated
- **AND** renderer receives a truncated marker instead of unbounded output

### Requirement: Patch updated before approval

#### Scenario: apply_patch requests approval

- **WHEN** apply_patch enters approval wait
- **THEN** runtime emits patch updated before the user approves or rejects
- **AND** the event includes raw patch, patch summary and affected paths

#### Scenario: apply_patch rejected

- **WHEN** user rejects apply_patch approval
- **THEN** no changed-file artifact is emitted
- **AND** patch updated remains a preview only

### Requirement: Turn diff

#### Scenario: Run finishes after mutation

- **WHEN** a run completes after an approved file mutation
- **THEN** runtime emits turn diff
- **AND** the diff content matches `git diff` for the workspace at completion time

#### Scenario: Workspace has huge diff

- **WHEN** turn diff exceeds the configured budget
- **THEN** diff content is truncated with an explicit marker

### Requirement: AG-UI mapping

#### Scenario: Runtime emits new events

- **WHEN** AG-UI mapper receives command output, patch updated or turn diff events
- **THEN** outgoing event preserves ids, content, truncation flag and seq

### Requirement: Desktop inspector projection

#### Scenario: Command output arrives

- **WHEN** Desktop receives command output delta
- **THEN** Terminal inspector updates incrementally without waiting for `TOOL_CALL_RESULT`

#### Scenario: Patch and diff arrive

- **WHEN** Desktop receives patch updated or turn diff
- **THEN** Changes inspector shows the patch preview and final diff

## Modified Requirements

- Tool execution progress is no longer limited to status text; selected tools may emit bounded content deltas.
- apply_patch approval UI is supplemented by a raw patch preview in Changes inspector.

## Removed Requirements

- 无。
