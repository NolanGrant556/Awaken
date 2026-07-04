# Slice 10：同一 Thread 多轮对话 Spec

## Why

当前每个 run 都创建全新的 thread_id 和 Agent/Conversation，用户无法在同一 thread 内连续追问。本切片让 runtime 按 thread_id 复用 Agent 和 Conversation，使用户能在同一上下文中多轮对话，agent 保留之前的文件操作和对话历史。

## What Changes

- API：`StartRunRequest` 新增可选 `thread_id` 字段；未知 thread_id 返回 `THREAD_NOT_FOUND`。
- Runtime：`RunService` 维护内存级 `threadID → *Thread` 映射，每个 Thread 持有 Agent 和 Conversation；`StartRun` 根据 thread_id 复用或新建 Thread。
- Agent Core：支持从外部传入 thread_id；`NewAgentWithConversation` 已存在，runtime 调用它复用 Conversation。
- UI：Conversation list 展示已有 threads；新建 thread 入口；提交 run 时携带 thread_id（续聊）或不带（新建）。
- Storage：v1 内存级，不跨 runtime 重启。
- Safety：跨 workspace 的 thread 复用必须明确失败。

## Impact

- 受影响能力：多轮对话、上下文复用、thread 管理。
- 受影响代码：
  - `src/runtime/run_service.go`（Thread 存储 + StartRun 复用逻辑）
  - `src/runtime/session.go`（ThreadID 来源改为外部传入）
  - `src/server/handlers.go`（thread_id 解析 + THREAD_NOT_FOUND）
  - `src/agentcore/agent.go`（支持外部 thread_id）
  - `src/agentcore/runner.go`（thread_id 传递）
  - `cmd/awaken-core/main.go`（executor 按 thread 复用 Agent）
  - `apps/desktop/src/shared/runtime.ts`（StartRunRequest 加 thread_id）
  - `apps/desktop/src/renderer/runtime/client.ts`（startRun 传 thread_id）
  - `apps/desktop/src/renderer/runtime/reducer.ts`（thread 列表状态）
  - `apps/desktop/src/renderer/App.tsx`（conversation list + thread 切换）

## ADDED Requirements

### Requirement: API 支持 thread_id 输入

The system SHALL accept an optional `thread_id` in the start run request to continue an existing thread.

#### Scenario: Continue existing thread

- **WHEN** `POST /v1/runs` is called with a valid `thread_id`
- **THEN** the run reuses the thread's existing conversation context
- **AND** the run's `thread_id` matches the input

#### Scenario: Create new thread

- **WHEN** `POST /v1/runs` is called without `thread_id`
- **THEN** a new thread is created with a fresh conversation
- **AND** the run's `thread_id` is a new unique ID

#### Scenario: Unknown thread_id

- **WHEN** `POST /v1/runs` is called with a `thread_id` that does not exist
- **THEN** the API returns 404 with `THREAD_NOT_FOUND` error code
- **AND** no run is created

### Requirement: Runtime 按 thread 复用 Agent/Conversation

The system SHALL maintain an in-memory thread store that reuses Agent and Conversation across runs with the same thread_id.

#### Scenario: Reuse conversation context

- **WHEN** a second run is started with the same `thread_id` as a previous run
- **THEN** the Agent's Conversation contains all messages from previous runs
- **AND** the agent can reference files created or modified in previous runs

#### Scenario: Thread workspace binding

- **WHEN** a thread is created with workspace A
- **AND** a subsequent run with the same thread_id specifies workspace B
- **THEN** the API returns a structured error indicating workspace mismatch
- **AND** no run is created

### Requirement: Agent Core 支持外部 thread_id

The system SHALL allow the runtime to pass an existing thread_id and conversation to the Agent.

#### Scenario: Agent with existing conversation

- **WHEN** the runtime creates an Agent for an existing thread
- **THEN** the Agent uses the thread's existing Conversation
- **AND** the Agent's thread_id matches the thread's ID
- **AND** multi-turn tool_use/tool_result pairing is maintained

### Requirement: UI Conversation List 与 Thread 管理

The system SHALL provide a conversation list UI showing threads, with the ability to create new threads and switch between them.

#### Scenario: Conversation list shows threads

- **WHEN** the user has created one or more runs
- **THEN** the conversation list displays each thread with a label (first user input)
- **AND** the current thread is highlighted

#### Scenario: New thread

- **WHEN** the user clicks "New Task"
- **THEN** a new thread is started (no thread_id sent)
- **AND** the conversation list shows the new thread

#### Scenario: Continue thread

- **WHEN** the user submits a follow-up message in an existing thread
- **THEN** the start run request includes the current `thread_id`
- **AND** the agent continues with previous context

### Requirement: thread/context 归 runtime 管

The system SHALL manage thread context in the runtime, not in the renderer.

#### Scenario: Runtime owns context

- **WHEN** a run is started with a thread_id
- **THEN** the runtime retrieves the conversation from its thread store
- **AND** the renderer does not send message history in the request
- **AND** the renderer only sends the new user input

## MODIFIED Requirements

无

## REMOVED Requirements

无
