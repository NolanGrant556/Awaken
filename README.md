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
│   ├── tool/            # 工具系统公共层：接口、schema、注册、校验
│   └── tools/           # 具体工具实现与内置工具聚合注册
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
```

`.env` 只负责敏感信息。模型、协议和输出长度等非敏感配置放在 `config.toml`：

```toml
[llm]
base_url = "https://api.deepseek.com/anthropic"
model_id = "deepseek-v4-pro"
protocol = "anthropic_messages"
auth_scheme = "auto"

[model]
context_window = 400000
temperature = 0.7
max_output_tokens = 8192
auto_compact_token_limit = 360000
```

当前 CLI 会通过 `llm.LoadConfig()` 读取 `.env` 和 `config.toml`。如果 `config.toml` 缺少旧字段，仍会 fallback 到旧环境变量 `LLM_BASE_URL`、`LLM_MODEL_ID`、`LLM_PROTOCOL`、`LLM_AUTH_SCHEME`，便于平滑迁移。

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

## 当前工具面

模型可见的内置工具包括：

```text
calculator     # 数学计算
read_file      # 读取文件
list_files     # 按 glob 模式查找文件
grep_search    # 按正则搜索文件内容
apply_patch    # 使用 Codex 风格 patch 新增、修改、移动、删除文件
shell          # 高级 shell 兜底
web_search     # 网络搜索
```

文件任务的推荐顺序：

```text
读取文件 -> read_file
查找文件 -> list_files
搜索内容 -> grep_search
新增/修改/移动/删除文件 -> apply_patch
高级系统操作 -> shell
```

`write_file` 已删除；文件新增、修改、移动、删除统一通过 `apply_patch` 完成。`apply_patch` 支持 Codex 风格的 `Add File`、`Update File`、`Move to`、`Delete File` 和 `End of File`。

文件工具带有基础防洪保护：`read_file` 支持 `offset`、`limit`、`max_bytes` 并拒绝二进制文件；`list_files` 和 `grep_search` 默认跳过常见依赖、构建和版本控制目录；`grep_search` 会裁剪超长匹配行；所有工具结果写回上下文前还会经过统一 head/tail 截断。

## CLI CRUD Smoke 示例

启动 CLI 后，可以用这些输入验证文件工具链：

```text
读取 README.md
搜索 package agentcore
帮我看看项目里工具相关代码在哪
运行一个会产生大量输出的命令，并确认输出会被截断
请使用 apply_patch 在项目根目录创建 tmp_cli_patch_test.md，内容写入 Awaken patch smoke test
把 tmp_cli_patch_test.md 内容改为 Awaken patch update smoke test
删除 tmp_cli_patch_test.md
```

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
- `src/tool` 管工具契约、schema 和 registry，`src/tools/*` 放具体工具实现。
- LLM Client 支持普通调用、streaming 输出、thinking delta 和 native `tool_calls`。
- XML `<action>` parser 仍保留为兼容 fallback，不是目标核心协议。
