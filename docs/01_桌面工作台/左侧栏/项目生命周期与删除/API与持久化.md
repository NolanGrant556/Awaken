# 项目生命周期与删除：API 与持久化

## 1. 权威范围

本文唯一拥有项目生命周期 API 动作、领域错误、FileStore mutation/outbox 和 Electron Trash worker 协作。当前 route 注册清单及条件由 [09 协议域](../../../09_AGUI与A2UI协议/HTTP与SSE接口契约.md) 唯一拥有；用户语义见 [产品与状态机](产品与状态机.md)，文件布局与恢复原语见 [03 全局数据与会话存储](../../../03_运行时与本地服务/全局数据与会话存储.md)。

## 2. 代码 owner

- `src/runtime/project_store.go`：稳定 project/operation 类型、枚举与纯 helper。
- `src/runtime/repository.go`：项目生命周期 repository contract。
- `src/runtime/file_desktop_store.go`：lifecycle journal、snapshot、thread metadata 和 cleanup job 的唯一产品实现。
- `src/runtime/project_service.go`：状态机、idempotency、lease、staging、恢复和 run 协调。
- `src/server/project_handlers.go`、`router.go`：HTTP 与领域错误映射。
- `apps/desktop/src/main/project-trash-worker.ts`：系统 trash 调用与 attempt 握手。
- `apps/desktop/src/renderer/projectWorkspace.ts`：operation/project reconciliation 纯判断。

## 3. 公共 API 动作

具体 method、path 与注册条件只查 09 域。项目域提供：列出项目（默认排除 removed）、upsert workspace project、以完整active project id序列替换侧栏顺序、detach 且保留历史和目录、逻辑删除项目历史、prepare trash-and-purge、列出未终态 trash operation、查询 operation，以及只在合法等待阶段取消 operation。

创建operation的修改请求必须带 `Idempotency-Key`。同 key/同 project/action 返回同 operation；同 key 被其他 operation 占用返回冲突。“删除会话”内部使用稳定 `thread-delete:<thread-id>`，重复请求不重复创建 job。项目排序使用幂等的完整替换 `PUT`，body为 `{"project_ids":[...]}`，不创建operation，也不修改project `version/updated_at`；请求必须包含当前每个active project且每个id恰好一次。

## 4. Worker 内部动作

| 动作 | 责任 |
|---|---|
| `claim-trash` | worker 领取 lease、生成 attempt id，并确认 run 已停 |
| `attest-trash` | staging 前复验原路径身份 |
| `stage-trash` | 原子 rename 到同卷 operation 目录并持久化路径 |
| `begin-trash` | 系统调用前耐久记录 invoked boundary |
| `complete-trash` | 确认 staging 消失后提交 tombstone/history/cleanup |
| `fail-trash` | 恢复 staging 和 project state，保存稳定错误 |

除 claim 外的后续请求必须带当前 `attempt_id`。lease 恢复必须重新 claim；旧 attempt 返回 `PROJECT_TRASH_ATTEMPT_INVALID`。

## 5. 领域错误

| code | HTTP | 语义 |
|---|---:|---|
| `PROJECT_NOT_FOUND` | 404 | project 不存在 |
| `PROJECT_OPERATION_NOT_FOUND` | 404 | operation 不存在 |
| `PROJECT_OPERATION_CONFLICT` | 409 | 状态/阶段不允许 |
| `PROJECT_RUNS_BUSY` | 409 | active run 尚未停止 |
| `PROJECT_PATH_FINGERPRINT_CHANGED` | 409 | 路径身份被替换 |
| `PROJECT_TRASH_ATTEMPT_INVALID` | 409 | attempt 已过期/不匹配 |
| `IDEMPOTENCY_KEY_CONFLICT` | 409 | key 属于其他 operation |
| `IDEMPOTENCY_KEY_REQUIRED` | 400 | 修改请求缺 key |
| `PROJECT_PATH_INVALID` | 400 | workspace path 无效 |
| `PROJECT_ORDER_INVALID` | 400 | 排序缺少active project、包含重复/空id或非active project |

未知错误返回 `INTERNAL_ERROR`；详细错误只写带 operation id 的结构化日志，Renderer 显示稳定 notice。

## 6. FileStore transaction

项目、项目顺序、operation、idempotency index、operation-thread cutoff 和 cleanup jobs 的 authority 是活动 generation 下的 `lifecycle/mutations.jsonl`。每个领域 commit 形成一条包含完整 post-state 的 mutation；`ReorderProjects` mutation保存完整 `project_order`，新增项目排在现有顺序之前，removed项目可保留在内部顺序尾部但不进入active侧栏：

```text
获取 store.lock
  -> append + fsync lifecycle mutation
  -> 更新内存 state
  -> 幂等物化 thread metadata side files
  -> 原子替换 lifecycle/state.json
  -> 释放 lock
```

`state.json` 是可重建快照，不是第二 authority；启动从 journal seq 重放到最新。禁止先删除 session 目录再提交逻辑 mutation，否则 crash 会造成 UI 历史与物理文件不一致。operation/job ID、时间和 cutoff 必须先生成并写进 mutation，重放不得重新生成。

## 7. Cleanup 与业务完成

`detach` 可同步完成。`delete_history`、`delete_thread` 和 trash finalize 先在 lifecycle mutation 中 soft-delete thread 并创建 cleanup jobs；pending cleanup 大于 0 时 HTTP 可返回 202。

业务完成（logical committed）与物理完成（session metadata/journal/附件清理）分离：

- UI 以逻辑提交和 project durable state 为准；
- cleanup job 幂等 claim/retry；
- 只有已 soft-deleted session 才可物理清理；
- 已逻辑删除的 thread 不能因 physical failure 复活；
- pending cleanup 清零后 operation 进入 `phase=completed, status=succeeded`。

## 8. Renderer tracker

pending trash tracker 至少保存 operationId、UI projectId、backendProjectId、label 和原 expanded 状态，并写入 localStorage。启动/重载后继续查询 operation 与 project：

- pending/unknown：继续低频跟踪；
- committed 且 project removed：清除 tracker，保持移除；
- `status=failed` 且 `phase=failed|cancelled`、project active：清除 tracker 并恢复项目/expanded；
- operation 请求失败或 project refresh 失败：不清 tracker。

`projectWorkspace.ts` 的 reconciliation 纯函数是判断 authority；App 不应散落第二套 phase 集合。

## 9. FileStore 唯一项目 authority

Renderer 启动无条件调用项目列表 API，并只把返回的 stable `project_id` 投影到侧栏。空 FileStore 必须返回 JSON `[]`，Electron Main 和 Renderer 不接受 `null` 作为集合，也不回退到旧 UI 状态。

当前产品不存在旧 Local Storage、workspace registry 或历史 thread workspace 的读取、清理、导入、转换及 upsert 入口。用户重新添加目录时，先通过公共 upsert API取得 FileStore `project_id`，后续生命周期动作只使用该 ID；不得把路径编码的 Renderer ID 当成后端 ID。

## 10. 安全不变量

1. 系统 trash 只接收 staging 路径。
2. staging 必须同卷原子 rename。
3. begin-trash 先于系统 trash 调用耐久提交。
4. complete 前必须确认 staging 不存在。
5. original path 已被新目录占用时不覆盖、不删。
6. cutoff 后新建 thread 不被旧 operation 误删。
7. Renderer 不能直接调用文件删除命令。
8. FileStore lifecycle mutation journal-first；snapshot 损坏只能重建，不能反向覆盖 journal。
9. Renderer与localStorage不得成为项目顺序authority；拖拽只产生乐观UI，Core拒绝后必须按FileStore回滚。

## 11. 验证

- `src/runtime/file_desktop_store_test.go`、`project_service_test.go`：状态、项目排序完整性与restart replay、idempotency、cutoff、lease、fingerprint、crash recovery、cleanup。
- `src/server/project_handlers_test.go`：routes、HTTP 错误、attempt 链。
- `apps/desktop/src/main/project-trash-worker.test.ts`：claim、path replacement、invocation boundary、attempt、vanished staging。
- `apps/desktop/src/renderer/App.test.ts`、`projectWorkspace` tests：tracker 与 reconcile。

真实 Desktop 必须在专用测试项目执行 detach、delete history、delete thread、trash success/failure/restart recovery；同时检查活动 `~/.awaken` generation 的 lifecycle journal、session 物理清理与原 workspace 路径，不能用 Renderer cache 或 mock state 作为结果证据。
