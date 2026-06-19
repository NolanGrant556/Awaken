# Tasks

- [x] Task 1：Runtime Thread 存储与复用
  - [x] SubTask 1.1：在 `src/runtime/run_service.go` 新增 `Thread` 结构体（持有 `*agentcore.Agent`、`WorkspaceConfig`、`threadID`）和 `threads map[string]*Thread`
  - [x] SubTask 1.2：`StartRunRequest` 新增 `ThreadID string json:"thread_id,omitempty"` 字段
  - [x] SubTask 1.3：`StartRun` 逻辑：若 `ThreadID` 非空，查找 thread；不存在返回 `ErrThreadNotFound`；存在则复用 Agent/Conversation；workspace 不匹配返回 `ErrThreadWorkspaceMismatch`
  - [x] SubTask 1.4：`StartRun` 无 `ThreadID` 时创建新 Thread（新建 Agent + Conversation），存入 `threads` map
  - [x] SubTask 1.5：executor 工厂改为按 thread 复用 Agent（调用 `NewAgentWithConversation`）
  - [x] SubTask 1.6：新增 `ErrThreadNotFound`、`ErrThreadWorkspaceMismatch` 错误变量

- [x] Task 2：Agent Core 支持外部 thread_id
  - [x] SubTask 2.1：在 `src/agentcore/agent.go` 新增 `NewAgentWithConversationAndThreadID(config, conv, threadID)` 工厂方法
  - [x] SubTask 2.2：确保 `Agent.threadID` 可从外部传入而非总是新建
  - [x] SubTask 2.3：`cmd/awaken-core/main.go` 的 executor 工厂改为接受 thread 复用入口

- [x] Task 3：API 端点与错误处理
  - [x] SubTask 3.1：在 `src/server/handlers.go` 的 `handleCreateRun` 中处理 `ErrThreadNotFound` → 404 `THREAD_NOT_FOUND`
  - [x] SubTask 3.2：处理 `ErrThreadWorkspaceMismatch` → 400 `THREAD_WORKSPACE_MISMATCH`
  - [x] SubTask 3.3：更新 server 测试验证未知 thread_id 返回 404、workspace 不匹配返回 400

- [x] Task 4：UI 类型与 client 扩展
  - [x] SubTask 4.1：在 `apps/desktop/src/shared/runtime.ts` 的 `StartRunRequest` 和 `StartRunRequestWire` 新增 `threadId?` / `thread_id?` 字段
  - [x] SubTask 4.2：在 `apps/desktop/src/renderer/runtime/client.ts` 的 `startRun` 传递 threadId
  - [x] SubTask 4.3：在 reducer 新增 thread 列表状态（`threads: ThreadItem[]`）和相关 actions

- [x] Task 5：UI Conversation List 与 Thread 管理
  - [x] SubTask 5.1：在 `apps/desktop/src/renderer/App.tsx` 中 conversation list 展示真实 threads（从 reducer state 读取）
  - [x] SubTask 5.2："New Task" 按钮清除当前 threadId，开始新 thread
  - [x] SubTask 5.3：续聊时 `handleSubmit` 携带当前 `threadId`
  - [x] SubTask 5.4：run 创建后把 thread 信息加入 thread 列表
  - [x] SubTask 5.5：thread 切换时恢复对应 thread 的展示状态

- [x] Task 6：真实 LLM smoke 测试
  - [x] SubTask 6.1：第一轮 run：创建 a.txt，记录 thread_id
  - [x] SubTask 6.2：第二轮 run（同 thread_id）："追加 world 到刚才那个文件"
  - [x] SubTask 6.3：验证 agent 识别上一轮文件，a.txt 内容为 hello world
  - [x] SubTask 6.4：验证未知 thread_id 返回 THREAD_NOT_FOUND

- [x] Task 7：验证与验收
  - [x] SubTask 7.1：运行 `go test ./src/runtime/... ./src/agentcore/... ./src/server/...` 通过
  - [x] SubTask 7.2：Desktop build 与 type check 通过
  - [x] SubTask 7.3：代码审查确认 thread 复用链路完整

# Task Dependencies

- Task 1 与 Task 2 可并行
- Task 3 依赖 Task 1（需要错误变量）
- Task 4 依赖 Task 1（需要 StartRunRequest 字段）
- Task 5 依赖 Task 4（需要 UI 类型扩展）
- Task 6 依赖 Task 1、Task 2、Task 3
- Task 7 依赖 Task 1-6
