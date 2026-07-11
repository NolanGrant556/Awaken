# 项目生命周期与删除：API与持久化

## 1. 权威范围

本文唯一拥有项目生命周期各 API 动作的业务语义、领域错误、SQLite 表、事务/outbox 和 Electron Trash worker 协作。当前 route 注册清单及条件由 [09 协议域](../../../09_AGUI与A2UI协议/HTTP与SSE接口契约.md) 唯一拥有；用户语义见 [产品与状态机](产品与状态机.md)。

## 2. 代码 owner

- `src/runtime/project_store.go`：schema、迁移、scan和稳定project/operation数据。
- `src/runtime/project_service.go`：状态机、事务、idempotency、lease、staging、恢复和cleanup。
- `src/server/project_handlers.go`、`router.go`：HTTP和领域错误映射。
- `apps/desktop/src/main/project-trash-worker.ts`：系统trash调用与attempt握手。
- `apps/desktop/src/renderer/projectWorkspace.ts`：operation/project reconciliation纯判断。
- `apps/desktop/src/renderer/App.tsx`：dialog、tracker、refresh和用户notice。

## 3. 公共 API 动作语义

具体 method、path 与注册条件只查 09 域 route 清单。项目域当前提供以下动作：列出项目（默认排除 removed）、upsert workspace project、detach 且保留历史和目录、逻辑删除项目历史、prepare trash-and-purge、列出未终态 trash operation、查询 durable operation，以及只在合法等待阶段取消 operation。

修改类公共请求必须带 `Idempotency-Key`。同key/同project/action返回同operation；同key被其他operation占用返回冲突。

## 4. Worker 内部动作

| 动作 | 责任 |
|---|---|
| `claim-trash` | worker领取lease并生成attempt id；确认run已停 |
| `attest-trash` | 在staging前复验原路径身份 |
| `stage-trash` | 原子rename到同卷operation目录并持久化路径 |
| `begin-trash` | 系统调用前durable记录invoked boundary |
| `complete-trash` | 确认staging消失后提交tombstone/history/cleanup |
| `fail-trash` | 恢复staging和project state，保存稳定错误 |

除 claim 外的后续请求必须带当前 `attempt_id`。lease 恢复也必须重新 claim；旧 attempt 调用返回 `PROJECT_TRASH_ATTEMPT_INVALID`。这些动作对应的 route path 只由 09 域清单定义。

## 5. 领域错误

| code | HTTP | 语义 |
|---|---:|---|
| `PROJECT_NOT_FOUND` | 404 | project不存在 |
| `PROJECT_OPERATION_NOT_FOUND` | 404 | operation不存在 |
| `PROJECT_OPERATION_CONFLICT` | 409 | 状态/阶段不允许 |
| `PROJECT_RUNS_BUSY` | 409 | active run尚未停止 |
| `PROJECT_PATH_FINGERPRINT_CHANGED` | 409 | 路径身份被替换 |
| `PROJECT_TRASH_ATTEMPT_INVALID` | 409 | attempt已过期/不匹配 |
| `IDEMPOTENCY_KEY_CONFLICT` | 409 | key属于其他operation |
| `IDEMPOTENCY_KEY_REQUIRED` | 400 | 修改请求缺key |
| `PROJECT_PATH_INVALID` | 400 | workspace path无效 |

未知内部错误返回 `INTERNAL_ERROR`，详细错误只写带operation id的结构化日志；renderer显示稳定notice。

## 6. SQLite与事务

共享 `sql.DB` 开启WAL、foreign keys、busy timeout，关键mutation使用单writer/`BEGIN IMMEDIATE`。

主要表：

- `projects`：workspace identity、state、reason、version；
- `desktop_state`：迁移/桌面状态；
- `threads`：durable thread与project关系；
- `project_operations`：action/status/phase/fingerprint/attempt/staging/invocation；
- `project_operation_threads`：prepare时的thread cutoff集合；
- `cleanup_jobs`：thread purge和trash worker outbox。

逻辑状态、operation和outbox必须同事务提交。禁止先删JSONL再写SQLite；否则crash会出现UI历史与物理文件不一致。

## 7. Cleanup与业务完成

`detach` 可同步完成。`delete_history` 和trash finalize会soft-delete thread并写cleanup jobs；pending cleanup >0时HTTP可返回202。

业务完成（logical committed）与物理完成（JSONL/附件清理）分离：

- UI以逻辑提交和project durable state为准；
- cleanup job幂等重试；
- 已逻辑删除的thread不能因physical失败复活；
- operation 在 pending cleanup 清零后进入 `phase=completed, status=succeeded`。

## 8. Renderer tracker

pending trash tracker至少保存：operationId、UI projectId、backendProjectId、label和原expanded状态，并写入localStorage。启动/重载后继续查询operation和project：

- pending/unknown：继续低频跟踪；
- committed且project removed：清除tracker，保持移除；
- `status=failed` 且 `phase=failed|cancelled`、project active：清除 tracker 并恢复项目/expanded；
- operation请求失败或project refresh失败：不清tracker。

`projectWorkspace.ts` 的reconciliation纯函数是判断authority；App不应散落第二套phase集合。

## 9. 迁移与兼容

一次性导入旧Local Storage project状态、`workspace-registry.json`、`last-workspace.json`和distinct thread workspace：

- 显式removed优先；
- 只有历史thread证据的workspace按detached导入；
- 回填稳定project id；
- renderer不再用thread workspace反向创建/激活tombstone project；
- 重新添加同路径通过upsert显式恢复。

## 10. 安全不变量

1. 系统trash只接收staging路径。
2. staging必须同卷原子rename。
3. begin-trash先于shell trash调用durable提交。
4. complete前必须确认staging不存在。
5. original path若已被新目录占用，不覆盖、不删。
6. cutoff之后新建thread不被旧operation误删。
7. renderer不能直接调用文件删除命令。

## 11. 测试

- `src/runtime/project_service_test.go`：三动作、状态、idempotency、cutoff、lease、fingerprint、crash recovery、cleanup。
- `src/runtime/project_service_test.go` 中的store-backed场景：schema/migration/FK与operation持久化。
- `src/server/project_handlers_test.go`：routes、HTTP错误、attempt链。
- `apps/desktop/src/main/project-trash-worker.test.ts`：claim、path replacement、invocation boundary、attempt透传、vanished staging。
- `apps/desktop/src/renderer/App.test.ts`、`projectWorkspace` tests：tracker和reconcile。
