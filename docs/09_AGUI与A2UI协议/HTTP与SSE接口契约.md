# 本地 HTTP 与 SSE 接口契约

## 1. 权威范围

本文唯一拥有当前 core route 清单、注册条件、SSE framing、cursor 和断线续传。各 route 的业务规则仍由对应功能域拥有。

## 2. 地址与信任边界

Desktop 正式路径由 `apps/desktop/src/main/runtime-supervisor.ts` 动态分配 `127.0.0.1` 端口并把 base URL 交给 app。独立 `awaken-core` 的 `8787` fallback 和 Vite proxy fallback 只用于各自开发路径，不能写成 Desktop 固定地址。

本地 loopback 不等于已建立跨用户认证边界。新增高风险 route 必须同时评估 Electron 代理、请求来源、文件/凭据权限和审计，不得因“只在本机”绕过功能域安全合同。

## 3. 当前 route 清单

`src/server/router.go` 是注册 authority。带条件的 route 只有相应 service/manager/registry 注入后才可用。

### 3.1 基础、运行与线程

```text
GET    /health
GET    /v1/agui/demo
GET    /v1/runtime/info
GET    /v1/model-providers
GET    /v1/threads
PATCH  /v1/threads/{thread_id}
DELETE /v1/threads/{thread_id}
GET    /v1/threads/{thread_id}/transcript
POST   /v1/threads/{thread_id}/context-snapshot
POST   /v1/runs
GET    /v1/runs/{run_id}
GET    /v1/runs/{run_id}/events
POST   /v1/runs/{run_id}/cancel
POST   /v1/runs/{run_id}/user-questions/{question_id}/answer
GET    /v1/runs/{run_id}/audit
POST   /v1/shutdown
```

`/v1/shutdown` 只有注入 shutdown callback 才注册。`/audit` 当前返回空数组，查询链尚未接通；其审计语义由 04 域记录。`POST /v1/threads/{thread_id}/context-snapshot` 接收与 run 相同的 `model_profile`，只重建并耐久化该 thread 的当前下一请求 projection；不发 provider 请求，响应为 204。Desktop 在 GET transcript 前调用它；没有可运行 profile 时直接读取历史 transcript。

### 3.2 项目与可恢复删除

```text
GET    /v1/projects
POST   /v1/projects
PUT    /v1/projects/order
DELETE /v1/projects/{project_id}
DELETE /v1/projects/{project_id}/threads
POST   /v1/projects/{project_id}/trash
GET    /v1/project-operations
GET    /v1/project-operations/{operation_id}
POST   /v1/project-operations/{operation_id}/cancel
POST   /v1/project-operations/{operation_id}/claim-trash
POST   /v1/project-operations/{operation_id}/attest-trash
POST   /v1/project-operations/{operation_id}/stage-trash
POST   /v1/project-operations/{operation_id}/begin-trash
POST   /v1/project-operations/{operation_id}/complete-trash
POST   /v1/project-operations/{operation_id}/fail-trash
```

只有 ProjectService 存在时注册。

### 3.3 Tool、MCP、Connector 和 Skill

```text
GET    /v1/tools
GET    /v1/mcp/tools
GET    /v1/mcp/servers
POST   /v1/mcp/servers
GET    /v1/mcp/servers/{name}/config
PATCH  /v1/mcp/servers/{name}/config
POST   /v1/mcp/servers/{name}/auth
POST   /v1/mcp/servers/{name}/disable
POST   /v1/mcp/servers/{name}/enable
DELETE /v1/mcp/servers/{name}
GET    /v1/connectors
POST   /v1/connectors/{id}/connect
POST   /v1/connectors/{id}/disconnect
GET    /v1/skills
POST   /v1/skills/install
```

`/v1/mcp/servers*` 依赖 MCP manager；connector routes 依赖 registry。Skill list/install 始终注册，registry 缺失时返回 503。`POST /v1/skills/{id}/execute` 未注册，只是保留 handler。

### 3.4 Memory 与 Dream

```text
GET    /v1/memories
GET    /v1/memories/{memory_id}
DELETE /v1/memories/{memory_id}
GET    /v1/memory/settings
PATCH  /v1/memory/settings
POST   /v1/memory/dream
GET    /v1/memory/dream
GET    /v1/memory/dream/{task_id}
POST   /v1/memory/dream/{task_id}/cancel
```

Memory list/get/delete 与 Dream routes 始终注册；project scope 只有在 ProjectService 可解析 project/workspace identity 时可用。settings routes 只有 `MemorySettingsStore` 成功建立时注册。正文、revision、scope、Dream task 与恢复语义由 11 域拥有；Renderer 只通过 Main/preload typed bridge 调用，不把任意绝对路径当作授权。

## 4. SSE wire

`GET /v1/runs/{run_id}/events` 返回 `text/event-stream`。每个 frame：

```text
id: <stream cursor>
data: <one JSON Event>

```

encoder 优先用 `streamCursor` 生成 `id:`，为零时回退到 `seq`。每个 data 行是独立事件，consumer 不应把多个 reasoning/tool frames 当作一个 JSON 对象。

## 5. Cursor 与重放

当前查询参数：

- `after_cursor=<n>`：正式续传参数；
- `after_seq=<n>`：兼容旧 Desktop 的同义 transport watermark。

两者当前都按“最后已确认的 frame cursor”解释，而不是 SemanticRecord 序号。server 先以 `cursor / 16` 做 coarse EventBus 过滤，再在展开帧后精确丢弃 `cursor <= afterCursor` 的帧。

帧序号公式：

```text
frame cursor = runtime/semantic event sequence * 16 + frame sub-index
```

16 是当前 `agui.FrameSeqStride`。一条记录可产生多帧；重连在其中一帧之后时必须保留同一记录尚未确认的后续帧。`SSE id`、`streamCursor` 和 active `seq` 保持兼容一致；耐久 frame 同时携带 `semanticSeq`。

## 6. Live 与 transcript 一致性

订阅先输出历史，再接 active channel。run 执行期间“transcript snapshot + after_cursor SSE”必须做到每个文本、工具、terminal frame 恰好一次。live 与重放都通过同一 semantic projector；任何只修 live mapper、不修 journal projector 的变化都不完整。

用户 prompt 从 run metadata 重建，不由 SemanticKindUserMessage 产生重复 AG-UI frame。terminal、artifact、tool pairing 也必须按相同 record identity 去重。

## 7. 错误与恢复

- 无效 cursor 按 0 处理；consumer 应保存最近成功处理的 `id`。
- run 不存在返回 404；建立 stream 后客户端断开由 request context 终止订阅。
- runtime 重启后的耐久恢复通过 transcript/projector，不保证恢复进程内 active channel。
- 未知事件仍可通过 `AWAKEN_UNKNOWN_EVENT` 传输；不得中止整个 stream。

## 8. 验证依据

- `src/server/router.go`
- `src/server/handlers.go`
- `src/server/sse.go`
- `src/runtime/event_bus.go`
- `src/protocol/agui/encoder.go`
- `src/protocol/agui/sequence.go`
- `src/server/server_test.go`
- `src/protocol/agui/encoder_test.go`
