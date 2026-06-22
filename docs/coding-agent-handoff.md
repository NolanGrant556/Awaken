# Coding Agent Handoff

这是 0 上下文 Coding Agent 在 `README.md` 之后应该阅读的第一份文档。

## 当前产品形态

Awaken 是一个本地优先的 Agent 产品原型：

```text
Awaken Desktop
  -> awaken-core local HTTP/SSE runtime
  -> Agent Core
  -> LLM provider + local tools
```

当前项目重点是让本地 run 可观察、可取消、可审批、可产出 artifact，并能被桌面端消费。它不是托管式多用户后端。

Desktop 当前主界面是 Run Workbench：左 Sidebar、中心 Run Narrative、右 Inspector、底部 Composer。不要恢复旧 Dashboard、卡片墙或开发者日志主面板。

## AG-UI / A2UI 一句话边界

Awaken 用 AG-UI 的事件协议格式对外输出 run stream，但实现是 Awaken 自己写的轻量兼容层；不要把 AG-UI / A2UI 外部参考仓库当成要部署或 vendor 的服务。

- 内部事件：`src/agentcore` 的 `AwakenEvent`。
- 协议映射：`src/protocol/agui`。
- SSE 输出：`src/server`。
- Desktop 消费：`apps/desktop`。
- UI 组件：`Awaken Design System` 只收 props/callbacks，不知道 AG-UI。
- A2UI 当前只是只读 `a2ui_surface` payload，不是完整 SDK/runtime 集成。
- 上游参考与固定 commit：`docs/upstream-references.md`。

## 当前实现面

```text
src/agentcore
```

负责 agent 执行循环、LLM request 组装、工具执行、内部 `AwakenEvent` 发射、workspace config、permission-aware `ToolRuntime`。

```text
src/runtime
```

负责 run/session 生命周期、事件历史与回放、取消、approval wait/resolve、runtime event dispatch。

```text
src/protocol/agui
```

负责把内部 Awaken runtime events 映射为 AG-UI-compatible JSON events，用于 SSE 输出。

```text
src/protocol/a2ui
```

负责生成窄范围只读 approval surface payload。它不是完整 A2UI renderer，也不是 SDK 集成。

```text
src/server
```

负责本地 HTTP endpoints 与 SSE streams。

```text
src/tool and src/tools
```

负责工具接口、schema generation、registry、permission decisions、内置工具实现。

```text
apps/desktop
```

Electron + React desktop consumer。它连接 `awaken-core`，订阅 run events，resolve approvals，并把 AG-UI events reduce 成 view state。

```text
Awaken Design System
```

Awaken Design System component library。只接收 props 和 callbacks。

## 当前 Runtime API

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

desktop 通过 Electron preload bridge 或 browser fallback client 消费这些接口。

## 当前 Event Contract

标准 AG-UI-compatible events：

```text
RUN_STARTED
RUN_FINISHED
RUN_ERROR
RUN_STATUS_CHANGED
TEXT_MESSAGE_START
TEXT_MESSAGE_CONTENT
TEXT_MESSAGE_END
REASONING_START
REASONING_MESSAGE_START
REASONING_MESSAGE_CONTENT
REASONING_MESSAGE_END
REASONING_END
TOOL_CALL_START
TOOL_CALL_ARGS_DELTA
TOOL_CALL_ARGS
TOOL_CALL_END
TOOL_PROGRESS
TOOL_CALL_RESULT
```

Awaken extension events：

```text
AWAKEN_COMMAND_OUTPUT
AWAKEN_PATCH_UPDATED
AWAKEN_TURN_DIFF
AWAKEN_APPROVAL_REQUESTED
AWAKEN_APPROVAL_RESOLVED
AWAKEN_ARTIFACT_CREATED
AWAKEN_ARTIFACT_UPDATED
```

approval request 当前还会创建：

```text
artifactKind: "a2ui_surface"
```

desktop renderer 会把这个 payload 作为只读 generated surface 预览。

## 当前 Tool Permission Model

权限模型是显式三态：

```text
allow
ask
deny
```

相关文件：

```text
src/tool/permission.go
src/agentcore/tool_runtime.go
src/tools/apply_patch/apply_patch.go
src/tools/shell/shell.go
```

当前行为：

- 只读 file/search/calculator 工具默认允许。
- `apply_patch` 修改文件前需要审批。
- `shell` 允许窄范围只读命令，复杂或危险命令需要审批。
- `web_search` 不需要审批。
- 工具相对路径按配置的 workspace root 解析。

## 安全第一步

```bash
cd /Users/nolangrant/Awaken
git status --short
go test ./...
go vet ./...
npm run build:desktop
git diff --check
```

UI-only 工作：

```bash
npm run typecheck:ui
npm run build:ui
```

desktop 工作：

```bash
npm --workspace @awaken/desktop run test
npm run typecheck:desktop
npm run build:desktop
```

Run Workbench 收口验收：

```bash
rg -n "Dashboard|card wall|developer log|开发者日志|卡片墙" apps/desktop "Awaken Design System"
go test ./src/server -run 'TestPlainQASmoke|TestReadOnlyFileSmoke|TestFileMutation(Approval|Reject|Artifact|PatchAndTurnDiff)Smoke' -count=1 -v
go test ./src/server -run 'TestRunsEndpointsCreateReplayAndCancel|TestRunEventsAfterSeq|TestRunEventsAfterSeqReconnectMultiFrame' -count=1
git diff --check
```

## 本地运行

启动 desktop：

```bash
npm run dev:desktop
```

单独调试 `awaken-core` 时读 `docs/runtime-development.md`。

启动 Awaken Design System Gallery：

```bash
npm run dev:ui
```

## 必须保留的边界

- `Awaken Design System` 必须保持 UI-only。不能出现 `fetch`、Electron IPC、`window.awaken`、SSE、AG-UI、A2UI imports。
- `apps/desktop` 后续应通过 `@awaken/design-system` 使用组件，不要引用 `Awaken Design System/src`。
- `src/agentcore` 只发内部事件，不 import AG-UI structs。
- `src/protocol/agui` 只做协议映射，不执行工具、不管理 session。
- `src/protocol/a2ui` 是窄范围 payload builder，不是 vendored A2UI dependency。
- Runtime approval 和 tool policy 属于 Go runtime/tool 层，不属于 React components。
- 不要为了理论扩展添加大抽象，除非当前调用方需要。

## 改代码前

1. 先看当前 worktree 和 diff。
2. 判断改动属于哪个 owner：Agent Core、runtime、protocol、server、desktop、UI kit 或 docs。
3. 把改动限制在 owner 边界内。
4. 修改 runtime/protocol/tool 行为时，加或更新聚焦测试。
5. 先跑最小相关检查，交付前再跑更大的验证。

## 现在不要做

- 不要 vendor AG-UI / A2UI 外部参考仓库，也不要把本机参考路径写进代码或配置。
- 不要让 `apps/desktop` 拥有后端 policy。
- 不要让 `Awaken Design System` 依赖 Electron 或 runtime API。
- 不要用 generated A2UI surface 替代当前 approval UI。
- 不要把 Gallery/mock state 当成真实 run 验收。
- 不要提交 secrets 或构建产物。
- 不要清理无关 dirty files。
