# Tasks

- [x] Task 1：添加 run 状态显式切换 awaiting_approval / running
  - [x] SubTask 1.1：在 `src/runtime/session.go` 增加 `RunStatusAwaitingApproval` 状态
  - [x] SubTask 1.2：在 `Session.AwaitApproval` 进入等待时设置状态为 `awaiting_approval` 并发射状态事件
  - [x] SubTask 1.3：在 `Session.ResolveApproval` 时设置状态回 `running` 并发射状态事件
  - [x] SubTask 1.4：在 `src/server/handlers.go` 的 `mapRuntimeEvent` 或协议层正确映射 `RUN_STATUS_CHANGED` 事件
  - [x] SubTask 1.5：为状态切换添加单元测试

- [x] Task 2：增强 ApprovalCard 展示 affected paths 与 patch 摘要
  - [x] SubTask 2.1：在 `packages/ui/src/types.ts` 的 `ApprovalItem` 类型中增加 `affectedPaths?: string[]` 与 `patchSummary?: string`
  - [x] SubTask 2.2：在 `src/tools/apply_patch/apply_patch.go` 的 `ApprovalRequest` 中补充 affected paths 与 patch summary 信息
  - [x] SubTask 2.3：更新 `apps/desktop/src/renderer/runtime/reducer.ts` 将 affected paths / patch summary 写入 approval item
  - [x] SubTask 2.4：更新 `packages/ui/src/components/feedback/ApprovalCard.tsx` 展示 affected paths 与 patch summary

- [x] Task 3：确认审批前不变、批准后 mutation、拒绝后错误继续
  - [x] SubTask 3.1：检查 `src/agentcore/tool_runtime.go` 的 `awaitApproval` 流程，确认批准前不调用 `selectedTool.Execute`
  - [x] SubTask 3.2：检查拒绝后返回 `IsError=true` 的结果并被 conversation 接收
  - [x] SubTask 3.3：为 happy path 与 reject path 添加 agentcore 层测试

- [x] Task 4：确认越界 writable root 失败路径
  - [x] SubTask 4.1：检查 `apply_patch` 的 `patchHasDangerousChange` 对 workspace 外路径返回 danger
  - [x] SubTask 4.2：为 patch 目标路径超出 `writable_roots` 添加确定性测试

- [x] Task 5：真实 LLM 文件修改 smoke 测试
  - [x] SubTask 5.1：在临时 workspace 创建 `note.md`（内容为 `hello`）
  - [x] SubTask 5.2：使用真实 LLM 启动 run：输入"把 note.md 中 hello 改成 world"
  - [x] SubTask 5.3：验证 `apply_patch` 触发 approval；approve 后 `note.md` 内容变为 `world`；run 完成

- [x] Task 6：修复因 apply_patch Execute 越界检查导致的既有 agentcore 测试失败
  - [x] SubTask 6.1：诊断 `TestRunnerOpenAIApplyPatchCreateAndDelete` 与 `TestRunnerAnthropicApplyPatchToolUse` 失败原因
  - [x] SubTask 6.2：更新测试使其正确设置 `AgentConfig.Workspace.Root` 为 `t.TempDir()`
  - [x] SubTask 6.3：运行 `go test ./src/agentcore/...` 确认全部通过

- [x] Task 7：验证与验收
  - [x] SubTask 7.1：运行 `go test ./src/runtime/... ./src/agentcore/... ./src/tools/...` 通过
  - [x] SubTask 7.2：Desktop build 与 type check 通过
  - [x] SubTask 7.3：代码审查确认 ApprovalCard、状态事件、失败路径
  - [x] SubTask 7.4：记录真实 LLM smoke 结果

# Task Dependencies

- Task 2 depends on Task 1（状态事件先定义清楚）
- Task 3 depends on Task 1
- Task 4 can run in parallel with Task 1
- Task 5 depends on Task 1, Task 2, Task 3, Task 4
- Task 6 depends on Task 4
- Task 7 depends on Task 1, Task 2, Task 3, Task 4, Task 5, Task 6
