# Checklist

- [x] 空 input 提交到 runtime 返回 HTTP 400 与 `INVALID_INPUT`
- [x] `RunService.StartRun` 对空 input 返回 `ErrInvalidInput`
- [x] `handleCreateRun` 有空 input 的确定性测试
- [x] 普通问答 run 的事件流为 `RUN_STARTED -> TEXT_MESSAGE_* -> RUN_FINISHED`
- [x] 普通问答 run 不产生 `TOOL_CALL_START` 或 `AWAKEN_APPROVAL_REQUESTED`
- [x] `RUN_FINISHED` 前正确发送 `TEXT_MESSAGE_END`
- [x] runner 普通问答路径有事件断言测试
- [x] Desktop UI 阻止空输入发送并显示可读错误
- [x] Desktop UI 正确展示 streaming 回复文本
- [x] Desktop UI 正确展示 run 状态（running/done/error）
- [ ] 真实 LLM smoke：输入"请只回复 pong"最终回复为 `pong`（环境无 `LLM_API_KEY`，`TestPlainQASmoke` 已跳过）
- [ ] 真实 LLM smoke：run 状态为 succeeded，无 approval/artifact（环境无 `LLM_API_KEY`，`TestPlainQASmoke` 已跳过）
- [x] `go test ./src/runtime/... ./src/server/... ./src/agentcore/...` 通过
- [x] Desktop build 与 type check 通过
