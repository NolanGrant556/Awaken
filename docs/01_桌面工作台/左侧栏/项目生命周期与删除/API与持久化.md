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
- `apps/desktop/src/renderer/projectWorkspace.ts`：operation/project reconciliation 与多根 run scope 的纯判断。
- `src/desktopdata/paths.go`：`SessionsRoot` / `SessionWorkspacePath` / `SessionOutputPath`，零文件夹项目 session 工作目录路径的唯一构造点。
- `Awaken Design System` 的 `AwakenProjectFoldersEditor`：文件夹行、主文件夹徽标、行内切主/移出控件与额外操作菜单的唯一 UI 组件，创建项目与编辑项目两个弹窗复用。

## 3. 公共 API 动作

具体 method、path 与注册条件只查 09 域。项目域提供：列出项目（默认排除 removed）、创建或 upsert 项目、按稳定 ID 重命名、挂载文件夹、解绑文件夹、切换主文件夹、以完整active project id序列替换侧栏顺序、detach 且保留历史和目录、逻辑删除项目历史、prepare 文件夹级 trash-and-purge、列出未终态 trash operation、查询 operation，以及只在合法等待阶段取消 operation。

### 3.1 创建与文件夹组成

`POST /v1/projects` 有两种 body 形态，由字段决定：

- `{"workspace_root": "...", "display_name": "..."}`：单文件夹 **upsert**。重新打开一个已知目录时继续复用并重新激活它原来的项目，不会新建重复项目。
- `{"folders": [{"path": "...", "primary": true}], "display_name": "..."}`：多文件夹 **创建**，总是新项目，`project_id` 随机生成。`folders` 顺序即展示顺序；没有任何 `primary=true` 时第一个文件夹成为主文件夹。`folders` 为空数组（或两个字段都不给）也合法，项目就此以零文件夹存在，只靠 `display_name` 立项。

两种形态都返回 `201` 与完整 project。project 的 `folders` 是权威字段，`workspace_root` 只镜像主文件夹路径以兼容既有 wire consumer。

文件夹不被独占：同一个磁盘目录可以同时属于任意多个项目，创建和挂载都不因为它已被别的项目持有而失败。一个目录参与哪些工作由用户决定，存储层不做仲裁。

文件夹级动作都返回 `200` 与更新后的完整 project，都不创建 operation、不需要 `Idempotency-Key`：

| 动作 | body | 语义 |
|---|---|---|
| `POST /v1/projects/{id}/folders` | `{"path": "..."}` | 挂载一个文件夹作为次文件夹；项目此前没有文件夹时，它直接成为主文件夹。目录已属于其他项目不影响挂载 |
| `DELETE /v1/projects/{id}/folders` | `{"path": "..."}` | 解绑，不碰磁盘，也不影响仍持有该目录的其他项目；解绑主文件夹时由剩余文件夹接任，全部解绑后项目回到零文件夹状态 |
| `PUT /v1/projects/{id}/primary-folder` | `{"path": "..."}` | 在已挂载的文件夹之间切换主文件夹 |

`POST /v1/projects/{id}/folders/trash` 的 body 是 `{"path": "..."}`，指明本次进废纸篓的那个用户文件夹。它创建 operation，因此仍然要求 `Idempotency-Key`。

### 3.2 重命名

`PATCH /v1/projects/{id}` 接受 `{"display_name":"..."}`，返回 `200` 与更新后的完整 project。Core 去掉名称首尾空白；结果为空返回 `400 PROJECT_NAME_INVALID`。项目不存在返回 `404 PROJECT_NOT_FOUND`，项目处于 `trash_pending` 返回 `409 PROJECT_OPERATION_CONFLICT`。

重命名按稳定 `project_id` 原子 set，不创建 operation，也不需要 `Idempotency-Key`。新名称与当前名称相同是 no-op，不推进 `version` / `updated_at`，也不追加空 mutation；真正变化只推进这三个名称相关事实，不改变 `folders`、`workspace_root`、`state`、`project_order`、thread metadata 或历史。共享同一个文件夹的其他项目不受影响。零文件夹项目使用完全相同的合同。

Desktop 后续创建或继续 run 时，把当前 project authority 的最新 `display_name` 作为 `workspace.project_name` 送入 TaskScope；该字段进入模型上下文，但不参与路径、权限或 identity 解析。名称变化会让后续 run 重建 scope 上下文，不回写既有 transcript。

### 3.3 幂等与排序

创建operation的修改请求必须带 `Idempotency-Key`。同 key/同 project/action 返回同 operation；同 key 被其他 operation 占用返回冲突。“删除会话”内部使用稳定 `thread-delete:<thread-id>`，重复请求不重复创建 job。项目排序使用幂等的完整替换 `PUT`，body为 `{"project_ids":[...]}`，不创建operation，也不修改project `version/updated_at`；请求必须包含当前每个active project且每个id恰好一次。

## 4. Worker 内部动作

| 动作 | 责任 |
|---|---|
| `claim-trash` | worker 领取 lease、生成 attempt id，并确认 run 已停 |
| `attest-trash` | staging 前复验目标文件夹的路径身份 |
| `stage-trash` | 原子 rename 到同卷 operation 目录并持久化路径 |
| `begin-trash` | 系统调用前耐久记录 invoked boundary |
| `complete-trash` | 确认 staging 消失后解绑该文件夹并恢复 project state；这是最后一个文件夹时项目回到零文件夹状态 |
| `fail-trash` | 恢复 staging 和 project state，保存稳定错误 |

operation 的目标是一个文件夹，不是整个项目：`complete-trash` 不再 tombstone project，也不再按 cutoff soft-delete 项目历史。

废纸篓真的删掉磁盘目录，所以它对共享文件夹是全局事件：`prepare-trash` 把该文件夹对**每个**持有它的项目围栏起来并停掉锚在它上面的线程，`complete-trash` 把它从每个持有它的项目里一并解绑。失败或取消时这些围栏全部释放。非破坏性的项目级动作（detach 项目、删除历史）只围栏没有其他活跃持有者的文件夹，不牵连别的项目。

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
| `PROJECT_NAME_INVALID` | 400 | 重命名去掉首尾空白后为空 |
| `PROJECT_FOLDER_NOT_FOUND` | 404 | 目标文件夹不在这个项目里 |

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

`state.json` 是可重建快照，不是第二 authority；启动从 journal seq 重放到最新。

`RenameProject` mutation携带重命名后的完整 project post-state。它不携带 `project_order`、thread side-file mutation 或 operation/job，因此重启只重放名称及该次 material change 的 `version/updated_at`；同名 no-op 不写 journal。

文件夹归属只有一处事实：每个 project 自己的 `folders`。存储层不再维护路径到 project 的反向索引；「这个目录属于哪些项目」由 `ProjectsHoldingFolder` 按侧栏顺序扫描 project 得出，可能返回多个。旧 `state.json` 里遗留的 `workspace_index` 字段不再被读取，解码时忽略，无需迁移。禁止先删除 session 目录再提交逻辑 mutation，否则 crash 会造成 UI 历史与物理文件不一致。operation/job ID、时间和 cutoff 必须先生成并写进 mutation，重放不得重新生成。

## 7. Cleanup 与业务完成

`detach` 可同步完成。`delete_history`、`delete_thread` 和 trash finalize 先在 lifecycle mutation 中 soft-delete thread 并创建 cleanup jobs；pending cleanup 大于 0 时 HTTP 可返回 202。

业务完成（logical committed）与物理完成（session metadata/journal/附件清理）分离：

- UI 以逻辑提交和 project durable state 为准；
- cleanup job 幂等 claim/retry；
- 只有已 soft-deleted session 才可物理清理；
- 已逻辑删除的 thread 不能因 physical failure 复活；
- pending cleanup 清零后 operation 进入 `phase=completed, status=succeeded`。

## 8. Renderer tracker

pending trash tracker 至少保存 operationId、UI projectId、backendProjectId、目标 folderPath、label 和原 expanded 状态，并写入 localStorage。缺少 folderPath 的旧条目在解析时被丢弃。启动/重载后继续查询 operation 与 project，判据是「项目是否还挂着这个文件夹」而不是「项目是否被整体移除」：

- pending/unknown：继续低频跟踪；
- committed 且项目已不含该文件夹：清除 tracker，保持移除；
- `status=failed` 且 `phase=failed|cancelled`、project active 且文件夹仍在：清除 tracker 并恢复展示/expanded；
- operation 请求失败或 project refresh 失败：不清 tracker。

`projectWorkspace.ts` 的 reconciliation 纯函数是判断 authority；App 不应散落第二套 phase 集合。

## 9. FileStore 唯一项目 authority

Renderer 启动无条件调用项目列表 API，并只把返回的 stable `project_id` 投影到侧栏。空 FileStore 必须返回 JSON `[]`，Electron Main 和 Renderer 不接受 `null` 作为集合，也不回退到旧 UI 状态。

当前产品不存在旧 Local Storage、workspace registry 或历史 thread workspace 的读取、清理、导入、转换及 upsert 入口。用户重新添加目录时，先通过公共 API取得 FileStore `project_id`，后续生命周期动作只使用该 ID；不得把路径编码的 Renderer ID 当成后端 ID。

同一个文件夹可以出现在多个项目的 `folders` 里，因此 `project_id` 不能从路径推导。新项目使用随机 id；已有 path-derived id 原样保留。加载旧 snapshot 或重放旧 journal 时，没有 `folders` 的项目合成 `[{path: workspace_root, role: "user", is_primary: true}]`，不需要迁移脚本。零文件夹项目的 `workspace_root` 为空字符串、`folders` 为空数组；它的 session 工作目录路径由 `desktopdata.SessionWorkspacePath` 唯一构造，不写入项目状态。

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
10. 项目的文件夹数可以为零；有文件夹时恰好一个是 primary。零文件夹项目的 run 由 `workspace.project_id` 解析到项目 session 工作目录。`workspace_root` 与 `project_id` 同时为空属于独立对话，不创建 project row，也不进入项目生命周期状态机；其 thread/session 契约由 02、03、04 域拥有。
11. 一个磁盘路径可以属于任意多个项目；归属只从各 project 的 `folders` 派生，不存在需要维护一致性的反向索引。判断「这个文件夹还能不能开 run」必须聚合它的全部持有者，而不是任取一个。
12. 项目名称是模型可见上下文而不是授权事实；重命名不得改变 `project_id`、路径 scope、文件夹归属、项目顺序或 thread ownership。

## 11. 验证

- `src/runtime/file_desktop_store_test.go`、`project_service_test.go`：状态、项目重命名的原子性/no-op/restart replay、项目排序完整性与restart replay、idempotency、cutoff、lease、fingerprint、crash recovery、cleanup。
- `src/runtime/project_folders_test.go`：多文件夹创建与 primary 指定、只凭名字创建零文件夹项目、跨项目 attach 冲突、primary 切换、解绑最后一个文件夹后项目仍存活且回到零文件夹、次文件夹 trash 后项目存活、旧 snapshot 合成单 primary、多根 TaskScope 保护项、session 工作目录解析与 sessions 子树例外。
- `src/server/project_handlers_test.go`：routes、重命名 trim/错误映射、HTTP 错误、attempt 链。
- `apps/desktop/src/main/project-trash-worker.test.ts`：claim、path replacement、invocation boundary、attempt、vanished staging。
- `apps/desktop/src/renderer/App.test.ts`、`projectWorkspace` tests：多根 run scope、folder tracker 与 reconcile。

真实 Desktop 必须在专用测试项目执行 detach、delete history、delete thread、文件夹级 trash success/failure/restart recovery；同时检查活动 `~/.awaken` generation 的 lifecycle journal、session 物理清理与原文件夹路径，不能用 Renderer cache 或 mock state 作为结果证据。
