# Awaken AG-UI / A2UI 架构边界

本文用于固定 Awaken 接入 AG-UI、评估 A2UI 时的代码边界、文件树和阶段计划。

核心结论：

- AG-UI 是 Awaken runtime 对外输出的事件协议，不是要部署的 Agent 服务，也不是 UI 页面。
- A2UI 是后续可评估的“Agent 生成声明式 UI surface”方案，不是 Awaken 主界面的基础。
- Awaken 必须先保留自己的内部事件模型，再由 adapter 映射成 AG-UI JSON event。
- `packages/ui` 继续保持 UI-only，不接后端、不接 Electron IPC、不接 SSE/WebSocket。

## 当前现状

Awaken 当前是本地优先的 Agent runtime 原型：

- Go Agent Core 在 `src/agentcore`。
- LLM streaming、thinking delta、native tool calls 解析在 `src/llm`。
- 工具契约和工具实现在 `src/tool`、`src/tools`。
- React 组件库已经独立为 `packages/ui`。
- 未来桌面客户端边界预留在 `apps/desktop`。

当前真正缺口不是组件库，而是 runtime 还没有结构化事件流。现在的 stream 形态能表达文本和 thinking delta，但不足以稳定表达：

- run lifecycle
- tool call lifecycle
- tool result
- approval
- artifact
- cancellation
- workspace/runtime policy

因此第一阶段要先补内部事件模型和协议输出，不要先搬 AG-UI 仓库，也不要先上 A2UI。

## 总体设计

Awaken 的事件链路应该是：

```text
Agent Core
  -> AwakenEvent
  -> Protocol Adapter
  -> Transport
  -> Desktop/Web Client
  -> @awaken/ui Components
```

AG-UI 只存在于 protocol adapter 边界：

```text
AwakenEvent -> AG-UI JSON event
```

不要让 Agent Core 直接依赖 AG-UI；不要让 UI 组件库知道 AG-UI。

## 目标文件树

```text
/Users/nolangrant/Awaken
├── cmd/
│   └── awaken-core/
│       └── main.go
│
├── src/
│   ├── agentcore/
│   │   ├── agent.go
│   │   ├── runner.go
│   │   ├── events.go
│   │   ├── event_sink.go
│   │   └── ...
│   │
│   ├── runtime/
│   │   ├── session.go
│   │   ├── run_service.go
│   │   ├── event_bus.go
│   │   └── store.go
│   │
│   ├── protocol/
│   │   └── agui/
│   │       ├── events.go
│   │       ├── mapper.go
│   │       ├── encoder.go
│   │       └── README.md
│   │
│   ├── server/
│   │   ├── router.go
│   │   ├── handlers.go
│   │   ├── sse.go
│   │   └── middleware.go
│   │
│   ├── context/
│   ├── llm/
│   ├── tool/
│   └── tools/
│
├── apps/
│   └── desktop/
│       ├── main/
│       ├── preload/
│       └── renderer/
│
└── packages/
    └── ui/
        ├── src/
        ├── playground/
        └── demos/
```

这是目标方向，不要求一次性全部建完。后续实现应按阶段逐步引入。

## 职责边界

### `src/agentcore`

职责：Agent 执行循环。

可以做：

- 组装 LLM request。
- 处理 LLM response。
- 通过 `ToolRuntime` 执行工具。
- 写入 conversation context。
- 发出 Awaken 内部事件，例如 `RunStarted`、`TextDelta`、`ToolCallStarted`、`ToolResult`。

不可以做：

- import `src/protocol/agui`。
- 知道 SSE、WebSocket、HTTP 或 Electron。
- 直接输出 AG-UI JSON。
- 管理前端状态。

### `src/runtime`

职责：应用级 session / run 编排。

可以做：

- 创建 session、thread、run。
- 把一次用户输入转成一次 Agent run。
- 存储或回放 run events。
- 把事件分发给监听者。
- 承担 approval、workspace access、cancel 等 runtime policy。

不可以做：

- 放 AG-UI JSON struct。
- 渲染 UI。
- 绕过 `agentcore` 直接执行工具。

### `src/protocol/agui`

职责：AG-UI 兼容层。

可以做：

- 定义 Awaken 需要输出的最小 AG-UI JSON struct。
- 映射 `agentcore.Event` 或 `runtime.Event` 到 AG-UI event。
- 编码 JSON 和 SSE frame。
- 写测试锁定 wire format。

不可以做：

- 调 LLM。
- 执行工具。
- 管理 session。
- import Electron 或 React。
- vendor AG-UI monorepo。

### `src/server`

职责：本地 HTTP API 和 streaming transport。

可以做：

- 定义路由。
- 校验请求。
- 通过 SSE 输出 AG-UI-compatible events。
- 暴露 health / readiness endpoint。
- 把 handler 接到 `src/runtime`。

不可以做：

- 组装 Agent prompt。
- 放本应属于 `runtime` 或 `agentcore` 的业务逻辑。
- import UI package。

### `cmd/awaken-core`

职责：进程启动入口。

可以做：

- 加载配置。
- 创建 runtime service。
- 启动本地 HTTP server。
- 后续处理 graceful shutdown。

不可以做：

- 写 handler 逻辑。
- 写 Agent 执行逻辑。

### `apps/desktop`

职责：未来 Electron 桌面产品。

可以做：

- 启动或连接 `awaken-core`。
- 管 Electron main / preload / renderer。
- 提供 `window.awaken` adapter。
- 消费 SSE 或本地 IPC 事件。
- 把 runtime 数据转成 `@awaken/ui` props。

不可以做：

- 从 `packages/ui/src` 做源码路径 import。
- 在 renderer 里执行后端工具。
- 把 AG-UI protocol mapping 放进 UI components。

### `packages/ui`

职责：可复用 React UI 组件库。

可以做：

- 渲染 primitives、inputs、navigation、layout、feedback、content components。
- 通过 props 接收数据。
- 通过 callbacks 把用户操作交给业务层。
- 提供 visual tokens 和 platform-aware UI affordances。

不可以做：

- 调 `fetch`。
- 打开 SSE/WebSocket。
- 使用 Electron IPC 或 `window.awaken`。
- import AG-UI 或 A2UI protocol package。
- 知道 Agent runtime 内部实现。

## 最小 AwakenEvent 集合

Phase 1 只需要最小事件集，能驱动 Run Timeline 即可：

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
```

后续有真实消费方后，再增加：

```text
ApprovalRequested
ApprovalResolved
ArtifactCreated
ArtifactUpdated
RunCancelled
StepStarted
StepFinished
WorkspaceChanged
```

不要为了“完整性”提前加事件。事件是 runtime contract，应该随真实需求扩展。

## AG-UI 映射规则

Phase 1 映射必须明确并有测试：

```text
Awaken RunStarted            -> AG-UI RUN_STARTED
Awaken RunFinished           -> AG-UI RUN_FINISHED
Awaken RunError              -> AG-UI RUN_ERROR
Awaken TextMessageStarted    -> AG-UI TEXT_MESSAGE_START
Awaken TextDelta             -> AG-UI TEXT_MESSAGE_CONTENT
Awaken TextMessageFinished   -> AG-UI TEXT_MESSAGE_END
Awaken ReasoningStarted      -> AG-UI REASONING_START + REASONING_MESSAGE_START
Awaken ReasoningDelta        -> AG-UI REASONING_MESSAGE_CONTENT
Awaken ReasoningFinished     -> AG-UI REASONING_MESSAGE_END + REASONING_END
Awaken ToolCallStarted       -> AG-UI TOOL_CALL_START
Awaken ToolCallArgs          -> AG-UI TOOL_CALL_ARGS
Awaken ToolCallFinished      -> AG-UI TOOL_CALL_END
Awaken ToolResult            -> AG-UI TOOL_CALL_RESULT
```

如果某个模型 provider 暂时不能实时 streaming tool args，Phase 1 可以在 provider response 完成后一次性发 `TOOL_CALL_ARGS`。实时 tool args delta 是后续优化，不是第一阶段门槛。

## A2UI 边界

A2UI 不进入 AG-UI Phase 1。

只有当 Agent 需要生成临时声明式 UI surface 时，才评估 A2UI，例如：

- 生成表单。
- 结构化审批面板。
- 选择器。
- artifact 预览 surface。
- tool call 参数编辑面板。

未来可能的链路是：

```text
Agent output / runtime event
  -> ArtifactCreated(kind: "a2ui_surface")
  -> AG-UI CUSTOM or state event carrying A2UI payload
  -> apps/desktop renderer validates payload
  -> trusted renderer maps payload to Awaken UI components
```

A2UI 不替代 Awaken 主界面。Sidebar、Composer、RunTimeline、ApprovalCard、ArtifactGrid 等主界面组件继续来自 `@awaken/ui`。

## 第三方引用记录

不要把 AG-UI 或 A2UI monorepo 复制进 Awaken。

如果需要记录参考来源，只加一个很小的文档：

```text
third_party/README.md
```

记录内容包括：

- 项目名。
- 本地参考路径。
- commit SHA。
- Awaken 用它参考什么。
- Awaken 明确不 import / 不 vendor 什么。

## Phase 计划

### Phase 1: 协议出口 spike（已完成）

状态：已完成。Awaken 已具备最小 AG-UI-compatible SSE 出口，当前 `/v1/agui/demo` 是 fake/demo stream，用于验证 wire format；真实 runtime session stream 留到 Phase 2。

目标：证明 Awaken 可以输出 AG-UI-compatible SSE events，不改变 UI 架构。

实现范围：

```text
src/agentcore/events.go
src/agentcore/event_sink.go
src/protocol/agui/events.go
src/protocol/agui/mapper.go
src/protocol/agui/encoder.go
src/server/sse.go
cmd/awaken-core/main.go
```

最小验收：

```bash
go test ./...
go build ./...
go vet ./...
curl --noproxy 127.0.0.1 http://127.0.0.1:<port>/health
curl --noproxy 127.0.0.1 http://127.0.0.1:<port>/v1/agui/demo
```

demo stream 必须包含：

```text
RUN_STARTED
TEXT_MESSAGE_CONTENT
TOOL_CALL_START
TOOL_CALL_RESULT
RUN_FINISHED
```

### Phase 2: 真实 runtime session stream

目标：用真实 Agent run 替换 fake/demo events。

实现范围：

```text
src/runtime/session.go
src/runtime/run_service.go
src/runtime/event_bus.go
src/server/handlers.go
```

建议 endpoints：

```text
GET  /health
POST /v1/runs
GET  /v1/runs/{run_id}/events
POST /v1/runs/{run_id}/cancel
```

endpoint 名字可以后续调整，但职责边界不要变。

### Phase 3: Desktop consumer

目标：让 `apps/desktop` 消费 runtime stream。

实现范围：

```text
apps/desktop/main/
apps/desktop/preload/
apps/desktop/renderer/
```

renderer 应把 AG-UI events 转成 view state，再把 props 传给 `@awaken/ui`。

### Phase 4: Approvals and artifacts

目标：让本地权限审批和 artifact 成为一等 runtime event。

只在 runtime 有真实生产和消费需求后增加：

```text
ApprovalRequested
ApprovalResolved
ArtifactCreated
ArtifactUpdated
```

### Phase 5: A2UI evaluation

目标：判断 A2UI 是否值得用于 generated surfaces。

从一个窄场景开始，例如 generated parameter editor 或 approval form。不要一开始就在主应用里做通用 A2UI renderer。
