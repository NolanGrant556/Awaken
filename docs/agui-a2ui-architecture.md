# AG-UI / A2UI Runtime Boundary

本文描述 Awaken 当前 protocol boundary。它是实现契约，不是历史记录。

## 核心定位

一句话：Awaken 用 AG-UI-compatible event names 和 payload shape 作为对外事件格式，但 runtime、adapter、SSE server 和 desktop consumer 都是 Awaken 自己实现的代码。

- AG-UI 是 Awaken 对客户端输出的事件格式，不是本项目要运行的服务。
- Awaken 不 vendor AG-UI 外部参考仓库，也不让 Agent Core 直接依赖 AG-UI structs。
- 内部协议是 `AwakenEvent`；外部协议是 `src/protocol/agui` 输出的 AG-UI-compatible JSON events。
- A2UI 当前只作为窄范围只读 generated surface payload。
- Awaken 保留自己的内部事件模型，位于 `src/agentcore`。
- 协议 adapter 位于 `src/protocol`。
- `packages/ui` 里的组件不知道 AG-UI、A2UI、SSE、Electron 或 Agent runtime。

## 当前事件链路

```text
Agent Core
  -> AwakenEvent
  -> runtime session/event bus
  -> protocol adapter
  -> HTTP/SSE
  -> desktop runtime client
  -> renderer reducer
  -> @awaken/ui props
```

AG-UI 只出现在协议层和 desktop consumer 边界：

```text
src/protocol/agui
apps/desktop/src/shared/agui.ts
apps/desktop/src/renderer/runtime/agui.ts
apps/desktop/src/renderer/runtime/reducer.ts
```

## 当前 Runtime API

`cmd/awaken-core` 启动本地 runtime server。默认地址：

```text
http://127.0.0.1:8787
```

Routes：

```text
GET  /health
GET  /v1/agui/demo
POST /v1/runs
GET  /v1/runs/{run_id}/events
POST /v1/runs/{run_id}/cancel
POST /v1/runs/{run_id}/approvals/{approval_id}/resolve
```

Route owner：

```text
src/server/router.go
src/server/handlers.go
src/server/sse.go
```

Run/session owner：

```text
src/runtime/run_service.go
src/runtime/session.go
src/runtime/event_bus.go
```

## 内部 Awaken Events

当前内部事件定义在：

```text
src/agentcore/events.go
src/agentcore/event_sink.go
```

当前事件族：

```text
RunStarted
RunFinished
RunError

TextMessageStarted
TextDelta
TextMessageFinished

ReasoningStarted
ReasoningDelta
ReasoningFinished

ToolCallStarted
ToolCallArgs
ToolCallFinished
ToolResult

ApprovalRequested
ApprovalResolved

ArtifactCreated
ArtifactUpdated
```

`agentcore` 只能发内部事件，不应 import `src/protocol/agui`。

## AG-UI Mapping

当前 Go mapper：

```text
src/protocol/agui/mapper.go
src/protocol/agui/events.go
src/protocol/agui/encoder.go
```

当前映射：

```text
RunStarted            -> RUN_STARTED
RunFinished           -> RUN_FINISHED
RunError              -> RUN_ERROR

TextMessageStarted    -> TEXT_MESSAGE_START
TextDelta             -> TEXT_MESSAGE_CONTENT
TextMessageFinished   -> TEXT_MESSAGE_END

ReasoningStarted      -> REASONING_START + REASONING_MESSAGE_START
ReasoningDelta        -> REASONING_MESSAGE_CONTENT
ReasoningFinished     -> REASONING_MESSAGE_END + REASONING_END

ToolCallStarted       -> TOOL_CALL_START
ToolCallArgs          -> TOOL_CALL_ARGS
ToolCallFinished      -> TOOL_CALL_END
ToolResult            -> TOOL_CALL_RESULT

ApprovalRequested     -> AWAKEN_APPROVAL_REQUESTED
ApprovalResolved      -> AWAKEN_APPROVAL_RESOLVED

ArtifactCreated       -> AWAKEN_ARTIFACT_CREATED
ArtifactUpdated       -> AWAKEN_ARTIFACT_UPDATED
```

`AWAKEN_*` 是 Awaken extension events。保持显式，并用测试锁定。

## Approval And Artifact Contract

Runtime approval wait 位于：

```text
src/runtime/session.go
src/runtime/run_service.go
```

Tool permission decision 位于：

```text
src/tool/permission.go
src/agentcore/tool_runtime.go
```

当前 approval flow：

```text
ToolRuntime decides ask
  -> ApprovalAwaiter.AwaitApproval
  -> runtime emits ApprovalRequested
  -> runtime emits read-only a2ui_surface artifact
  -> desktop shows ApprovalCard
  -> user resolves approval
  -> POST /v1/runs/{run_id}/approvals/{approval_id}/resolve
  -> runtime emits ApprovalResolved
  -> ToolRuntime continues or returns rejected result
```

Artifact event 是 runtime 可见记录，不等于自动写文件，也不能当作可信可执行内容。

## A2UI Evaluation Boundary

当前 A2UI Go 代码：

```text
src/protocol/a2ui/surface.go
```

当前 desktop A2UI 代码：

```text
apps/desktop/src/renderer/a2ui/surface.ts
apps/desktop/src/renderer/a2ui/A2UISurfaceView.tsx
```

当前 A2UI 行为：

- Runtime 生成只读 approval surface artifact。
- Artifact kind 是 `a2ui_surface`。
- Payload 使用窄范围 v0.9-style JSON shape。
- Desktop 只校验和渲染小白名单。
- Generated surface action 不执行。
- 真正的 approval decision 仍走 `ApprovalCard` 和 runtime resolve API。

支持的 renderer 白名单：

```text
Card
Column
Row
Text
Badge
KeyValue
Button
```

没有明确产品需求前，不要把它扩成通用 renderer。

## Desktop Consumer Boundary

Desktop 代码位于：

```text
apps/desktop/src/main
apps/desktop/src/preload
apps/desktop/src/renderer
apps/desktop/src/shared
```

职责：

- Electron main 创建窗口，并代理 runtime HTTP/SSE 访问。
- Preload 暴露 `window.awaken`。
- Renderer 创建 run、订阅 run events、取消 run、resolve approvals，并把 events reduce 成 view state。
- Renderer 把普通 props 传给 `@awaken/ui`。

Desktop app 可以知道 AG-UI event shape，但 UI library components 不可以知道。

## UI Package Boundary

UI package：

```text
packages/ui
```

允许：

- Components。
- Tokens 和 styles。
- 局部组件交互。
- Props 和 callbacks。
- Platform-aware visual affordances。

不允许：

- `fetch`。
- SSE 或 WebSocket。
- Electron IPC。
- `window.awaken`。
- AG-UI 或 A2UI imports。
- Agent runtime logic。

## 外部参考项目

AG-UI 和 A2UI 是外部参考项目，不是 Awaken 的 vendored source，也不是运行时必需依赖。

Fresh GitHub clone 如果需要对照上游协议，应按 `docs/external-references.md` 在 Awaken repo 外部 clone 并 checkout 固定 commit。

引用说明位于：

```text
docs/external-references.md
```

## Verification

Protocol/runtime/tool 改动：

```bash
go test ./...
go vet ./...
go build ./...
```

Desktop 或 renderer 改动：

```bash
npm run typecheck:desktop
npm run build:desktop
```

UI package 改动：

```bash
npm run typecheck:ui
npm run build:ui
```

始终运行：

```bash
git diff --check
```
