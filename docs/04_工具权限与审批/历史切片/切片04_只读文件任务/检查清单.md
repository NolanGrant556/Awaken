# Checklist

- [x] Tool result 被正确加入 conversation
- [x] 下一 LLM 请求包含配对的 tool_use / tool_result
- [x] `read_file` workspace 外读取返回权限错误
- [x] `list_files` workspace 外 root 返回权限错误
- [x] `grep_search` workspace 外 root 返回权限错误
- [x] 只读工具调用不产生 `AWAKEN_APPROVAL_REQUESTED`
- [x] Desktop RunTimeline 展示 TOOL_CALL_START / TOOL_CALL_RESULT / TOOL_CALL_END
- [x] 工具结果摘要在 UI 可见
- [x] 真实 LLM smoke：输入"找出 README 并总结项目"触发只读工具调用
- [x] 真实 LLM smoke：出现 TOOL_CALL_START 和 TOOL_CALL_RESULT
- [x] 真实 LLM smoke：无 approval 与 artifact
- [x] `go test ./src/agentcore/... ./src/tools/... ./src/runtime/...` 通过
- [x] Desktop build 与 type check 通过
- [x] 只读工具 workspace guard 有确定性测试覆盖
