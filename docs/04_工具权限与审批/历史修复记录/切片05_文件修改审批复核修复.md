# Slice 5 Review 修复计划

## Summary

针对 Slice 5（文件修改审批）review 中发现的必须修、建议修、可选优化项，执行以下修复：

1. 统一执行 `gofmt` 修复 Go 代码格式。
2. 在 `src/runtime/session.go` 中把 approval 登记、状态切换、事件生成做成原子操作；`RequestCancel` 支持从 `awaiting_approval` 切到 `cancelling`；`ResolveApproval` 在 run 已 terminal/cancelling 时不恢复 `running`。
3. 在 `src/tools/apply_patch/apply_patch.go` 中增加 symlink-resolved writable root 校验，防止通过 symlink 写出/删除到 writable root 外。
4. 为 `ResolveApproval` 增加结构化错误（terminal/cancelling），并在 HTTP handler 中用 JSON 错误体返回。
5. 同步把 `AffectedPaths` / `PatchSummary` 写入 A2UI approval surface payload。
6. 优化 `ApprovalCard` 可访问性与中文文案。

## Current State Analysis

### 1. gofmt 问题
- `src/runtime/session.go`：`EventKind` 常量块字段未对齐，常量块后多空行。
- `src/agentcore/events.go`、`src/protocol/agui/events.go`：新增 `ApprovalAffectedPaths`/`ApprovalPatchSummary` 后，其他字段未重新对齐。
- `src/agentcore/runner_test.go`、`src/server/server_test.go`：存在缩进/对齐问题。

### 2. approval 生命周期竞态
- `AwaitApproval` 先 `s.mu.Lock()` 登记 approval 再解锁，然后调用 `recordRunStatusChange` 切换状态。中间存在窗口：
  - `RequestCancel` 可把状态先改成 `cancelling`；随后 `recordRunStatusChange` 又因“非 terminal”把状态改回 `awaiting_approval`，UI 状态错误。
  - 若 run 在此窗口内被 external 置为 terminal，`ResolveApproval` 仍可能把状态改回 `running` 并发出 `RUN_STATUS_CHANGED`。
- `RequestCancel` 只把 `running` 改 `cancelling`，没有处理 `awaiting_approval`。
- `ResolveApproval` 删除 approval 后直接恢复 `running`，未检查 run 是否已 terminal/cancelling。
- `ResolveApproval` 错误路径只有 `ErrApprovalNotFound`，无法区分 run 已结束、正在取消、重复 resolve 等场景。

### 3. apply_patch symlink 逃逸
- `Execute` 中仅通过 `permissionContext.IsWithinWritableRoots(op.path)` 做词法路径检查。
- 文件实际操作在 `src/tools/apply_patch/executor.go`：`deriveUpdatedContent` 用 `os.ReadFile`，`writeFileWithParents` 用 `os.MkdirAll`+`os.WriteFile`，`deleteFile` 用 `os.Remove`。这些调用都会跟随 symlink。
- 攻击示例：workspace 内存在 `link -> /etc`，patch 写 `link/hosts`，词法路径在 workspace 内，实际会写到 `/etc/hosts`。

### 4. A2UI surface 未同步新字段
- `src/protocol/a2ui/surface.go` 的 `ApprovalSurfaceRequest` 没有 `AffectedPaths` / `PatchSummary`。
- `src/runtime/session.go` 的 `approvalSurfaceArtifactEvent` 也未传入这两项。

### 5. ApprovalCard 可访问性与文案
- `aria-describedby` 只指向 description，patch summary 与 affected paths 对屏幕阅读器不可见。
- “Summary / Affected paths” 为英文，与默认中文按钮文案不一致。
- `patchSummary` 由 `buildPatchSummary` 生成英文 `Add 1, Updates 2` 等，不适合中文界面。

## Proposed Changes

### Step 1: gofmt

一次性运行：

```bash
gofmt -w src/runtime/session.go src/agentcore/events.go src/protocol/agui/events.go src/agentcore/runner_test.go src/server/server_test.go
```

### Step 2: 修复 approval 生命周期

文件：`src/runtime/session.go`

#### 2.1 原子化 `AwaitApproval`

把以下三步放到同一把锁内完成：
- 检查 terminal/cancelling；
- 登记 `s.approvals[request.ID]`；
- 设置 `s.run.Status = RunStatusAwaitingApproval`、更新时间、构造 `RUN_STATUS_CHANGED` event；
- 解锁后再 `bus.Append(event, false)`。

伪代码：

```go
now := time.Now().UTC()
s.mu.Lock()
if isTerminalStatus(s.run.Status) || s.run.Status == RunStatusCancelling {
    s.mu.Unlock()
    return "", context.Canceled
}
s.approvals[request.ID] = pending
s.run.Status = RunStatusAwaitingApproval
s.run.UpdatedAt = now
statusEvent := Event{Kind: EventKindRunStatusChanged, Timestamp: now, ThreadID: s.run.ThreadID, RunID: s.run.RunID, Status: string(RunStatusAwaitingApproval)}
bus := s.bus
s.mu.Unlock()
bus.Append(statusEvent, false)
```

然后照常 emit `ApprovalRequestedEventType` 与 A2UI surface artifact。

#### 2.2 `RequestCancel` 支持 `awaiting_approval`

修改状态判断：

```go
if s.run.Status == RunStatusRunning || s.run.Status == RunStatusAwaitingApproval {
    s.run.Status = RunStatusCancelling
}
```

保持调用 `cancel()` 以唤醒 `AwaitApproval`。

#### 2.3 `ResolveApproval` 增加 terminal/cancelling 防护与结构化错误

新增错误变量（`src/runtime/run_service.go` 或 `src/runtime/session.go`）：

```go
var (
    ErrApprovalNotFound = errors.New("approval not found")
    ErrRunTerminal      = errors.New("run is already finished")
    ErrRunCancelling    = errors.New("run is being cancelled")
)
```

`ResolveApproval` 逻辑：

```go
s.mu.Lock()
pending, ok := s.approvals[approvalID]
if !ok {
    s.mu.Unlock()
    return ErrApprovalNotFound
}
delete(s.approvals, approvalID)
request := pending.request

switch {
case isTerminalStatus(s.run.Status):
    s.mu.Unlock()
    return ErrRunTerminal
case s.run.Status == RunStatusCancelling:
    s.mu.Unlock()
    return ErrRunCancelling
}

now := time.Now().UTC()
s.run.Status = RunStatusRunning
s.run.UpdatedAt = now
statusEvent := Event{Kind: EventKindRunStatusChanged, ...}
bus := s.bus
s.mu.Unlock()

bus.Append(statusEvent, false)
// emit AWAKEN_APPROVAL_RESOLVED
pending.decision <- decision
close(pending.decision)
return nil
```

#### 2.4 HTTP handler 返回结构化错误

文件：`src/server/handlers.go`

更新 `handleResolveApproval` 的 error 分支，使用 `writeErrorJSON`：

```go
case errors.Is(err, awruntime.ErrRunNotFound), errors.Is(err, awruntime.ErrApprovalNotFound):
    writeErrorJSON(w, http.StatusNotFound, "APPROVAL_NOT_FOUND", err.Error())
case errors.Is(err, awruntime.ErrRunTerminal):
    writeErrorJSON(w, http.StatusConflict, "RUN_TERMINAL", "run is already finished")
case errors.Is(err, awruntime.ErrRunCancelling):
    writeErrorJSON(w, http.StatusConflict, "RUN_CANCELLING", "run is being cancelled")
default:
    writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to resolve approval")
```

同时把 handler 中现有的 `http.Error` 调用统一成 `writeErrorJSON`（至少 `handleResolveApproval` 内的 bad request）。

#### 2.5 新增/更新测试

文件：`src/runtime/session_test.go`

- 已有 `TestSessionAwaitApprovalTransitionsStatus` 覆盖正常状态切换，保留。
- 新增 `TestSessionAwaitApprovalAtomicWithCancel`：在 `AwaitApproval` 登记后立刻 `RequestCancel`，验证不会出现 `awaiting_approval` 覆盖 `cancelling`。
- 新增 `TestSessionResolveApprovalAfterCancelReturnsCancelling`：在 `RequestCancel` 后调用 `ResolveApproval`，验证返回 `ErrRunCancelling` 且不恢复 `running`。
- 新增 `TestSessionResolveApprovalAfterTerminalReturnsTerminal`：手动调用 `CompleteFinished` 后 `ResolveApproval`，验证返回 `ErrRunTerminal`。

### Step 3: 修复 apply_patch symlink 逃逸

文件：`src/tools/apply_patch/apply_patch.go`、`src/tools/apply_patch/executor.go`

#### 3.1 新增路径校验 helper

在 `apply_patch.go` 或新建 `src/tools/apply_patch/safe_paths.go` 中增加：

```go
// validateMutationPath 在词法 writable root 检查之后、实际 IO 之前调用。
// 对已有的文件/目录解析 symlink，确保解析后的真实路径仍在 writable roots 内。
// 对新增文件，检查最深存在的父目录解析后仍在 writable roots 内。
func validateMutationPath(path string, permissionContext tool.PermissionContext) error
```

实现要点：
- 使用 `os.Lstat` 判断路径本身或父目录是否为 symlink。
- 使用 `filepath.EvalSymlinks` 解析 symlink。
- 解析后的路径通过 `permissionContext.IsWithinWritableRoots` 检查。
- 如果路径/父目录不存在，向上递归到第一个存在的祖先再解析。
- 返回带路径信息的错误，便于用户理解。

#### 3.2 在 `Execute` 中调用校验

在现有词法 `IsWithinWritableRoots` 循环之后、调用 `applyPatch` 之前，对每个 operation 的涉及路径调用 `validateMutationPath`：

- `patchKindAdd`：目标 `op.path`。
- `patchKindDelete`：源 `op.path`。
- `patchKindUpdate`：源 `op.path`；若 `op.movePath != ""`，目标 `op.movePath` 也要校验。

#### 3.3 更新 executor 错误处理（如需要）

`executor.go` 保持现有实现，因为 `Execute` 已在校验通过后才进入 `applyPatch`。如果 helper 设计得当，executor 内无需重复校验。

#### 3.4 新增确定性测试

文件：`src/tools/apply_patch/apply_patch_test.go`

新增 `TestApplyPatchRejectsSymlinkEscape`：

```go
workspace := t.TempDir()
outside := t.TempDir()
permissionContext := tool.NewPermissionContext(workspace, []string{workspace})

// 在 workspace 内创建 link -> outside/note.txt
linkPath := filepath.Join(workspace, "link")
outsideFile := filepath.Join(outside, "note.txt")
os.WriteFile(outsideFile, []byte("secret\n"), 0644)
os.Symlink(outsideFile, linkPath)

// patch 试图通过 link 修改 outside 文件
patch := "*** Begin Patch\n" +
    "*** Update File: link\n" +
    "@@\n" +
    "-secret\n" +
    "+hacked\n" +
    "*** End Patch"

ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
_, err := (&ApplyPatchTool{}).Execute(ctx, patch)
if err == nil { t.Fatal("expected symlink escape error") }
// 验证 outside/note.txt 未被修改
```

再新增一个 add-through-symlink 用例：workspace 内 `dirLink -> outsideDir`，patch add `dirLink/new.txt`，验证写出失败且 outsideDir 下无新文件。

### Step 4: A2UI surface 同步新字段

文件：`src/protocol/a2ui/surface.go`、`src/runtime/session.go`

#### 4.1 扩展 `ApprovalSurfaceRequest`

```go
type ApprovalSurfaceRequest struct {
    SurfaceID     string
    ApprovalID    string
    Title         string
    Description   string
    RiskTone      string
    ToolName      string
    ToolCallID    string
    ApproveLabel  string
    RejectLabel   string
    AffectedPaths []string
    PatchSummary  string
}
```

#### 4.2 在 surface 组件中展示

在 `ApprovalSurface` 函数中，于 description 之后插入两个 `Component{Component: "KeyValue", ...}`：
- `ID: "affected-paths"`, `Label: "Affected paths"`, `Value: strings.Join(request.AffectedPaths, "\n")`。
- `ID: "patch-summary"`, `Label: "Summary"`, `Value: request.PatchSummary`。

更新 `root` Card 的 `Children` 包含这两个新组件。

#### 4.3 更新 `approvalSurfaceArtifactEvent`

```go
content, err := a2ui.ApprovalSurfaceJSON(a2ui.ApprovalSurfaceRequest{
    ApprovalID:    request.ID,
    Title:         request.Title,
    Description:   request.Description,
    RiskTone:      string(request.RiskTone),
    ToolName:      request.ToolName,
    ToolCallID:    request.ToolCallID,
    AffectedPaths: request.AffectedPaths,
    PatchSummary:  request.PatchSummary,
})
```

#### 4.4 更新测试

文件：`src/protocol/a2ui/surface_test.go`

验证 JSON 中 `affected-paths` 与 `patch-summary` 两个 KeyValue 组件存在且值正确。

### Step 5: ApprovalCard 可访问性与中文文案

文件：`packages/ui/src/components/feedback/ApprovalCard.tsx`

#### 5.1 可访问性

- 为 summary 区域增加 `const summaryId = useId();`，为 affected paths 区域增加 `const pathsId = useId();`。
- 构造 `aria-describedby`：

```tsx
const describedBy = [
  description ? descId : undefined,
  patchSummary ? summaryId : undefined,
  affectedPaths?.length ? pathsId : undefined,
].filter(Boolean).join(" ") || undefined;
```

并把 `aria-describedby={describedBy}` 传给 alertdialog。

#### 5.2 中文文案

- “Summary” → “变更摘要”。
- “Affected paths” → “受影响路径”。

#### 5.3 `patchSummary` 中文化（后端）

文件：`src/tools/apply_patch/apply_patch.go`

修改 `buildPatchSummary` 与 `pluralize`：

```go
func buildPatchSummary(patch *patchDocument) string {
    // ...
    if addCount > 0 {
        parts = append(parts, fmt.Sprintf("新增 %d 个文件", addCount))
    }
    if updateCount > 0 {
        parts = append(parts, fmt.Sprintf("修改 %d 个文件", updateCount))
    }
    if deleteCount > 0 {
        parts = append(parts, fmt.Sprintf("删除 %d 个文件", deleteCount))
    }
    if moveCount > 0 {
        parts = append(parts, fmt.Sprintf("移动 %d 个文件", moveCount))
    }
    if len(parts) == 0 {
        return "无变更"
    }
    return strings.Join(parts, "，")
}
```

同步更新 `apply_patch_test.go` 中断言 `PatchSummary` 的测试（如有）。

### Step 6: 可选 — overlay token

文件：`packages/ui/src/styles/tokens.css`（如存在 overlay token 则直接用），`packages/ui/src/components/feedback/ApprovalCard.tsx`

- 若 `tokens.css` 已存在 `--aw-overlay-backdrop` 或类似 token，直接替换 `rgba(0, 0, 0, 0.28)`。
- 若不存在，可新增 `--aw-overlay-backdrop: rgba(0, 0, 0, 0.28)` 到 tokens.css，再在组件中引用。
- 因用户列为可选优化，本计划不强制执行，但建议顺手替换。

## Assumptions & Decisions

1. **ResolveApproval 在 terminal/cancelling 时返回错误但不发送 decision**：依赖 `RequestCancel` 的 `cancel()` 唤醒 `AwaitApproval`，因此不发送 decision 不会导致 goroutine 永久阻塞。
2. **symlink 校验只加在 Execute 中**：CheckPermission 阶段保持词法检查，审批时仍按原逻辑展示 danger/warning；实际执行前再加 symlink-resolved 硬拦截。这符合用户“mutation 前”校验的要求，也避免改动审批 UI 的风险判定。
3. **A2UI surface 需要同步新字段**：因其描述为“Phase 5 只读评估 payload”，包含 patch 详情有助于评估一致性。
4. **patchSummary 改为中文**：当前 ApprovalCard 默认按钮为中文，统一为中文更一致；若后续需要国际化可再抽象。
5. **不强制修复 overlay raw rgba**：列为可选，本次核心修复不包括。

## Verification

执行以下命令验证：

```bash
# 1. gofmt
gofmt -l src/runtime/session.go src/agentcore/events.go src/protocol/agui/events.go src/agentcore/runner_test.go src/server/server_test.go
# 期望无输出

# 2. git diff 无空白错误
git diff --check

# 3. 全量 Go 测试
go test ./...

# 4. 真实 LLM smoke 测试
go test -v ./src/server -run TestFileMutationApprovalSmoke -count=1

# 5. Desktop typecheck / build
npm run typecheck:desktop
npm run build:desktop
```

新增测试应覆盖：
- `AwaitApproval` 与 `RequestCancel` 的竞态；
- `ResolveApproval` 在 terminal/cancelling 后的错误；
- `apply_patch` symlink 逃逸（update/add-through-symlink）；
- A2UI surface 包含 affected paths / patch summary；
- rejection / outside-writable-root 等原有 happy/failure path 回归通过。
