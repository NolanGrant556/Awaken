# Awaken

Awaken 是一个本地优先的 Agent runtime 与桌面应用原型。当前仓库有四个活跃面：

- `src/`：Go Agent Core、工具系统、runtime、协议层与本地 server。
- `cmd/awaken-core`：本地 HTTP/SSE runtime 进程。
- `apps/desktop`：Electron + React 桌面端 consumer。
- `packages/ui`：UI-only React 组件库。

## 协议归属

一句话：Awaken 用的是 AG-UI 的事件协议格式，但实现是 Awaken 自己写的轻量兼容层；本项目不部署、不 vendor、不直接依赖 AG-UI 项目。

当前边界：

- 内部事件模型是 Awaken 自己的 `AwakenEvent`，位于 `src/agentcore`。
- 对外 SSE 输出是 AG-UI-compatible JSON events，映射层位于 `src/protocol/agui`。
- Desktop 可以消费 AG-UI-compatible events，但 `packages/ui` 组件库不知道 AG-UI、SSE、Electron 或 runtime。
- A2UI 当前只用于窄范围只读 `a2ui_surface` payload 评估，不是完整 A2UI SDK 集成，也不 vendor A2UI 项目。

## 先读这里

0 上下文 Coding Agent 接手时，先读：

```text
README.md
docs/README.md
docs/coding-agent-handoff.md
```

然后按任务继续读：

- 改 runtime / protocol / AG-UI / A2UI：读 `docs/agui-a2ui-architecture.md`。
- 需要对照 AG-UI / A2UI 上游：读 `docs/upstream-references.md`。
- 改 desktop：读 `apps/desktop/README.md`。
- 改 UI 组件库：读 `packages/ui/README.md`。

开始改代码前先看真实工作区：

```bash
cd /Users/nolangrant/Awaken
git status --short
```

当前仓库可能存在未提交实现。除非用户明确要求撤回或丢弃，否则以当前工作区为事实来源。

## 目录结构

```text
.
├── CLI/                         # 本地交互式 CLI 入口
├── cmd/
│   └── awaken-core/             # 本地 HTTP/SSE runtime 进程
├── apps/
│   └── desktop/                 # Electron main/preload/renderer consumer
├── docs/                        # 当前架构与接手指引
├── packages/
│   └── ui/                      # @awaken/ui，UI-only React 组件库
├── src/
│   ├── agentcore/               # agent loop、request builder、tool runtime、内部事件
│   ├── context/                 # conversation messages 与 tool result pairing
│   ├── llm/                     # provider transport、streaming、native tool_calls
│   ├── prompt/                  # system prompt 模板与 runtime metadata
│   ├── protocol/
│   │   ├── agui/                # AwakenEvent -> AG-UI-compatible events
│   │   └── a2ui/                # 窄范围只读 generated surface payload
│   ├── runtime/                 # run/session/event bus/cancel/approval
│   ├── server/                  # 本地 HTTP routes 与 SSE
│   ├── tool/                    # tool interface、registry、schema、permission model
│   └── tools/                   # 内置工具实现
├── config.toml                  # 非敏感本地 LLM/runtime 配置
└── package.json                 # npm workspaces：ui 与 desktop
```

## 当前 Runtime 契约

Desktop 会自己启动并管理 `cmd/awaken-core`。用户和 Desktop 开发者不需要单独启动 runtime。

Runtime 开发调试入口见 `docs/runtime-development.md`。

当前 HTTP surface：

```text
GET  /health
GET  /v1/agui/demo
POST /v1/runs
GET  /v1/runs/{run_id}/events
POST /v1/runs/{run_id}/cancel
POST /v1/runs/{run_id}/approvals/{approval_id}/resolve
```

`/v1/runs/{run_id}/events` 输出 AG-UI-compatible SSE events。Awaken 目前有这些 extension events：

```text
AWAKEN_APPROVAL_REQUESTED
AWAKEN_APPROVAL_RESOLVED
AWAKEN_ARTIFACT_CREATED
AWAKEN_ARTIFACT_UPDATED
```

approval request 还会生成一个只读的 `artifactKind: "a2ui_surface"` payload，用于 desktop generated surface 评估。

## 配置

敏感信息放 `.env`：

```bash
LLM_API_KEY=your_api_key
```

非敏感配置放 `config.toml`：

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

`.env` 已被 git ignore，不能提交。

## 开发命令

安装 npm workspace 依赖：

```bash
npm install
```

Go 测试与检查：

```bash
go test ./...
go vet ./...
go build ./...
```

UI package 检查：

```bash
npm run typecheck:ui
npm run build:ui
```

desktop 检查：

```bash
npm run typecheck:desktop
npm run build:desktop
```

常用完整验证：

```bash
go test ./... && go vet ./... && go build ./... && npm run build:desktop && git diff --check
```

## 本地运行

交互式 CLI：

```bash
go run ./CLI/main.go
```

UI 组件库 playground：

```bash
npm run dev:ui
```

打开：

```text
http://127.0.0.1:5173/
```

桌面端：

```bash
npm run dev:desktop
```

`npm run dev:desktop` 会构建 `@awaken/ui`，确保 `awaken-core` binary 存在，启动 Vite renderer（`127.0.0.1:5174`），编译 Electron main/preload，并启动 Electron。

## 当前工具面

模型可见的内置工具：

```text
calculator
read_file
list_files
grep_search
apply_patch
shell
web_search
```

只读工具不应要求审批。会修改文件或有风险的工具通过 `src/tool/permission.go` 与 `src/agentcore/tool_runtime.go` 走权限模型。

当前权限规则：

- `apply_patch` 修改文件前需要审批。
- `shell` 允许窄范围只读命令，复杂或危险命令需要审批。
- `web_search` 不需要审批。
- 相对路径按配置的 workspace root 解析。

## 硬边界

- 不要把后端、Electron、SSE、AG-UI 或 A2UI 逻辑放进 `packages/ui`。
- desktop 不要从 `packages/ui/src` 做源码路径 import，要用 `@awaken/ui` 包出口。
- `src/agentcore` 不要 import `src/protocol/agui`，协议映射只属于 `src/protocol/agui`。
- 不要 vendor AG-UI / A2UI 外部参考仓库，也不要把本机参考路径写进代码或配置。
- 不要提交 `.env`、依赖目录或构建产物。
- 不要擅自回滚未提交的用户改动。

## 当前文档

```text
docs/README.md                    # docs 索引
docs/coding-agent-handoff.md      # 0 上下文 Coding Agent 接手指引
docs/agui-a2ui-architecture.md    # 当前 protocol/runtime/desktop 边界
docs/upstream-references.md       # AG-UI / A2UI 上游参考与固定 commit
docs/component-library.md         # 组件库实现档案
```
