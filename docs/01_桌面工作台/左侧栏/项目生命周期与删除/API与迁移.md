# 项目生命周期与删除：API 与迁移

## API

- `GET /v1/projects`
- `POST /v1/projects`
- `DELETE /v1/projects/{id}`
- `DELETE /v1/projects/{id}/threads`
- `POST /v1/projects/{id}/trash`
- `GET /v1/project-operations/{id}`
- `POST /v1/project-operations/{id}/cancel`

Electron Main 的废纸篓 worker 另使用 `claim-trash`、`attest-trash`、`stage-trash`、`begin-trash`、`complete-trash` 与 `fail-trash` 内部握手。每次 claim 生成新的 durable attempt token，后续回调必须持有当前 token。`stage-trash` 按卷与文件 ID 复验目录身份，再把原目录原子移动到同盘 operation 专属目录；同一目录内新增或修改文件不会误判，同路径被替换为新目录则拒绝删除且新目录永远不会进入 `shell.trashItem`。

修改请求使用 `Idempotency-Key`。异步动作返回 operation ID、phase、逻辑提交状态和待清理数量；内部错误只写带 operation ID 的结构化日志，Renderer 只消费稳定领域错误码。

## 存储

共享 `sql.DB` 管理 `projects`、`desktop_state`、`threads`、`project_operations`、`project_operation_threads` 和 `cleanup_jobs`，开启 WAL、foreign keys、busy timeout，并使用单 writer/`BEGIN IMMEDIATE` 完成状态、operation 和 outbox 原子提交。

## 迁移

一次性导入旧 Local Storage 项目状态、`workspace-registry.json`、`last-workspace.json` 和 distinct thread workspace。显式 removed 状态优先；只有历史 thread 证据的 workspace 按 detached 导入。回填稳定 project ID 后，Renderer 不再用 thread workspace 反向创建项目，避免已移除项目被历史会话重新激活。
