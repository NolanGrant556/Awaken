# Slice 6：拒绝审批 Spec

## Why

Slice 5 已完成文件修改审批的 Approve 路径，但 Reject 路径需要被显式强化：用户点击 Reject 后文件绝不能被修改，agent 必须收到合法的 tool result 以维持 tool_use/tool_result pairing，且 Desktop 不能把 rejected 渲染成 runtime crash。本切片在已有基础设施上，把 Reject 路径做到完整、可测试、可验收。

## What Changes

- UI：ApprovalCard 在 Reject 后禁用按钮、显示 rejected 状态，避免重复点击。
- Runtime：拒绝后不执行 mutation，保持 run 状态正确恢复。
- Agent Core：拒绝后生成合法 tool result（IsError=true），保持 tool_use/tool_result pairing。
- Event：`AWAKEN_APPROVAL_RESOLVED` 带 `rejected` decision，Desktop reducer 正确渲染为 rejected 而非 crash。
- 测试：新增确定性测试覆盖 Reject 路径、重复 resolve、未知 approval_id 等失败路径。
- 真实 LLM smoke：覆盖 Reject 后 run 继续并给出可读解释。

## Impact

- 受影响能力：文件修改审批的 Reject 路径、run 状态恢复、tool result 配对完整性。
- 受影响代码：
  - `packages/ui/src/components/feedback/ApprovalCard.tsx`
  - `apps/desktop/src/renderer/App.tsx`
  - `apps/desktop/src/renderer/runtime/reducer.ts`
  - `src/runtime/session.go`
  - `src/runtime/run_service.go`
  - `src/agentcore/tool_runtime.go`
  - `src/server/handlers.go`
  - `src/server/server_test.go`
  - `src/agentcore/tool_runtime_test.go`
  - `src/runtime/run_service_test.go`

## ADDED Requirements

### Requirement: Reject 后文件完全不变

The system SHALL NOT modify any file when the user rejects an approval.

#### Scenario: User rejects apply_patch

- **WHEN** `apply_patch` is waiting for approval
- **AND** the user clicks Reject
- **THEN** no file on disk is changed
- **AND** the file content remains exactly the same as before the tool call

### Requirement: Reject 后生成合法 tool result

The system SHALL generate a valid tool result after rejection to maintain tool_use/tool_result pairing.

#### Scenario: Reject produces error tool result

- **WHEN** the user rejects an approval
- **THEN** the tool result has `IsError=true`
- **AND** the content is `"工具执行已被审批拒绝"`
- **AND** the result is fed back into the conversation context
- **AND** subsequent LLM requests do not produce orphan tool_use/tool_result pairs

### Requirement: UI 正确显示 Rejected 状态

The system SHALL render the rejected state clearly in the UI without treating it as a runtime crash.

#### Scenario: Desktop shows rejected state

- **WHEN** the user clicks Reject
- **THEN** the ApprovalCard buttons become disabled during resolution
- **AND** after `AWAKEN_APPROVAL_RESOLVED` with `rejected` decision arrives
- **THEN** the approval card is removed
- **AND** the timeline step shows status `"error"` with tag `"rejected"`
- **AND** the run continues (or ends) with a readable explanation
- **AND** Desktop does NOT display a runtime crash or generic error toast

### Requirement: Runtime 正确处理 Rejected decision

The system SHALL handle rejected decisions correctly in the runtime state machine.

#### Scenario: Reject resolves approval without mutation

- **WHEN** `ResolveApproval` is called with `rejected` decision
- **THEN** the run status changes from `awaiting_approval` back to `running`
- **AND** a `RUN_STATUS_CHANGED` event is emitted with status `running`
- **AND** an `AWAKEN_APPROVAL_RESOLVED` event is emitted with `ApprovalDecision: "rejected"`
- **AND** the pending approval is removed from the session
- **AND** no mutation is executed

### Requirement: 重复 resolve 返回结构化错误

The system SHALL reject duplicate or invalid approval resolution attempts.

#### Scenario: Double-resolve rejected

- **WHEN** an approval has already been resolved (approved or rejected)
- **AND** a second `ResolveApproval` call is made with the same `approval_id`
- **THEN** the API returns `APPROVAL_NOT_FOUND` (404)
- **AND** the response body contains a structured error

#### Scenario: Unknown approval_id

- **WHEN** `ResolveApproval` is called with an `approval_id` that does not exist
- **THEN** the API returns `APPROVAL_NOT_FOUND` (404)
- **AND** the response body contains a structured error

### Requirement: 真实 LLM Reject smoke 测试

The system SHALL verify the complete Reject path with a live LLM.

#### Scenario: Reject apply_patch and run continues

- **GIVEN** a workspace with `note.md` containing `hello`
- **WHEN** the user inputs "把 note.md 中 hello 改成 world"
- **THEN** the model calls `apply_patch`
- **AND** an ApprovalCard appears
- **WHEN** the user clicks Reject
- **THEN** `note.md` still contains `hello`
- **AND** the run continues with the rejection result fed back
- **AND** the model provides a readable explanation that the modification was rejected
- **AND** the run completes successfully (not crashed or failed)

## MODIFIED Requirements

无

## REMOVED Requirements

无
