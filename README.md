# Awaken

Awaken 是一个本地优先的 Agent runtime 与桌面应用原型。当前仓库有四个活跃面：

- `src/`：Go Agent Core、工具系统、runtime、协议层与本地 server。
- `cmd/awaken-core`：本地 HTTP/SSE runtime 进程。
- `apps/desktop`：Electron + React 桌面端 consumer，当前主界面是 Run Workbench。
- `Awaken Design System`：Awaken Design System package、Gallery、tokens 和组件源码。

## 入口契约

本项目只允许两套本地入口：

1. Awaken 使用入口：启动 `npm run open:desktop`，关闭 `npm run close:desktop`
   - 用户和开发者都从这里打开和关闭同一个 Awaken Desktop。
   - 这个命令会复用或启动 `awaken-dev-desktop` screen 会话，并检查 renderer、Electron、`awaken-core` 和真实窗口。
   - 关闭命令会停止 `awaken-dev-desktop` screen 会话，并清理同一入口留下的 renderer、Electron、`awaken-core` 进程。
   - Desktop 自己启动并管理 `awaken-core`。
   - 底层开发服务命令仍是 `npm run dev:desktop`，但只作为 `open:desktop` 使用的长期进程，不作为日常打开入口。
   - 不要把 `cmd/awaken-core`、Vite renderer 或 Electron 子脚本当成独立使用入口。
   - 不要用普通浏览器打开 `127.0.0.1:5174` 判断 Desktop 是否成功；renderer 需要 `window.awaken` Electron bridge。
2. 组件独立观测入口：`npm run open:ui`
   - 在仓库根目录运行，会启动本地组件观测服务并打开 Gallery。
   - 如果服务已经在跑，会直接打开 `http://127.0.0.1:5173/`。
   - 只需要启动服务、不需要自动打开浏览器时，用 `npm run dev:ui`。
   - 每个组件都有自己的可刷新、可截图 URL。
   - `http://127.0.0.1:5173/` 只会自动落到某个组件 URL，本身不作为独立入口。

## 协议归属

一句话：Awaken 用的是 AG-UI 的事件协议格式，但实现是 Awaken 自己写的轻量兼容层；本项目不部署、不 vendor、不直接依赖 AG-UI 项目。

当前边界：

- 内部事件模型是 Awaken 自己的 `AwakenEvent`，位于 `src/agentcore`。
- 对外 SSE 输出是 AG-UI-compatible JSON events，映射层位于 `src/protocol/agui`。
- Desktop 可以消费 AG-UI-compatible events，但 `Awaken Design System` 组件不知道 AG-UI、SSE、Electron 或 runtime。
- A2UI 目前只作为外部参考，不存在运行时 payload、renderer 或 SDK 集成，也不 vendor A2UI 项目。

## 先读这里

0 上下文 Coding Agent 接手时，先读：

```text
README.md
```

然后按任务继续读：

- 需要项目文件职责总览：读 `docs/01_项目情况说明/项目文件清单/README.md`。
- 改 runtime / protocol / AG-UI 边界：读 `docs/01_项目情况说明/AG-UI&A2UI说明/agui-a2ui-architecture.md`。
- 需要对照 AG-UI / A2UI 上游：读 `docs/01_项目情况说明/AG-UI&A2UI说明/upstream-references.md`。
- 改 desktop：以本 README 的入口契约、验收命令和 `apps/desktop/src` 下真实实现为准。
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
├── scripts/                     # 构建、验收、runtime binary 自动化脚本；不是使用入口
├── Awaken Design System/         # @awaken/design-system，组件源码、tokens、Gallery
├── src/
│   ├── agentcore/               # agent loop、request builder、tool runtime、内部事件
│   ├── context/                 # conversation messages 与 tool result pairing
│   ├── llm/                     # provider transport、streaming、native tool_calls
│   ├── prompt/                  # system prompt 模板与 runtime metadata
│   ├── protocol/
│   │   └── agui/                # AwakenEvent -> AG-UI-compatible events
│   ├── runtime/                 # run/session/event bus/cancel/approval
│   ├── server/                  # 本地 HTTP routes 与 SSE
│   ├── tool/                    # tool interface、registry、schema、permission model
│   └── tools/                   # 内置工具实现
└── package.json                 # npm workspaces：Awaken Design System 与 desktop
```

## 当前 Runtime 契约

Desktop 会自己启动并管理 `cmd/awaken-core`。用户和 Desktop 开发者不需要单独启动 runtime，也不应绕过 Electron bridge 直连 runtime。

Runtime / server 只作为内部工程层测试与验证，不形成第三个使用入口。

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

approval request 只通过 `AWAKEN_APPROVAL_REQUESTED` / `AWAKEN_APPROVAL_RESOLVED` 和 Desktop 原生 `ApprovalCard` 完成，不再生成 generated surface payload。

## Coding Agent 接手规则

0 上下文 Coding Agent 应以本 README 为唯一接手入口。

当前产品形态：

```text
Awaken Desktop
  -> awaken-core local HTTP/SSE runtime
  -> Agent Core
  -> LLM provider + local tools
```

当前主界面是 Run Workbench：左 Sidebar、中心 Run Narrative、右 Inspector、底部 Composer。不要恢复旧 Dashboard、卡片墙或开发者日志主面板。

实现 owner 速查：

- `src/agentcore`：agent 执行循环、LLM request 组装、工具执行、内部 `AwakenEvent` 发射、workspace config、permission-aware `ToolRuntime`。
- `src/runtime`：run/session 生命周期、事件历史与回放、取消、approval wait/resolve、runtime event dispatch。
- `src/protocol/agui`：把内部 Awaken events 映射为 AG-UI-compatible JSON events。
- `src/server`：本地 HTTP endpoints 与 SSE streams。
- `src/tool` 和 `src/tools`：工具接口、schema generation、registry、permission decisions、内置工具实现。
- `apps/desktop`：Electron + React Desktop consumer，通过 `window.awaken` bridge 消费 runtime。
- `Awaken Design System`：UI-only component library，只接收 props 和 callbacks。

改代码前：

1. 先看当前 worktree 和 diff：`git status --short`。
2. 判断改动属于哪个 owner：Agent Core、runtime、protocol、server、desktop、UI kit 或 docs。
3. 把改动限制在 owner 边界内。
4. 修改 runtime/protocol/tool 行为时，加或更新聚焦测试。
5. 先跑最小相关检查，交付前再跑更大的验证。

现在不要做：

- 不要 vendor AG-UI / A2UI 外部参考仓库，也不要把本机参考路径写进代码或配置。
- 不要让 `apps/desktop` 拥有后端 policy。
- 不要让 `Awaken Design System` 依赖 Electron 或 runtime API。
- 不要重新引入 generated A2UI approval surface 替代当前 approval UI。
- 不要把 Gallery/mock state 当成真实 run 验收。
- 不要提交 secrets、依赖目录或构建产物。
- 不要清理无关 dirty files，不要回滚用户未提交改动。

## 配置

模型配置的唯一用户入口是 Awaken Desktop 的 `Settings > Models`。

用户在这里配置 provider、model、API key、上下文窗口和输出上限；Desktop 会把 profile 注入每次 run。正式 Desktop 路径不读取项目根目录的 `.env` 或 `config.toml`，也不会要求用户编辑本地配置文件。

API key 由 Electron `safeStorage` 保存；非敏感 profile 字段保存到 Desktop user data 目录下的 `config/model-profiles.json`。

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

打开组件 Gallery：

```bash
npm run open:ui
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

runtime/server 内部验证：

```bash
go test ./src/runtime ./src/server ./src/agentcore
```

真实 LLM smoke：

```bash
go test ./src/server -run 'TestPlainQASmoke|TestReadOnlyFileSmoke|TestFileMutation(Approval|Reject|Artifact|PatchAndTurnDiff)Smoke' -count=1 -v
```

常用完整验证：

```bash
go test ./... && go vet ./... && go build ./... && npm run build:desktop && git diff --check
```

Run Workbench 收口验收：

```bash
rg -n "Dashboard|card wall|developer log|开发者日志|卡片墙" apps/desktop "Awaken Design System"
go test ./src/server -run 'TestRunsEndpointsCreateReplayAndCancel|TestRunEventsAfterSeq|TestRunEventsAfterSeqReconnectMultiFrame' -count=1
git diff --check
```

## 本地运行

### Awaken 使用入口

```bash
npm run open:desktop
npm run close:desktop
```

`npm run open:desktop` 会复用或启动 `awaken-dev-desktop` screen 会话，并确认 renderer、Electron、`awaken-core` 和真实 Desktop 窗口都已就绪。`npm run close:desktop` 会停止这个正式入口管理的 screen 会话，并清理遗留的 renderer、Electron、`awaken-core` 进程。底层的 `npm run dev:desktop` 会构建 `@awaken/design-system`，确保 `awaken-core` binary 存在，启动 Vite renderer（`127.0.0.1:5174`），编译 Electron main/preload，并启动 Electron；日常不要直接用它打开 Awaken。

### 组件独立观测入口

```bash
npm run open:ui
```

`npm run open:ui` 会复用或启动本地组件观测服务，然后打开 Gallery。每个组件 URL 都可刷新、可截图、可作为视觉回归目标，例如：

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

## Runtime 内部验证配置

这部分只面向 `awaken-core` / Go runtime 的内部测试与验证，不是 Awaken 使用流程，也不是独立启动入口。

- `AWAKEN_DEFAULT_WORKSPACE`：指定 `/v1/runtime/info` 返回的默认 workspace。
- 未设置这些环境变量时，runtime 保持 Go 侧默认行为。
- Runtime smoke 会在测试进程内启动本地 server，不形成 Awaken 的第三个使用入口。

## Desktop 打包状态

`electron-builder` 当前可以完成本地 Desktop 打包；Awaken 自己的 app icon 已配置在 `apps/desktop/build/`：

- `icon-source.png`：原始输入图。
- `icon.png`：运行期 Dock/window 图标和 Linux 图标。
- `icon.icns`：macOS 打包图标。
- `icon.ico`：Windows 图标预留。

正式分发前还有 release packaging readiness 项：

- `apps/desktop/package.json` 还没有完整的 `description` / `author`。
- 本机没有有效的 `Developer ID Application` 证书时，macOS code signing 会跳过，打包产物未签名、未公证。

这些提醒不影响本机开发测试、统一启动流程、真实 LLM run 或 runtime supervisor 验收。正式分发给其他 macOS 用户前，需要补齐 app icon、Developer ID signing 和 notarization，否则可能触发 Gatekeeper 的“无法验证开发者”等提示。

## 硬边界

- 不要把后端、Electron、SSE、AG-UI 或 A2UI 逻辑放进 `Awaken Design System`。
- desktop 不要从 `Awaken Design System/src` 做源码路径 import；后续接入应使用 `@awaken/design-system` 包出口。
- `src/agentcore` 不要 import `src/protocol/agui`，协议映射只属于 `src/protocol/agui`。
- 不要 vendor AG-UI / A2UI 外部参考仓库，也不要把本机参考路径写进代码或配置。
- 不要提交 `.env`、依赖目录或构建产物。
- 不要擅自回滚未提交的用户改动。
