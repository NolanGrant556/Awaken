# Tasks

- [x] Task 1：Runtime 发射 cancelling/cancelled 状态事件
  - [x] SubTask 1.1：在 `src/runtime/session.go` 的 `RequestCancel` 中，状态转为 `cancelling` 时发射 `RUN_STATUS_CHANGED` 事件
  - [x] SubTask 1.2：在 `src/runtime/session.go` 的 `CompleteCancelled` 中，发射 `RUN_STATUS_CHANGED(cancelled)` 后再发终态 `EventKindRunCancelled`
  - [x] SubTask 1.3：更新 `src/runtime/run_service_test.go` 和 `src/runtime/session_test.go` 验证新状态事件

- [x] Task 2：Agent Core cancel 时补齐 synthetic tool result
  - [x] SubTask 2.1：在 `src/agentcore/runner.go` 的 `RunStreamWithEvents` 循环中增加 `ctx.Done()` 检查，cancel 时早退
  - [x] SubTask 2.2：cancel 时检查 conversation 中是否有未配对的 tool_use，若有则注入 synthetic tool result（IsError=true，内容 "工具执行已被取消"）
  - [x] SubTask 2.3：确保 `ValidateToolResultPairing` 在 cancel 路径通过
  - [x] SubTask 2.4：在 `src/agentcore/runner_test.go` 新增 cancel 测试：cancel 时有 pending tool_use → synthetic result 注入 → 配对完整

- [x] Task 3：UI 区分 cancelled 终态
  - [x] SubTask 3.1：在 `apps/desktop/src/renderer/runtime/reducer.ts` 的 `RuntimePhase` 类型新增 `"cancelled"`
  - [x] SubTask 3.2：`syncFromRun` 中 `run.status === "cancelled"` 映射到 phase `"cancelled"`（不再落入 error）
  - [x] SubTask 3.3：`RUN_STATUS_CHANGED` handler 处理 `"cancelling"` 和 `"cancelled"` status
  - [x] SubTask 3.4：`RUN_ERROR` handler 中 `code === "RUN_CANCELLED"` 时 phase 设为 `"cancelled"` 而非 `"error"`
  - [x] SubTask 3.5：在 `apps/desktop/src/renderer/App.tsx` 的 `formatStatusLabel` 中 phase `"cancelled"` 显示 "Cancelled"

- [x] Task 4：确定性测试与 smoke
  - [x] SubTask 4.1：确认 `src/runtime/run_service_test.go` 已有 cancel 测试覆盖 running cancel、late race、approval cancel
  - [x] SubTask 4.2：确认 `src/server/server_test.go` 已有 cancel 不存在 run 返回 404 的测试
  - [x] SubTask 4.3：确认 agentcore runner_test.go 新增 cancel 测试验证 synthetic result 与早退

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/runtime/... ./src/agentcore/... ./src/server/...` 通过
  - [x] SubTask 5.2：Desktop build 与 type check 通过
  - [x] SubTask 5.3：代码审查确认 cancel 链路完整

# Task Dependencies

- Task 1 与 Task 2 可并行
- Task 3 依赖 Task 1（UI 需要处理新的状态事件）
- Task 4 依赖 Task 1、Task 2、Task 3
- Task 5 依赖 Task 1、Task 2、Task 3、Task 4
