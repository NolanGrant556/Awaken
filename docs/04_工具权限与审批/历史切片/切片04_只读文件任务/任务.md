# Tasks

- [x] Task 1：确保工具调用结果回填到 conversation
  - [x] SubTask 1.1：检查 `src/agentcore/runner.go` 的 tool call 处理流程，确认 tool result 被加入 conversation
  - [x] SubTask 1.2：检查 `src/context/context.go` 是否支持 `ToolUse` / `ToolResult` 消息
  - [x] SubTask 1.3：如缺失，补充 tool result 回填逻辑
  - [x] SubTask 1.4：为 tool_use/tool_result 配对添加单元测试或断言

- [x] Task 2：补齐只读工具 workspace 边界确定性测试
  - [x] SubTask 2.1：检查 `read_file`、`list_files`、`grep_search` 是否已有 workspace 外拒绝测试
  - [x] SubTask 2.2：补充针对 `/etc/hosts`、`~/Desktop`、`/etc` 等真实越界路径的测试
  - [x] SubTask 2.3：补充相对路径解析、路径归一化测试

- [x] Task 3：确认 Desktop UI 展示只读工具调用
  - [x] SubTask 3.1：检查 `reducer.ts` 是否已处理 `TOOL_CALL_START`、`TOOL_CALL_RESULT`、`TOOL_CALL_END`
  - [x] SubTask 3.2：检查 App.tsx 是否正确将 steps 传给 RunTimeline
  - [x] SubTask 3.3：如缺失，补充 UI 渲染或事件映射

- [x] Task 4：真实 LLM 只读 smoke 测试
  - [x] SubTask 4.1：在 Go runtime 或 Desktop 端添加/执行一次真实 LLM smoke：输入"找出 README 并总结项目"
  - [x] SubTask 4.2：验证出现 TOOL_CALL_START/TOOL_CALL_RESULT、无 approval/artifact、最终总结可读

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/agentcore/... ./src/tools/... ./src/runtime/...` 通过
  - [x] SubTask 5.2：Desktop build 与 type check 通过
  - [x] SubTask 5.3：代码审查确认 tool result 回填与 UI 事件展示
  - [x] SubTask 5.4：记录真实 LLM smoke 结果（成功/跳过/失败）

# Task Dependencies

- Task 2 depends on Task 1
- Task 3 can run in parallel with Task 1 and Task 2
- Task 4 depends on Task 1
- Task 5 depends on Task 1, Task 2, Task 3, Task 4
