# Awaken

Awaken 是一个本地优先的 Agent runtime 与桌面应用原型。当前仓库有四个活跃面：

- `src/`：Go Agent Core、工具系统、runtime、协议层与本地 server。
- `cmd/awaken-core`：本地 HTTP/SSE runtime 进程。
- `apps/desktop`：Electron + React 桌面端 consumer，当前主界面是 Run Workbench。
- `Awaken Design System`：Awaken Design System package、Gallery、tokens 和组件源码。

## 入口契约

本项目只允许两套本地入口：

1. Awaken 使用入口：`npm run dev:desktop`
   - 用户和开发者都从这里打开同一个 Awaken Desktop。
   - Desktop 自己启动并管理 `awaken-core`。
   - 不要把 `cmd/awaken-core`、Vite renderer 或 Electron 子脚本当成独立使用入口。

2. 组件独立观测入口：`http://127.0.0.1:5173/#/components/<component-id>`
   - 先用 `npm run dev:ui` 启动本地组件观测服务。
   - 每个组件都有自己的可刷新、可截图 URL。
   - `http://127.0.0.1:5173/` 只会自动落到某个组件 URL，本身不作为独立入口。

## 协议归属

一句话：Awaken 用的是 AG-UI 的事件协议格式，但实现是 Awaken 自己写的轻量兼容层；本项目不部署、不 vendor、不直接依赖 AG-UI 项目。

当前边界：

- 内部事件模型是 Awaken 自己的 `AwakenEvent`，位于 `src/agentcore`。
- 对外 SSE 输出是 AG-UI-compatible JSON events，映射层位于 `src/protocol/agui`。
- Desktop 可以消费 AG-UI-compatible events，但 `Awaken Design System` 组件不知道 AG-UI、SSE、Electron 或 runtime。
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
- 改 Awaken Design System / UI 组件库：读 `Awaken Design System/README.md`。

开始改代码前先看真实工作区：

```bash
cd /Users/nolangrant/Awaken
git status --short
```

当前仓库可能存在未提交实现。除非用户明确要求撤回或丢弃，否则以当前工作区为事实来源。

## 目录结构

```text
.
├── cmd/
│   └── awaken-core/             # 本地 HTTP/SSE runtime 进程
├── apps/
│   └── desktop/                 # Electron main/preload/renderer consumer
├── docs/                        # 当前架构与接手指引
├── Awaken Design System/         # @awaken/design-system，组件源码、tokens、Gallery
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
└── package.json                 # npm workspaces：Awaken Design System 与 desktop
```

## 当前 Runtime 契约

Desktop 会自己启动并管理 `cmd/awaken-core`。用户和 Desktop 开发者不需要单独启动 runtime，也不应绕过 Electron bridge 直连 runtime。

Runtime / server 只作为内部工程层测试与验证，说明见 `docs/runtime-development.md`。

当前 HTTP surface：

```text
GET  /health
GET  /v1/agui/demo
GET  /v1/runtime/info
GET  /v1/threads
POST /v1/runs
GET  /v1/runs/{run_id}
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
AWAKEN_COMMAND_OUTPUT
AWAKEN_PATCH_UPDATED
AWAKEN_TURN_DIFF
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

### Awaken 使用入口

```bash
npm run dev:desktop
```

`npm run dev:desktop` 会构建 `@awaken/design-system`，确保 `awaken-core` binary 存在，启动 Vite renderer（`127.0.0.1:5174`），编译 Electron main/preload，并启动 Electron。

### 组件独立观测入口

先启动本地组件观测服务：

```bash
npm run dev:ui
```

然后打开具体组件 URL。每个组件 URL 都可刷新、可截图、可作为视觉回归目标，例如：

```text
http://127.0.0.1:5173/#/components/awaken-composer-bar
http://127.0.0.1:5173/#/components/awaken-execution-feed
```

`http://127.0.0.1:5173/` 只会自动落到某个组件 URL，本身不作为独立入口。

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

- 不要把后端、Electron、SSE、AG-UI 或 A2UI 逻辑放进 `Awaken Design System`。
- desktop 不要从 `Awaken Design System/src` 做源码路径 import；后续接入应使用 `@awaken/design-system` 包出口。
- `src/agentcore` 不要 import `src/protocol/agui`，协议映射只属于 `src/protocol/agui`。
- 不要 vendor AG-UI / A2UI 外部参考仓库，也不要把本机参考路径写进代码或配置。
- 不要提交 `.env`、依赖目录或构建产物。
- 不要擅自回滚未提交的用户改动。

## 当前文档

```text
docs/README.md                    # docs 索引
docs/coding-agent-handoff.md      # 0 上下文 Coding Agent 接手指引
docs/agui-a2ui-architecture.md    # 当前 protocol/runtime/desktop 边界
Awaken Design System/README.md    # Awaken Design System 当前主规范
docs/upstream-references.md       # AG-UI / A2UI 上游参考与固定 commit
```
