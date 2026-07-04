# Slice 4：只读文件任务 Spec

## Why

用户需要让模型自主读取 workspace 内容来理解项目。本切片确保只读工具（`read_file`、`list_files`、`grep_search`）既能被模型自主选择，又受 workspace 边界约束，并且工具调用过程和结果能在 Desktop 中展示。

## What Changes

- UI：RunTimeline 已展示 TOOL_CALL_* 事件，本切片确认只读工具调用过程和结果摘要可见，无 approval。
- Runtime：确保工具调用结果正确回填到 conversation，使模型能基于工具结果继续生成最终回复。
- Tools：`read_file`、`list_files`、`grep_search` 的 workspace guard 已在 Slice 2 落地，本切片补齐只读场景确定性测试与失败路径。
- Agent Core：确保 `tool_use` / `tool_result` 配对校验在只读工具链路中继续通过。
- Smoke：真实 LLM 输入"找出 README 并总结项目"，模型自主调用只读工具并返回可读总结。

## Impact

- 受影响能力：只读文件工具调用、工具结果回填、workspace 边界安全。
- 受影响代码：
  - `apps/desktop/src/renderer/runtime/reducer.ts`
  - `apps/desktop/src/renderer/App.tsx`
  - `src/agentcore/runner.go`
  - `src/context/context.go`
  - `src/runtime/run_service.go`
  - `src/tools/file_read/file_read.go`
  - `src/tools/list_files/list_files.go`
  - `src/tools/grep_search/grep_search.go`

## ADDED Requirements

### Requirement: 只读工具结果回填

The system SHALL feed read-only tool results back into the LLM conversation.

#### Scenario: Model reads a file

- **WHEN** the model calls `read_file` and receives content
- **THEN** the tool result is added to the conversation as a `tool_result`
- **AND** the next LLM request includes the paired `tool_use` / `tool_result`

### Requirement: 只读工具 workspace 边界

The system SHALL prevent read-only tools from accessing paths outside the workspace.

#### Scenario: read_file outside workspace

- **WHEN** the model calls `read_file` with `/etc/hosts`
- **THEN** the tool returns a permission-denied error
- **AND** the error becomes the tool_result content

#### Scenario: list_files outside workspace

- **WHEN** the model calls `list_files` with `~/Desktop`
- **THEN** the tool returns a permission-denied error

#### Scenario: grep_search outside workspace

- **WHEN** the model calls `grep_search` with `/etc`
- **THEN** the tool returns a permission-denied error

### Requirement: 只读任务不产生审批

The system SHALL not request approval for read-only tool calls.

#### Scenario: Read-only run

- **WHEN** a run only involves `read_file`, `list_files`, `grep_search`
- **THEN** no `AWAKEN_APPROVAL_REQUESTED` event is emitted
- **AND** no approval UI appears

### Requirement: Desktop 展示只读工具调用

The system SHALL display read-only tool calls and their results in the run timeline.

#### Scenario: Model lists files

- **WHEN** the model calls `list_files`
- **THEN** the timeline shows a "Tool call" step with name and arguments
- **AND** a "Tool result" step with the result summary

### Requirement: 真实 LLM 只读 smoke

The system SHALL complete a real read-only task with a live LLM.

#### Scenario: Find README and summarize

- **WHEN** the user input is "找出 README 并总结项目"
- **THEN** the model autonomously calls `list_files` and/or `read_file`
- **AND** `TOOL_CALL_START` and `TOOL_CALL_RESULT` events appear
- **AND** no approval or artifact is produced
- **AND** the final summary is readable

## MODIFIED Requirements

无

## REMOVED Requirements

无
