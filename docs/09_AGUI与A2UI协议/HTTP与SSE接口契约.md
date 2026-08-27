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
GET    /v1/runtime/activity
GET    /v1/model-providers
GET    /v1/hooks?project_id={project_id}
POST   /v1/hooks/trust
POST   /v1/hooks/revoke
GET    /v1/threads
PATCH  /v1/threads/{thread_id}
DELETE /v1/threads/{thread_id}
POST   /v1/threads/batch-delete
GET    /v1/threads/{thread_id}/transcript
POST   /v1/threads/{thread_id}/context-snapshot
POST   /v1/runs
GET    /v1/runs/{run_id}
GET    /v1/runs/{run_id}/events
POST   /v1/runs/{run_id}/cancel
GET    /v1/runs/{run_id}/subagents
GET    /v1/runs/{run_id}/subagents/{task_id}
POST   /v1/runs/{run_id}/subagents/{task_id}/messages
POST   /v1/runs/{run_id}/subagents/{task_id}/cancel
POST   /v1/runs/{run_id}/user-questions/{question_id}/answer
POST   /v1/runs/{run_id}/user-questions/{question_id}/cancel
GET    /v1/runs/{run_id}/audit
POST   /v1/shutdown
```

`POST /v1/runs` 的 `workspace` 对象即 TaskScope wire。`workspace_root` 是已绑定项目的主文件夹；只给 `project_id` 表示零文件夹项目；两者都为空表示独立对话。Core 在规范化 workspace 前确定 thread ID：新对话生成 ID，follow-up 使用请求的 `thread_id`，然后解析到 thread 专属 session 工作目录。业务数据根不可用或 managed owner ID 不安全时返回 `INVALID_INPUT`。可选的 `project_name` 只用于让模型说得出自己在哪个项目里，Core 不从它解析路径或权限。`session_workspace` 由服务端派生并回写，客户端传入值一律被覆盖。roots 的语义与形状名单由 [TaskScope 与路径安全](../04_工具权限与审批/TaskScope与路径安全.md) 唯一拥有。

`GET /v1/instructions/global` 返回 `GlobalAgentsDocument{exists,revision,digest,content,byte_count,updated_at,max_bytes}`。`PUT /v1/instructions/global` 接受 `{expected_revision,content}`，`DELETE` 接受 `{expected_revision}`；revision 不匹配返回 409 `INSTRUCTION_REVISION_CONFLICT`。三条 route 只在 RunService 存在时注册。Renderer 不可通过 `POST /v1/runs` 提交 AGENTS 正文或路径。业务优先级、截断与恢复见 [02 AGENTS 指令与运行快照](../02_主智能体闭环/AGENTS指令与运行快照.md)。

`POST /v1/threads/batch-delete` 接受 `{"thread_ids":[...]}`，最多 200 个非空 thread ID，按首次出现顺序去重。响应始终区分 `deleted_thread_ids` 与 `failures[{thread_id,error,message}]`；逐项复用单条 durable delete，当前 active thread 以 `THREAD_BUSY` 留在失败集合。该 route 不承诺跨 session 原子性，Desktop 只能移除 `deleted_thread_ids`，不得把失败项乐观隐藏。

`/v1/shutdown` 只有注入 shutdown callback 才注册。`GET /v1/runtime/activity` 无条件注册、`RunService` 缺席时返回 503，只读返回 `{active_run_count, cancelling_run_count, observed_at}`（RFC3339 UTC）：`active_run_count` 统计当前 in-flight run（含正在取消的 run；普通、定时与 ACP run 都经 `RunService.StartRun`，统一计入），`cancelling_run_count` 是其中已进入 cancelling 状态的子集。其用途是 Electron Main 在退出/维护类操作（如更新重启）前读取运行时活动快照；Renderer 不直接调用。`/audit` 当前返回空数组，查询链尚未接通；其审计语义由 04 域记录。`POST /v1/threads/{thread_id}/context-snapshot` 接收与 run 相同的 `model_profile`，只重建并耐久化该 thread 的当前下一请求 projection；不发 provider 请求，响应为 204。Desktop 在 GET transcript 前调用它；没有可运行 profile 时直接读取历史 transcript。

Hook routes 只有 `RunService` 注入时注册，业务合同由 [Hook 产品与架构契约](../04_工具权限与审批/Hook产品与架构契约.md)拥有。GET 的 Settings 检查 DTO 返回 snapshot identity、enabled count、来源 receipt/诊断，以及 Core 严格解析和归一化后的静态 Hook 详情；详情可以包含命令 argv、URL、matcher 或 prompt 供当前用户在信任前审查，但不包含 secret 引用值、运行输入、环境或输出，也不进入 run snapshot/Journal。trust 绑定 `projectId + canonical sourcePath + digest`，revoke 绑定 project/source，均只影响后续 run。run 内 `hook_invocation_started/completed` 分别投影为 `AWAKEN_HOOK_STARTED/COMPLETED`；wire 只含 invocation/Hook/event/source identity、status/action/reason、duration 与有界 preview，不含 command、配置路径、输入正文、secret env 或完整 stdout/stderr。

Subagent list/detail/cancel/message route 只操作请求 run 的当前 task registry；run/task 不匹配返回 404。message body 只接受 `{content, idempotency_key}`，先耐久再入队；终态 task 返回 409 `subagent_not_running`。完整状态、取消和恢复边界见 [02 原生 Subagent 产品与架构契约](../02_主智能体闭环/原生Subagent产品与架构契约.md)。

### 3.2 项目与可恢复删除

```text
GET    /v1/projects
POST   /v1/projects
PUT    /v1/projects/order
DELETE /v1/projects/{project_id}
DELETE /v1/projects/{project_id}/threads
POST   /v1/projects/{project_id}/folders
DELETE /v1/projects/{project_id}/folders
PUT    /v1/projects/{project_id}/primary-folder
POST   /v1/projects/{project_id}/folders/trash
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

只有 ProjectService 存在时注册。`POST /v1/projects` 同时承担单文件夹 upsert（body 带 `workspace_root`）与多文件夹创建（body 带 `folders`）；废纸篓是文件夹粒度，`POST /v1/projects/{project_id}/folders/trash` 的 body 必须指明目标 `path`。请求与响应字段、错误码由 [项目生命周期与删除：API 与持久化](../01_桌面工作台/左侧栏/项目生命周期与删除/API与持久化.md) 唯一拥有。

### 3.3 Tool、MCP、Connector、Skill 和 Plugin

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
POST   /v1/skills/preview
POST   /v1/skills/install
DELETE /v1/skills/{id}
POST   /v1/skills/{id}/enable
POST   /v1/skills/{id}/disable
GET    /v1/plugins
POST   /v1/plugins/install
GET    /v1/plugins/{name}
POST   /v1/plugins/{name}/commit
POST   /v1/plugins/{name}/enable
POST   /v1/plugins/{name}/disable
POST   /v1/plugins/{name}/upgrade
DELETE /v1/plugins/{name}
POST   /v1/plugins/{name}/servers/{short}/enable
POST   /v1/plugins/{name}/servers/{short}/disable
```

`/v1/mcp/servers*` 依赖 MCP manager；connector routes 依赖 registry。Skill list/install/preview/delete/enable/disable 始终注册，registry 缺失时返回 503。`GET /v1/skills` 可按主文件夹优先重复提交 `projectRoot` query；Core 把这些值视为同一个项目的有序文件夹组，在一次 discovery 中合并并返回每条 project Skill 的实际 `projectPath`。只提交一次保持原单文件夹语义。`POST /v1/skills/{id}/execute` 未注册，只是保留 handler。Skill 预览与删除的业务合同由 [08 技能](../08_技能/索引.md) 拥有。

`/v1/plugins*` 始终注册；`PluginStore` 缺失返回 503 `PLUGINS_UNAVAILABLE`，`plugins.json` 损坏返回 503 `PLUGIN_STATE_CORRUPT`。`POST /v1/plugins/{name}/commit` 只接受修改前记录的 `expectedDigest`，CAS 冲突返回 409 `PLUGIN_RECORDED_DIGEST_CHANGED`，非法或不稳定的在线树返回 400 `PLUGIN_INVALID`。业务合同由 [14 插件](../14_插件/索引.md) 拥有。Renderer 只通过 Main `requestRuntime` 调用，不直接读写插件目录或状态文件。

`GET /v1/composer-catalog/events` 是 Main 常驻消费的 SSE 失效流。每次连接先发送当前 `{revision, kind:"snapshot"}`；Skill Registry 或 Plugin Store 的成功 catalog mutation 使 revision 单调递增并发送 `kind=skills|plugins`。事件不携带 Skill/Plugin 数据、路径或配置，Renderer 收到后只能通过 typed `listSkills/listPlugins` 按当前 workspace 与 marker 重读权威目录；重新连接不回放历史 mutation，因为 snapshot revision 与每次打开直接重读共同保证当前状态。

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

### 3.5 Computer Use 设置与控制

```text
GET   /v1/computer/settings
PATCH /v1/computer/settings

GET  /v1/control/computer/status
GET  /v1/control/computer/sessions/{run_id}
POST /v1/control/computer/sessions/{run_id}/pause
POST /v1/control/computer/sessions/{run_id}/takeover
POST /v1/control/computer/sessions/{run_id}/return
POST /v1/control/computer/sessions/{run_id}/terminate
POST /v1/control/computer/runs/{run_id}/reviews/{review_id}/resolve
POST /v1/control/computer/reviews/{review_id}/resolve
```

设置 route 在 Computer settings store 可建立时注册；更新会同时写入 store 与 RunService 内存配置，只影响后续 run。`/v1/control/computer/*` 仅在 Runtime control identity 注入时注册，并统一经 bearer control token middleware；Renderer 只能通过 Main 的 typed bridge 调用。业务状态、review 与兼容 route 约束见 [10 计算机操作能力](../10_计算机操作能力/索引.md)。

### 3.6 Desktop 网络 route 控制

```text
GET /v1/control/network-route
PUT /v1/control/network-route
```

仅在 Runtime control identity 注入时注册，并经同一 bearer control token middleware；network route manager 缺席时返回 503。GET 返回 Core 当前无凭据 snapshot；PUT 接受 Desktop Main 的 version 1 generation snapshot，拒绝未知字段、代理凭据与超过 32 KiB 的请求。业务优先级、MCP/CLI 恢复和跨平台验收由[03 跨平台网络代理与恢复](../03_运行时与本地服务/跨平台网络代理与恢复.md)拥有。

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

订阅先输出历史，再接 active channel。run 执行期间“transcript snapshot + after_cursor SSE”必须做到每个文本、工具、terminal 和 `AWAKEN_INSTRUCTION_SNAPSHOT` frame 恰好一次。live 与重放都通过同一 semantic projector；任何只修 live mapper、不修 journal projector 的变化都不完整。唯一例外是 live-only `AWAKEN_PROVIDER_RECONNECT`：它不在 transcript snapshot 中，重连订阅后只能靠后续 live 帧恢复，不得从 journal 补造。

用户 prompt 从 run metadata 重建，不由 SemanticKindUserMessage 产生重复 AG-UI frame。terminal、artifact、tool pairing 也必须按相同 record identity 去重。

## 7. 错误与恢复

- 无效 cursor 按 0 处理；consumer 应保存最近成功处理的 `id`。
- run 不存在返回 404；建立 stream 后客户端断开由 request context 终止订阅。
- runtime 重启后的耐久恢复通过 transcript/projector，不保证恢复进程内 active channel。
- Desktop SSE 客户端在 run 未收到 `RUN_FINISHED`/`RUN_ERROR` 前持续重连，退避上限 10 秒；只有 `run_not_found`/404 才把订阅收成失败。这与 02 域的 provider 网络等待/流重试不是同一条链。
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
