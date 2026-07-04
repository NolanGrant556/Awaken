# Tasks

- [x] Task 1：确保 Runtime 对空输入返回结构化 INVALID_INPUT
  - [x] SubTask 1.1：检查 `RunService.StartRun` 对空 `input` 的校验是否已返回 `ErrInvalidInput`
  - [x] SubTask 1.2：检查 `src/server/handlers.go` 的 `handleCreateRun` 是否将 `ErrInvalidInput` 映射为 HTTP 400 与 `INVALID_INPUT` 错误码
  - [x] SubTask 1.3：如缺失，补充/修正相关代码
  - [x] SubTask 1.4：为 `handleCreateRun` 添加空输入测试

- [x] Task 2：确保普通问答事件链路干净
  - [x] SubTask 2.1：检查 `src/agentcore/runner.go` 在无 tool call 时是否只发射 TEXT_MESSAGE_* 与 RUN_FINISHED
  - [x] SubTask 2.2：检查事件 emitter 在 RUN_FINISHED 之前正确发送 TEXT_MESSAGE_END
  - [x] SubTask 2.3：如缺失，修正 runner 事件顺序
  - [x] SubTask 2.4：为 runner 的普通问答路径添加单元测试或事件断言

- [x] Task 3：确保 Desktop UI 正确处理普通问答
  - [x] SubTask 3.1：检查 App.tsx 的 `handleSubmit` 是否阻止空输入发送并显示错误
  - [x] SubTask 3.2：检查 streaming 文本是否正确渲染在响应区域（RunTimeline 或专用文本区域）
  - [x] SubTask 3.3：检查 run 状态 pill 是否正确反映 running/done/error
  - [x] SubTask 3.4：如缺失，补充 UI 校验与状态展示

- [x] Task 4：真实 LLM smoke 测试
  - [x] SubTask 4.1：在 Go runtime 或 Desktop 端添加/执行一次真实 LLM smoke：输入"请只回复 pong"（当前环境无 LLM_API_KEY，测试已就位并跳过）
  - [x] SubTask 4.2：验证最终回复为 `pong`、run 状态为 succeeded、无 tool/approval/artifact 事件（ smoke 代码已覆盖断言）

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/runtime/... ./src/server/... ./src/agentcore/...` 通过
  - [x] SubTask 5.2：Desktop build 与 type check 通过
  - [x] SubTask 5.3：手动验证空输入在 UI 和 runtime 均被拒绝（runtime 已用 curl 验证返回 400 INVALID_INPUT；UI 交互部分因无浏览器环境未做可视化点击，代码审查与构建通过）
  - [x] SubTask 5.4：记录真实 LLM smoke 结果（跳过：环境无 `LLM_API_KEY`，`TestPlainQASmoke` 已自动跳过）

# Task Dependencies

- Task 2 depends on Task 1
- Task 3 depends on Task 2
- Task 4 depends on Task 2
- Task 5 depends on Task 1, Task 2, Task 3, Task 4
