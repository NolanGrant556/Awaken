# Slice 5：文件修改审批 Spec

## Why

用户需要让 Awaken 修改文件，但修改前必须被明确告知将要改什么并手动批准。本切片在已有 runtime approval 基础上，强化 UI 展示、run 状态显式切换以及失败路径处理。

## What Changes

- UI：增强 `ApprovalCard`，展示 affected paths、risk tone、patch 摘要。
- Runtime：在 approval 等待期间将 run 状态显式切换为 `awaiting_approval`；resolve 后恢复 `running`。
- API：复用已有的 `POST /v1/runs/{run_id}/approvals/{approval_id}/resolve`。
- Tools：`apply_patch` 触发审批；批准后执行 mutation；拒绝后返回错误结果。
- Agent Core：tool_use / tool_result 配对继续通过。

## Impact

- 受影响能力：文件修改审批、run 状态、apply_patch 执行流程。
- 受影响代码：
  - `packages/ui/src/components/feedback/ApprovalCard.tsx`
  - `packages/ui/src/types.ts`
  - `apps/desktop/src/renderer/App.tsx`
  - `apps/desktop/src/renderer/runtime/reducer.ts`
  - `src/runtime/session.go`
  - `src/runtime/run_service.go`
  - `src/agentcore/tool_runtime.go`
  - `src/tools/apply_patch/apply_patch.go`
  - `src/protocol/agui/event_mapper.go`

## ADDED Requirements

### Requirement: ApprovalCard 展示 patch 详情

The system SHALL display affected paths and a patch summary in the approval card.

#### Scenario: apply_patch requests approval

- **WHEN** `apply_patch` triggers `AWAKEN_APPROVAL_REQUESTED`
- **THEN** the approval card shows the list of affected paths
- **AND** shows a patch summary (add / update / delete / move)
- **AND** shows the risk tone

### Requirement: Run 状态显式切换 awaiting_approval

The system SHALL emit a run status event when entering and leaving the approval wait state.

#### Scenario: Run waits for approval

- **WHEN** a tool call requires approval
- **THEN** the run status changes from `running` to `awaiting_approval`
- **AND** a `RUN_STATUS_CHANGED` event is emitted with status `awaiting_approval`

#### Scenario: Approval resolved

- **WHEN** the user approves or rejects
- **THEN** the run status changes back to `running`
- **AND** a `RUN_STATUS_CHANGED` event is emitted with status `running`

### Requirement: 审批前文件不变

The system SHALL NOT modify files before approval.

#### Scenario: User has not approved

- **WHEN** `apply_patch` is waiting for approval
- **THEN** no file on disk is changed

### Requirement: Approve 后执行最小 diff

The system SHALL apply the patch after approval and produce the expected diff.

#### Scenario: User approves

- **WHEN** the user clicks Approve
- **THEN** `apply_patch` executes
- **AND** file content matches the patch
- **AND** only the expected files are changed

### Requirement: 拒绝后 run 继续

The system SHALL treat a rejected approval as a tool error and continue the run.

#### Scenario: User rejects

- **WHEN** the user clicks Reject
- **THEN** the tool result is an error: "工具执行已被审批拒绝"
- **AND** the run continues with the tool result fed back to the model

### Requirement: 越界 writable root 失败路径

The system SHALL reject `apply_patch` that touches paths outside writable roots.

#### Scenario: Patch targets outside writable root

- **WHEN** `apply_patch` targets a path outside `writable_roots`
- **THEN** the permission check returns `danger` risk tone
- **AND** approval does not allow the patch to escape writable roots

### Requirement: 真实 LLM 文件修改 smoke

The system SHALL complete a real file modification task with a live LLM.

#### Scenario: Replace hello with world

- **WHEN** the user input is "把 note.md 中 hello 改成 world"
- **THEN** the model calls `apply_patch`
- **AND** an approval card appears
- **AND** after approval, `note.md` content has `world` instead of `hello`
- **AND** the run completes successfully

## MODIFIED Requirements

无

## REMOVED Requirements

无
