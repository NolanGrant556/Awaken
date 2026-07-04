# Tasks

- [x] Task 1：Runtime 事件 seq 与 afterSeq 过滤
  - [x] SubTask 1.1：在 `src/runtime/session.go` 的 `Event` 结构体新增 `Seq uint64` 字段
  - [x] SubTask 1.2：在 `src/runtime/event_bus.go` 的 `Append` 方法中分配单调递增 seq 并写入 Event
  - [x] SubTask 1.3：在 `src/runtime/event_bus.go` 的 `Subscribe` 方法支持 `afterSeq` 参数，过滤 History 中 seq <= afterSeq 的事件
  - [x] SubTask 1.4：在 `src/runtime/run_service.go` 暴露公开 `GetRun(runID)` 方法，返回 `Session.Snapshot()`
  - [x] SubTask 1.5：在 `src/runtime/run_service.go` 的 `Subscribe` 支持 `afterSeq` 参数透传
  - [x] SubTask 1.6：更新 runtime 测试验证 seq 分配与 afterSeq 过滤

- [x] Task 2：API 端点与协议扩展
  - [x] SubTask 2.1：在 `src/server/router.go` 新增 `GET /v1/runs/{run_id}` 路由
  - [x] SubTask 2.2：在 `src/server/handlers.go` 新增 `handleGetRun` handler，返回 run JSON，404 处理
  - [x] SubTask 2.3：在 `src/server/handlers.go` 的 `handleRunEvents` 解析 `after_seq` query 参数，传给 `Subscribe(afterSeq)`
  - [x] SubTask 2.4：在 `src/protocol/agui/events.go` 的 `Event` 结构体新增 `Seq uint64 json:"seq,omitempty"` 字段
  - [x] SubTask 2.5：在 `src/server/handlers.go` 的 `mapRuntimeEvent` 中映射 seq 字段
  - [x] SubTask 2.6：在 `src/protocol/agui/encoder.go` 的 `EncodeSSE` 输出 `id: <seq>\n` 行
  - [x] SubTask 2.7：更新 server 测试验证 GET /v1/runs/{run_id} 和 after_seq 过滤

- [x] Task 3：UI 类型与 client 扩展
  - [x] SubTask 3.1：在 `apps/desktop/src/shared/agui.ts` 的 `AguiEvent` 新增 `seq?: number` 字段
  - [x] SubTask 3.2：在 `apps/desktop/src/shared/runtime.ts` 的 `RuntimeClient` 新增 `getRun(runId)` 方法签名
  - [x] SubTask 3.3：在 `apps/desktop/src/renderer/runtime/client.ts` 实现 `getRun`（browser + electron）
  - [x] SubTask 3.4：在 `apps/desktop/src/main/index.ts` 新增 IPC handler `awaken:get-run`，调用 `GET /v1/runs/{run_id}`
  - [x] SubTask 3.5：在 `apps/desktop/src/preload/index.ts` 暴露 `getRun` bridge 方法

- [x] Task 4：UI 断线自动重连
  - [x] SubTask 4.1：在 `apps/desktop/src/renderer/runtime/client.ts` 的 `subscribeRun` 中跟踪 last seq
  - [x] SubTask 4.2：断线时（非 terminal）自动重连，带 `after_seq=lastSeq` 参数
  - [x] SubTask 4.3：重连期间 dispatch `run/reconnecting` action，UI 显示 "Reconnecting..."
  - [x] SubTask 4.4：terminal 事件后不重连

- [x] Task 5：UI 刷新后恢复
  - [x] SubTask 5.1：在 `apps/desktop/src/renderer/App.tsx` 持久化 `runId` 到 localStorage（run 创建时写入，terminal 时清除）
  - [x] SubTask 5.2：挂载时检查 localStorage 中的 `runId`，若存在则调 `getRun` + `subscribeRun(after_seq=0)` 恢复
  - [x] SubTask 5.3：在 reducer 新增 `run/reconnecting` action 处理
  - [x] SubTask 5.4：在 reducer 新增 `run/recovered` action，恢复前先 reset 状态再应用 run 快照
  - [x] SubTask 5.5：全量 replay（after_seq=0）前 reducer reset 到 initial state，确保文本累加不重复

- [x] Task 6：测试与验证
  - [x] SubTask 6.1：runtime 测试：seq 单调递增、afterSeq 过滤正确
  - [x] SubTask 6.2：server 测试：GET /v1/runs/{run_id} 返回快照、404；after_seq 过滤
  - [x] SubTask 6.3：确认 reducer replay 幂等性（reset 后全量 replay 不重复）
  - [x] SubTask 6.4：运行 `go test ./src/runtime/... ./src/server/... ./src/protocol/...` 通过
  - [x] SubTask 6.5：Desktop build 与 type check 通过

# Task Dependencies

- Task 1 与 Task 2 可并行（Task 2 依赖 Task 1 的 seq 字段，但可先定义协议再实现）
- Task 3 依赖 Task 2（API 端点存在后才能实现 client）
- Task 4 依赖 Task 3（client 扩展后才能实现重连）
- Task 5 依赖 Task 3、Task 4（需要 getRun + 重连能力）
- Task 6 依赖 Task 1、Task 2、Task 3、Task 4、Task 5
