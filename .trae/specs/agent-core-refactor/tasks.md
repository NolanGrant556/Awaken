# Awaken Agent Core 与 System Prompt 重构 - 实现计划

## [ ] Task 1: 创建目录结构和静态 Prompt 文件
- **Priority**: P0
- **Depends On**: None
- **Description**: 
  - 创建 `src/prompt/templates/default_system.md` 静态提示词文件
  - 创建 `src/agentcore/` 目录
- **Acceptance Criteria Addressed**: AC-1
- **Test Requirements**:
  - `programmatic` TR-1.1: 文件 `src/prompt/templates/default_system.md` 存在且非空
  - `programmatic` TR-1.2: CLI 目录不包含硬编码的 prompt 文本（`rg "SystemPrompt|你是" CLI` 返回空）
- **Notes**: 静态 prompt 应包含 Identity、Operating Principles、Tool Use Rules、File/System Safety、Output Style

## [ ] Task 2: 创建 Prompt Package 核心文件
- **Priority**: P0
- **Depends On**: Task 1
- **Description**: 
  - 创建 `src/prompt/loader.go` - 加载 Markdown prompt 模板
  - 创建 `src/prompt/runtime.go` - 收集动态运行时信息（cwd、date、model、tools）
  - 创建 `src/prompt/effective.go` - 构建最终传入 LLM 的有效 system prompt
  - 调整 `src/prompt/system_prompt.go` - SystemPrompt 类型和基础 API
- **Acceptance Criteria Addressed**: AC-1, AC-4
- **Test Requirements**:
  - `programmatic` TR-2.1: `BuildEffectiveSystemPrompt()` 返回包含动态信息的字符串
  - `programmatic` TR-2.2: 修改 cwd/model/date 后 effective prompt 相应变化
  - `human-judgement` TR-2.3: 代码结构清晰，职责单一
- **Notes**: 动态信息不应污染静态 Markdown 模板

## [ ] Task 3: 创建 Agent Core Request Builder
- **Priority**: P0
- **Depends On**: Task 2
- **Description**: 
  - 创建 `src/agentcore/request_builder.go` - 组装 LLM 请求
  - 输入：AgentConfig、Conversation Context、Prompt RuntimeInfo、Tool Definitions
  - 输出：SystemPrompt、Messages、Tools、Model、Temperature、MaxTokens
- **Acceptance Criteria Addressed**: AC-3
- **Test Requirements**:
  - `programmatic` TR-3.1: `Build()` 返回完整的 LLM 请求结构
  - `programmatic` TR-3.2: 请求包含 system prompt、messages、tools
  - `human-judgement` TR-3.3: Request Builder 不包含业务逻辑，只做装配
- **Notes**: Request Builder 不写 prompt 内容，也不执行工具

## [ ] Task 4: 调整 Context 模块
- **Priority**: P0
- **Depends On**: Task 3
- **Description**: 
  - 修改 `src/context/context.go` - 移除 system prompt 相关逻辑
  - `GetMessages()` 不再返回 system message
  - `New()` 不再自动添加 system prompt 到 messages
- **Acceptance Criteria Addressed**: AC-2
- **Test Requirements**:
  - `programmatic` TR-4.1: `Context.GetMessages()` 返回的消息不包含 system role
  - `programmatic` TR-4.2: `Context.New()` 初始化后 messages 为空（或只有用户消息）
- **Notes**: Context 只保存会话历史，不负责 system prompt

## [ ] Task 5: 调整 Agent Core
- **Priority**: P0
- **Depends On**: Task 3, Task 4
- **Description**: 
  - 修改 `src/agentcore/agent.go` - 使用 Request Builder 组装请求
  - 创建 `src/agentcore/runner.go` - Agent 执行循环
  - 集成 Prompt Package 和 Context
- **Acceptance Criteria Addressed**: AC-3, AC-4
- **Test Requirements**:
  - `programmatic` TR-5.1: Agent.Run() 调用 Request Builder 组装请求
  - `programmatic` TR-5.2: 每轮 LLM 调用前动态生成 effective system prompt
- **Notes**: Agent Core 是唯一组装 LLM 请求的地方

## [ ] Task 6: 实现 Native Tool Calls 支持
- **Priority**: P1
- **Depends On**: Task 5
- **Description**: 
  - 修改 `src/llm/llm.go` - 支持 native tool_calls 格式
  - 修改 `src/agentcore/runner.go` - 优先处理 native tool_calls
  - 保留 `src/agentcore/action_parser.go` 作为 XML 兼容层
- **Acceptance Criteria Addressed**: AC-5
- **Test Requirements**:
  - `programmatic` TR-6.1: LLM 返回 native tool_calls 时正确解析并执行
  - `programmatic` TR-6.2: XML action parser 作为 fallback 正常工作
- **Notes**: 迁移策略：短期保留 XML parser，长期删除

## [ ] Task 7: 调整 CLI 入口
- **Priority**: P1
- **Depends On**: Task 5
- **Description**: 
  - 修改 `CLI/main.go` - 使用新的 Agent Core API
  - CLI 只负责输入输出，不处理 prompt 逻辑
- **Acceptance Criteria Addressed**: AC-1, AC-6
- **Test Requirements**:
  - `human-judgement` TR-7.1: CLI 代码不包含硬编码的 prompt 文本
  - `programmatic` TR-7.2: CLI 交互模式行为不变
- **Notes**: CLI 是最薄的一层，只做输入输出

## [ ] Task 8: 测试与验证
- **Priority**: P0
- **Depends On**: 所有任务
- **Description**: 
  - 运行 `go test ./...` 验证所有测试通过
  - 手动测试 CLI 交互模式
- **Acceptance Criteria Addressed**: AC-6
- **Test Requirements**:
  - `programmatic` TR-8.1: 所有单元测试通过
  - `human-judgement` TR-8.2: CLI 正常工作，对话历史保留
- **Notes**: 确保旧功能不回退

## [ ] Task 9: 删除旧兼容层（可选）
- **Priority**: P2
- **Depends On**: Task 6, Task 8
- **Description**: 
  - 删除 `src/agentcore/action_parser.go`（当 native tool_calls 稳定后）
- **Acceptance Criteria Addressed**: AC-5
- **Test Requirements**:
  - `programmatic` TR-9.1: 无代码引用 `ParseAction`
  - `programmatic` TR-9.2: 所有测试通过
- **Notes**: 需满足删除条件：native tool_calls 完成、测试通过、无引用