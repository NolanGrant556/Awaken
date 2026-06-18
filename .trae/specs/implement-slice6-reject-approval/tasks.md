# Tasks

- [x] Task 1：UI 强化 Reject 状态展示与防重复点击
  - [x] SubTask 1.1：在 `ApprovalCard` 中增加 `resolved` / `rejected` 状态展示，Reject 后禁用按钮
  - [x] SubTask 1.2：在 `App.tsx` 的 `handleResolveApproval` 中确保 Reject 后正确设置 resolving 状态
  - [x] SubTask 1.3：在 `reducer.ts` 的 `AWAKEN_APPROVAL_RESOLVED` 处理中确认 `rejected` 时 step status 为 `"error"`、tag 为 `"rejected"`
  - [x] SubTask 1.4：确认 Desktop 不把 `rejected` 渲染成 crash 或 generic error toast

- [x] Task 2：Runtime 确认 Reject 路径正确
  - [x] SubTask 2.1：检查 `src/runtime/session.go` 的 `ResolveApproval` 对 `rejected` decision 的处理：状态恢复、事件发射、approval 移除
  - [x] SubTask 2.2：检查 `src/agentcore/tool_runtime.go` 的拒绝后逻辑：`IsError=true`、结果内容正确、不执行 mutation
  - [x] SubTask 2.3：确认拒绝后 `tool_use/tool_result` 配对完整，无 orphan pair

- [x] Task 3：失败路径测试
  - [x] SubTask 3.1：在 `src/runtime/run_service_test.go` 添加测试：重复 resolve 同一 approval 返回 `APPROVAL_NOT_FOUND`
  - [x] SubTask 3.2：在 `src/runtime/run_service_test.go` 添加测试：resolve 未知 `approval_id` 返回 `APPROVAL_NOT_FOUND`
  - [x] SubTask 3.3：在 `src/server/server_test.go` 添加测试：HTTP 层重复 resolve 返回 404
  - [x] SubTask 3.4：在 `src/agentcore/tool_runtime_test.go` 添加测试：Reject 后不执行工具、结果正确、反馈到上下文（已有测试覆盖）

- [x] Task 4：真实 LLM Reject smoke 测试
  - [x] SubTask 4.1：在临时 workspace 创建 `note.md`（内容为 `hello`）
  - [x] SubTask 4.2：使用真实 LLM 启动 run：输入"把 note.md 中 hello 改成 world"
  - [x] SubTask 4.3：在 ApprovalCard 出现时点击 Reject
  - [x] SubTask 4.4：验证 `note.md` 内容仍为 `hello`
  - [x] SubTask 4.5：验证 run 继续并给出可读解释，终态为成功

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/runtime/... ./src/agentcore/... ./src/server/...` 通过
  - [x] SubTask 5.2：Desktop build 与 type check 通过
  - [x] SubTask 5.3：代码审查确认 Reject 路径无遗漏
  - [x] SubTask 5.4：记录真实 LLM smoke 结果

# Task Dependencies

- Task 1 与 Task 2 可并行
- Task 3 依赖 Task 2（需要确认 runtime 行为后再写测试）
- Task 4 依赖 Task 1、Task 2、Task 3
- Task 5 依赖 Task 1、Task 2、Task 3、Task 4
