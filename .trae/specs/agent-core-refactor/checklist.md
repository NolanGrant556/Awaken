# Awaken Agent Core 与 System Prompt 重构 - 验证清单

## 核心功能验证

- [ ] Checkpoint 1: 静态 Prompt 文件存在 - `src/prompt/templates/default_system.md`
- [ ] Checkpoint 2: CLI 目录不包含硬编码的 prompt 文本 - `rg "SystemPrompt|你是" CLI` 返回空
- [ ] Checkpoint 3: Context.GetMessages() 不返回 system role 消息
- [ ] Checkpoint 4: Request Builder 正确组装 LLM 请求（system prompt + messages + tools）
- [ ] Checkpoint 5: 动态信息（cwd/date/model/tools）正确注入 effective prompt
- [ ] Checkpoint 6: Native tool calls 优先处理，XML 作为 fallback
- [ ] Checkpoint 7: 所有单元测试通过 - `go test ./...`
- [ ] Checkpoint 8: CLI 交互模式正常工作，对话历史保留

## 目录结构验证

- [ ] Checkpoint 9: `src/prompt/templates/default_system.md` 存在
- [ ] Checkpoint 10: `src/prompt/loader.go` 存在
- [ ] Checkpoint 11: `src/prompt/runtime.go` 存在
- [ ] Checkpoint 12: `src/prompt/effective.go` 存在
- [ ] Checkpoint 13: `src/agentcore/request_builder.go` 存在
- [ ] Checkpoint 14: `src/agentcore/runner.go` 存在
- [ ] Checkpoint 15: `src/agentcore/agent.go` 存在（使用 Request Builder）

## 代码质量验证

- [ ] Checkpoint 16: 代码编译通过 - `go build ./...`
- [ ] Checkpoint 17: 无未使用的导入 - `go vet ./...`
- [ ] Checkpoint 18: CLI 代码职责单一（只做输入输出）
- [ ] Checkpoint 19: Agent Core 是唯一组装 LLM 请求的地方
- [ ] Checkpoint 20: Context 只保存会话历史，不处理 prompt