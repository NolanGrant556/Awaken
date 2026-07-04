# Slice 6 Reject 路径修复计划

## Summary

修复 Slice 6（拒绝审批）代码审查中发现的问题：1 个中等严重度的 UI 状态残留 bug、resolving 期间交互未真正禁用、以及补强 reject 专用确定性测试与 smoke 断言。不引入新框架，不改动 runtime/agentcore 的核心逻辑（已验证正确）。

## Current State Analysis

### 已验证正确的部分（不动）
- `src/runtime/session.go:259-313` `ResolveApproval` 不分支于 decision，状态恢复/事件发射/approval 移除均正确
- `src/agentcore/tool_runtime.go:84-88` reject 分支提前 return，不执行 mutation，生成 `IsError=true` + content `"工具执行已被审批拒绝"`
- `apps/desktop/src/renderer/runtime/reducer.ts:476-489` `AWAKEN_APPROVAL_RESOLVED` 正确产出 `status="error"`/`tag="rejected"`
- `src/server/handlers.go:113-149` 重复/未知 approval_id 返回 404 + `APPROVAL_NOT_FOUND`

### 待修复的问题
1. **`resolvedApprovalDecision` 状态残留**（[App.tsx:88-90](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx#L88-L90)）：仅在 `handleResolveApproval` 和 `onNewTask` 重置；`handleSubmit` 新建 run、收到新 `AWAKEN_APPROVAL_REQUESTED` 时未重置 → 后续审批卡按钮被错误禁用。
2. **resolving 期间交互未真正禁用**（[ApprovalCard.tsx](file:///Users/nolangrant/Awaken/packages/ui/src/components/feedback/ApprovalCard.tsx)）：仅有 `resolvedDecision` 控制 disabled，resolving 期间按钮/Escape/背景点击仍可触发。
3. **缺少 reject 专用确定性测试**：`run_service_test.go` 现有 resolve 测试均用 approved decision。
4. **重复 resolve 测试缺 reject 分支**：service 层与 HTTP 层测试均用 approved。
5. **smoke 测试断言不够严密**：未断言 `AWAKEN_APPROVAL_RESOLVED.approvalDecision == "rejected"` 与 `TOOL_CALL_RESULT.content == "工具执行已被审批拒绝"`。

## Proposed Changes

### Change 1: 修复 `resolvedApprovalDecision` 状态残留（必须修）

**文件**: `apps/desktop/src/renderer/App.tsx`

**What**: 把 `resolvedApprovalDecision` 从单值改为带 `approvalId` 的结构，确保只对当前 approval 生效；并在所有应重置的时机清空。

**Why**: 当前实现下，run 1 拒绝审批后，`handleSubmit` 新建 run 2 或同一 run 内出现第二个 `AWAKEN_APPROVAL_REQUESTED` 时，`resolvedApprovalDecision` 仍为 `"rejected"`，导致新审批卡按钮被禁用、显示"已拒绝"，用户无法操作。

**How**:
1. 将 state 类型从 `"approved" | "rejected" | null` 改为 `{ approvalId: string; decision: "approved" | "rejected" } | null`。
2. 在 `AWAKEN_APPROVAL_RESOLVED` 处理处（line 225-235），设置时带上 `event.approvalId`。
3. 传给 `ApprovalCard` 的 `resolvedDecision` prop 前，用 `resolvedApprovalDecision?.approvalId === activeApproval?.id` 过滤，不匹配时传 `undefined`。
4. 在 `handleSubmit`（line 197-203 附近）增加 `setResolvedApprovalDecision(null)`。
5. 在收到新的 `AWAKEN_APPROVAL_REQUESTED` 事件时（在 onEvent 中增加分支），重置 `setResolvedApprovalDecision(null)`。
6. `handleResolveApproval` 的 catch 分支已重置，保持不变。

### Change 2: resolving 期间真正禁用审批交互（必须修）

**文件**: `packages/ui/src/components/feedback/ApprovalCard.tsx`、`apps/desktop/src/renderer/App.tsx`

**What**: 给 `ApprovalCard` 增加 `resolving?: boolean` prop，resolving 时禁用两个按钮、阻止 Escape 与背景点击触发 reject。

**Why**: 当前 resolving 期间仅 label 变为 "Resolving..."，但按钮 `disabled` 只看 `resolvedDecision`，Escape/背景点击仍会调用 `handleReject()`。虽有 ref 守卫防重复提交，但视觉与 spec "buttons become disabled during resolution" 不符。

**How**:
1. `ApprovalCard.tsx`：
   - props 增加 `resolving?: boolean`。
   - 计算 `const interactiveDisabled = Boolean(resolvedDecision) || Boolean(resolving);`
   - 两个按钮 `disabled={interactiveDisabled}`，cursor/opacity 同步。
   - `handleApprove`/`handleReject` 开头增加 `if (resolving) return;`（保留 `resolvedDecision` 检查）。
   - Escape key handler 增加 `if (resolving) return;`（在 `!resolvedDecision` 检查之前）。
   - 背景点击 `onClick` 增加 `if (resolving) return;` 守卫。
   - useEffect 依赖数组加入 `resolving`。
2. `App.tsx`：
   - 传 `resolving={resolvingApprovalId === activeApproval?.id}` 给 `ApprovalCard`。

### Change 3: 补 reject 专用确定性测试（应该修）

**文件**: `src/runtime/run_service_test.go`

**What**: 新增 `TestResolveApprovalRejectedRestoresRunningAndEmitsResolved` 测试。

**Why**: 现有 `TestRunServiceResolveApprovalUnblocksExecutorAndReplaysEvents` 用 approved decision，未覆盖 reject decision 的事件序列。虽 `ResolveApproval` 不分支于 decision，但补 rejected 变体可防回归。

**How**:
- 复用 `TestRunServiceResolveApprovalUnblocksExecutorAndReplaysEvents` 的 executor 模式（await approval → emit RunFinished）。
- 调用 `service.ResolveApproval(run.RunID, "approval-1", agentcore.ApprovalDecisionRejected)`。
- 断言事件序列：`RunStatusChanged(running)` → `ApprovalResolved` 且 `ApprovalDecision == rejected`。
- 不需要断言 artifact（reject 不产出 artifact，executor 收到 rejected decision 后直接 RunFinished）。

### Change 4: 补重复 resolve 的 reject 分支（应该修）

**文件**: `src/runtime/run_service_test.go`、`src/server/server_test.go`

**What**:
- service 层：新增 `TestResolveApprovalDuplicateRejectedReturnsNotFound`。
- HTTP 层：新增 `TestResolveApprovalDuplicateRejectedHTTPReturnsNotFound`。

**Why**: 现有重复 resolve 测试两次都用 approved decision，未覆盖"先 reject 再重复 resolve"。虽 `delete` 不分支于 decision，但补 reject 变体贴合 spec 措辞。

**How**:
- 复用现有 duplicate 测试结构，把首次 resolve 的 decision 改为 `agentcore.ApprovalDecisionRejected` / HTTP body `{"decision":"rejected"}`。
- 断言第二次 resolve 返回 `ErrApprovalNotFound` / HTTP 404 + `APPROVAL_NOT_FOUND`。

### Change 5: 强化真实 LLM smoke 的关键断言（应该修）

**文件**: `src/server/server_test.go`

**What**: 在 `TestFileMutationRejectSmoke` 中增加对 `AWAKEN_APPROVAL_RESOLVED.approvalDecision` 与 `TOOL_CALL_RESULT.content` 的显式断言。

**Why**: 当前 smoke 验证了文件没改、run 继续、模型解释，但未显式断言拒绝事件 decision 与 tool result content，回归保护不够严密。

**How**:
1. 在事件循环中增加变量：
   - `sawApprovalResolvedRejected bool`
   - `rejectedToolResultContent string`
   - `approvalResolvedCount int` / `approvalRequestedCount int`（用于多 approval 场景）
2. `case agui.EventAwakenApprovalResolved:`：断言 `ev.ApprovalDecision == "rejected"`，计数 +1。
3. `case agui.EventToolCallResult:`：若 `ev.ToolCallName == "apply_patch"` 且 content 含 `"审批拒绝"`，记录 content。
4. 循环后断言：
   - `sawApprovalResolvedRejected == true`
   - `rejectedToolResultContent == "工具执行已被审批拒绝"`
   - `approvalResolvedCount == approvalRequestedCount`（每个被请求的 approval 都有对应 resolved）
5. 注意：tool result content 可能因 mapper 拼接而包含额外文本，用 `strings.Contains` 而非精确等于。

### Change 6: 前端测试与 Escape/背景点击（可选增强，本计划不做）

- 前端无测试框架，不引入新框架。保留手动验收步骤（见 Verification）。
- Escape/背景点击即 Reject 的 UX 问题非 Slice 6 必须项，不在本计划范围。

## Assumptions & Decisions

1. **不改动 runtime/agentcore 核心逻辑**：已验证 reject 路径在 `session.go`、`tool_runtime.go`、`runner.go` 中正确，无需修改。
2. **不改动 reducer.ts**：已验证 `AWAKEN_APPROVAL_RESOLVED` 处理正确产出 `status="error"`/`tag="rejected"`。
3. **`resolvedApprovalDecision` 改为带 approvalId 的结构**而非简单在多处重置：更健壮，避免遗漏重置点；且能正确处理"resolved event 到达时 activeApproval 已切换"的边界。
4. **smoke 断言用 `strings.Contains` 匹配 tool result content**：因 mapper 可能拼接前缀，精确匹配易脆。
5. **不引入前端测试框架**：项目当前无 vitest/jest 配置，引入新框架超出本修复范围。
6. **遵循 project_memory 约束**：不依赖 backend/Electron IPC/window.awaken/fetch；使用现有 tokens；不提交代码。

## Verification Steps

### 自动化验证
```bash
# 1. Go 确定性测试（含新增 reject 测试）
cd /Users/nolangrant/Awaken && go test ./src/runtime/... ./src/agentcore/... ./src/server/... -run 'TestResolveApproval|TestReject|TestFileMutationReject' -v -count=1

# 2. 真实 LLM smoke（需 LLM_API_KEY）
cd /Users/nolangrant/Awaken && go test ./src/server -run TestFileMutationRejectSmoke -count=1 -v

# 3. Desktop typecheck + build
cd /Users/nolangrant/Awaken && npm run typecheck:desktop
cd /Users/nolangrant/Awaken && npm run build:desktop

# 4. UI 包 typecheck
cd /Users/nolangrant/Awaken/packages/ui && npm run typecheck

# 5. 检查无空白符错误
cd /Users/nolangrant/Awaken && git diff --check
```

### 人工验收（最关键）
1. 启动 runtime + Desktop。
2. 发起一个会触发 `apply_patch` 的任务（如"把 note.md 中 hello 改成 world"）。
3. ApprovalCard 出现，点击 Reject。
4. 验证：文件未改、按钮变为"已拒绝"且禁用、run 继续并给出解释。
5. **关键场景**：如果模型重试 apply_patch 触发第二个 ApprovalCard，或用户在 Composer 直接输入新 prompt 发起 run 2，第二个 ApprovalCard 必须可操作（按钮可点击、不显示"已拒绝"）。
6. **resolving 场景**：点击 Approve/Reject 后、resolved event 到达前，按钮应立即禁用，Escape/背景点击不应触发额外 reject。
