# Awaken

Awaken 是一个本地运行的 Agent 原型。当前仓库重点在 Agent Core、System Prompt、Conversation Context、Tool Runtime 和 LLM Client 的职责拆分。

## 目录结构

```text
.
├── CLI/                 # CLI 交互入口
├── docs/                # 架构与设计文档
├── src/
│   ├── agentcore/       # Agent Core：执行循环、请求组装、工具调用
│   ├── context/         # Conversation Context：用户/助手/工具消息历史
│   ├── llm/             # LLM Client：模型请求、streaming、tool_calls 解析
│   ├── prompt/          # System Prompt：Markdown 模板与运行时注入
│   └── tool/            # 工具定义、注册与执行
└── config.toml
```

核心设计说明见：

```bash
docs/agent-core-system-prompt-design.md
```

## 环境配置

在项目根目录创建 `.env`：

```bash
LLM_API_KEY=你的 API Key
LLM_BASE_URL=https://api.example.com/v1
LLM_MODEL_ID=你的模型名
```

当前 CLI 会通过 `llm.LoadConfig()` 读取 `.env` 中的配置。

## 启动 CLI 交互模式

进入项目根目录：

```bash
cd /Users/nolangrant/Awaken
```

启动交互模式：

```bash
go run ./CLI/main.go
```

进入后可以直接输入问题：

```text
> 你好
```

交互模式内置命令：

```text
reset  # 重置当前对话上下文
clear  # 清屏
exit   # 退出
quit   # 退出
```

交互模式会保留同一次会话中的上下文，直到输入 `reset` 或退出程序。

当前 CLI 支持 streaming 输出。如果模型返回 thinking delta，CLI 会显示：

```text
[Thinking]
...
```

然后继续输出最终回答。

## LLM 手动 Smoke 测试

LLM 包下有一个手动 smoke 程序：

```bash
go run ./src/llm/test-llm.go
```

这个文件带有 `//go:build ignore`，不会参与普通 `go test ./...` 编译。

## 测试与验证

运行全部测试：

```bash
go test ./...
```

构建检查：

```bash
go build ./...
```

静态检查：

```bash
go vet ./...
```

常用完整验证：

```bash
go test ./... && go build ./... && go vet ./...
```

## 当前架构要点

- CLI 只负责输入输出，不写 System Prompt。
- System Prompt 静态内容在 Markdown 模板中，动态信息由运行时注入。
- Context 只保存对话历史，不保存 System Prompt。
- Agent Core 每轮调用前组装 `system prompt + messages + tools`。
- LLM Client 支持普通调用、streaming 输出、thinking delta 和 native `tool_calls`。
- XML `<action>` parser 仍保留为兼容 fallback，不是目标核心协议。
