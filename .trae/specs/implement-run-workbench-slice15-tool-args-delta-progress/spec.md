# Slice 15: Tool Args Delta 与 Tool Progress Spec

## Why

Run Narrative 现在能展示工具开始、完整参数、完成和结果，但用户仍会看到工具调用“突然出现”。Slice 15 让 provider stream 中的 tool args delta 和工具执行 progress 进入 Awaken event pipeline，Desktop 可以在工具准备和执行阶段持续展示状态。

## What Changes

- Agent Core 新增 `ToolCallArgsDelta` 与 `ToolProgress` 内部事件。
- LLM stream parser 在 OpenAI tool call delta 和 Anthropic `input_json_delta` 时发出 args delta。
- 保留现有完整 args fallback，不破坏最终 tool args。
- Tool runtime 执行时发出轻量 progress。
- AG-UI mapper 映射 args delta / progress。
- Desktop reducer/projector 接收 args delta / progress，更新 active tool/exploring action。
- 增加 provider parser / runner / mapper / reducer 测试。

## Impact

- 触及 `src/agentcore`、`src/protocol/agui`、`apps/desktop/src/shared`、Desktop reducer/projector。
- 不改变 tool 并发执行模型。
- 不改变审批策略或工具安全边界。

## Requirements

### Requirement: Provider args delta

#### Scenario: Anthropic streams `input_json_delta`

- **WHEN** Anthropic stream 返回 `input_json_delta`
- **THEN** Agent Core 发出 `ToolCallArgsDelta`，并最终仍发出完整 `ToolCallArgs`

#### Scenario: OpenAI streams tool call arguments

- **WHEN** OpenAI stream 返回 tool call `function.arguments` delta
- **THEN** Agent Core 发出 `ToolCallArgsDelta`，并最终仍发出完整 `ToolCallArgs`

### Requirement: Tool progress

#### Scenario: Tool runtime starts and finishes execution

- **WHEN** 工具进入执行阶段
- **THEN** Tool runtime 发出 `ToolProgress`，Desktop 可显示执行中状态

### Requirement: AG-UI mapping

#### Scenario: Runtime streams args delta and progress

- **WHEN** AG-UI mapper 收到内部 args delta/progress event
- **THEN** SSE event 保留 tool call id、tool name、delta/progress detail 与 seq

### Requirement: Desktop projection

#### Scenario: Desktop receives args delta/progress

- **WHEN** Desktop reducer/projector 收到 args delta/progress
- **THEN** active tool/exploring action 会更新 detail/status，不产生 orphan active item

### Requirement: Cancel fallback

#### Scenario: Run is cancelled during tool preparation

- **WHEN** ctx cancellation occurs before tool execution
- **THEN** 不留下 orphan active tool item，现有 synthetic tool result pairing 仍有效

## Modified Requirements

- Tool args 不再只在完整 JSON 形成后进入事件流；delta 也会进入事件流。

## Removed Requirements

- 无。
