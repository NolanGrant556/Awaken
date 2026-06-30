# Awaken Work 场景功能规划：对照 Claude Code 源码

本文用于两人协作开发 Awaken。GitHub `main` 是主分支。当前分工：

- Nolan：Agent 架构、runtime 产品路径、Desktop 前端。
- 伙伴：MCP、连接器、Skill、外部工具能力。

核心取舍：参考 Claude Code 的“工作执行内核”，不要搬它的 Coding 产品外壳。Awaken v1 应聚焦 work 场景：让用户把一个真实工作任务拆解、执行、审批、复核，并且过程可见、可控、可复用。

## 结论

Claude Code 最值得借鉴的不是代码编辑能力，而是这些抽象：

1. 角色定义：一个 Agent 是“何时使用、提示词、工具范围、权限、记忆、是否后台运行”的组合。
2. 上下文组装：系统提示、角色提示、项目上下文、用户上下文、记忆、Skill 以固定顺序进入模型。
3. 执行循环：模型输出、工具调用、权限判断、结果回写、压缩恢复，形成稳定 loop。
4. 任务生命周期：长任务可以前台、后台、取消、恢复、通知、回看。
5. 外部工具平面：MCP、连接器、Skill、插件最后都收敛为统一工具池，而不是各自独立接入。
6. 前端编排：输入、消息队列、审批、任务、通知、快捷键、子任务视图共享同一套状态和优先级。

Awaken 不应优先搬：

- Git / PR / commit / branch / worktree / LSP / diagnostics。
- 文件 patch、IDE diff、代码搜索、代码验证 Agent。
- Claude Code 终端 REPL 的 pager、alt-screen、scrollback 等终端专用交互。
- 把 `context_management`、prompt cache、beta header 等模型协议细节暴露成产品概念。

## Claude Code 参考源码地图

### Agent 与运行主链

- `/Users/nolangrant/claudecode源代码/src/main.tsx`
  - 启动时加载 commands、agents、tools、MCP clients，并把它们注入主会话。
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/loadAgentsDir.ts`
  - `AgentDefinition`、frontmatter/json agent schema、built-in / plugin / user / project agent 合并与覆盖。
  - 重点字段：`agentType`、`whenToUse`、`tools`、`disallowedTools`、`skills`、`mcpServers`、`hooks`、`model`、`effort`、`permissionMode`、`maxTurns`、`background`、`memory`、`isolation`。
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/AgentTool.tsx`
  - Agent 选择、权限校验、同步/后台/worktree/remote/teammate 分支。
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/runAgent.ts`
  - 构造 agent prompt、用户消息、系统上下文、工具池、MCP、Skill 后调用 `query()`。
- `/Users/nolangrant/claudecode源代码/src/query.ts`
  - 执行主循环：tool loop、streaming fallback、压缩、stop hooks、token limit 恢复。
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts`
  - 把 query 生命周期抽成可复用会话引擎，保存 messages、usage、permission denials、read state 等。
- `/Users/nolangrant/claudecode源代码/src/services/api/claude.ts`
  - 最终 LLM request builder：消息归一化、tool schema、context management、stream、fallback、usage。
- `/Users/nolangrant/claudecode源代码/src/services/tools/toolOrchestration.ts`
  - tool 调度、并发/串行处理、结果回写。
- `/Users/nolangrant/claudecode源代码/src/services/tools/toolExecution.ts`
  - 单次工具执行、权限、hook、错误处理。

### 前端交互与状态

- `/Users/nolangrant/claudecode源代码/src/screens/REPL.tsx`
  - 主工作台编排器，处理消息、审批、任务、通知、快捷键、背景会话。
- `/Users/nolangrant/claudecode源代码/src/state/AppStateStore.ts`
  - 统一状态模型，包含 tasks、notifications、elicitation、footerSelection、viewingAgentTaskId、foregroundedTaskId 等。
- `/Users/nolangrant/claudecode源代码/src/state/AppState.tsx`
  - 自定义 store provider，`useSyncExternalStore` 分片订阅，避免大面积重渲染。
- `/Users/nolangrant/claudecode源代码/src/utils/messageQueueManager.ts`
  - 统一消息/命令/任务通知队列，带 `now / next / later` 优先级。
- `/Users/nolangrant/claudecode源代码/src/components/PromptInput/PromptInput.tsx`
  - 输入框、footer pill、队列回填、任务/bridge 弹层入口。
- `/Users/nolangrant/claudecode源代码/src/components/PromptInput/PromptInputQueuedCommands.tsx`
  - 输入区下方展示排队消息。
- `/Users/nolangrant/claudecode源代码/src/components/permissions/PermissionRequest.tsx`
  - 审批分发器，按工具类型显示不同审批 UI。
- `/Users/nolangrant/claudecode源代码/src/components/TaskListV2.tsx`
  - 任务清单展示。
- `/Users/nolangrant/claudecode源代码/src/components/tasks/BackgroundTasksDialog.tsx`
  - 后台任务总览、详情、终止、切前台。
- `/Users/nolangrant/claudecode源代码/src/tasks/LocalAgentTask/LocalAgentTask.tsx`
  - 本地 agent task 状态、进度、完成通知。
- `/Users/nolangrant/claudecode源代码/src/hooks/useBackgroundTaskNavigation.ts`
  - 任务/子任务视图键盘导航。
- `/Users/nolangrant/claudecode源代码/src/keybindings/*`
  - 上下文化 keymap、用户覆盖、冲突校验、chord 支持。
- `/Users/nolangrant/claudecode源代码/src/context/notifications.tsx`
  - 站内通知队列、key 折叠、抢占优先级。

### MCP、连接器、Skill、外部工具

- `/Users/nolangrant/claudecode源代码/src/services/mcp/types.ts`
  - MCP transport、配置 schema、连接状态：`connected / failed / needs-auth / pending / disabled`。
- `/Users/nolangrant/claudecode源代码/src/services/mcp/client.ts`
  - MCP 连接、重连、tools/prompts/resources 映射、tool progress、401/session expired 恢复。
- `/Users/nolangrant/claudecode源代码/src/services/mcp/config.ts`
  - 配置合并、策略过滤、plugin server 去重、connector 去重、scope 优先级。
- `/Users/nolangrant/claudecode源代码/src/services/mcp/auth.ts`
  - OAuth/XAA、discovery、loopback callback、token refresh/revoke、安全存储键位。
- `/Users/nolangrant/claudecode源代码/src/services/mcp/useManageMCPConnections.ts`
  - 连接生命周期、reconnect、list_changed 刷新、elicitation handler。
- `/Users/nolangrant/claudecode源代码/src/services/mcp/claudeai.ts`
  - 一方连接器来源层，把 connector 拉成 MCP config。
- `/Users/nolangrant/claudecode源代码/src/services/mcpServerApproval.tsx`
  - 项目 MCP server 审批。
- `/Users/nolangrant/claudecode源代码/src/skills/loadSkillsDir.ts`
  - `SKILL.md` 装载，frontmatter 解析，`allowed-tools`、`hooks`、`context: fork`、参数替换。
- `/Users/nolangrant/claudecode源代码/src/tools/SkillTool/SkillTool.ts`
  - Skill 执行器，把本地 Skill、MCP Skill、远程 Skill 统一成工具调用。
- `/Users/nolangrant/claudecode源代码/src/skills/mcpSkillBuilders.ts`
  - MCP skill builder。
- `/Users/nolangrant/claudecode源代码/src/utils/plugins/schemas.ts`
  - 插件 manifest：hooks、commands、agents、skills、outputStyles、channels、mcpServers、userConfig。
- `/Users/nolangrant/claudecode源代码/src/utils/plugins/pluginOptionsStorage.ts`
  - 插件普通配置与敏感配置分流存储。

## Nolan 负责：Agent 架构与前端

### 1. Agent 注册表与固定工作角色

优先级：P0

为什么做：

Awaken 需要先定义“谁来做事”。不要一开始开放任意自定义 Agent，否则 UI、权限、Skill、任务状态都会变复杂。v1 先固定 4 个 work 角色：

- Researcher：搜集信息、读取连接器资源、整理事实。
- Planner：把目标拆成步骤、识别依赖、提出方案。
- Operator：执行被批准的动作，例如发消息、改表格、创建任务。
- Reviewer：检查结果、发现遗漏、生成复核报告。

具体内容：

- 新增 `AgentDefinition` 领域模型。
- 每个 Agent 定义包含：
  - `id`
  - `displayName`
  - `whenToUse`
  - `systemPrompt`
  - `allowedTools`
  - `allowedSkills`
  - `requiredConnectors`
  - `memoryScopes`
  - `defaultRunMode`: `foreground | background`
  - `maxTurns`
- 内置固定 4 角色，不开放外部编辑。
- Desktop 里以“角色卡片 / 任务模板”呈现，不暴露 coding 术语。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/loadAgentsDir.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/builtInAgents.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/prompt.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/AgentTool.tsx`

Awaken 落点：

- Go：`src/agentcore` 或新增 `src/agentdefs`
- Runtime：`src/runtime/run_service.go`
- Server：`src/server/router.go`、`src/server/handlers.go`
- Desktop：`apps/desktop/src/shared/runtime.ts`、`apps/desktop/src/renderer/App.tsx`

验收标准：

- UI 能选择 4 个固定角色启动任务。
- 不同角色展示不同说明和可用能力。
- Operator 不能访问未授权工具。
- Reviewer 可以读取任务结果，但默认不执行外部写入动作。

不搬：

- Claude Code 的 `Explore / Plan / Verification` 代码仓库文案。
- Agent 的 worktree、commit hash、PR 相关规则。

### 2. 分层上下文组装器

优先级：P0

为什么做：

工作任务失败常常不是模型能力不够，而是上下文混乱。Awaken 需要固定上下文装配顺序，保证每次任务行为可预测。

具体内容：

上下文顺序建议固定为：

1. 基础系统提示：Awaken 的行为准则。
2. Work 场景边界：不是 coding agent，优先澄清、计划、审批、复核。
3. Agent 角色提示：Researcher / Planner / Operator / Reviewer。
4. 用户当前任务。
5. 项目上下文：workspace、当前目标、相关文档。
6. 连接器上下文：已连接哪些外部系统、可用资源。
7. Skill 上下文：当前任务可用 SOP。
8. 记忆上下文：user / project / team。
9. 当前 run 状态：任务列表、审批状态、未完成 blocker。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/utils/systemPrompt.ts`
- `/Users/nolangrant/claudecode源代码/src/memdir/memdir.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/AgentTool/agentMemory.ts`
- `/Users/nolangrant/claudecode源代码/src/skills/loadSkillsDir.ts`
- `/Users/nolangrant/claudecode源代码/src/QueryEngine.ts`

Awaken 落点：

- `src/prompt`
- `src/agentcore/request_builder.go`
- `src/agentcore/runner.go`

验收标准：

- 测试能断言上下文组装顺序。
- 同一 Agent 在不同会话中能稳定继承正确 scope 的记忆。
- 缺 connector 时，上下文明确提示能力不可用，而不是让模型猜。

不搬：

- `CLAUDE.md` 层级规则。
- `omitClaudeMd` 这类 repo 指令优化。
- 把 token/cache/beta header 暴露给用户。

### 3. Work Run Engine 与任务生命周期

优先级：P0

为什么做：

work 场景的任务往往很长，需要暂停、后台、恢复、通知。不能只靠聊天流和 spinner。

具体内容：

- 定义 `WorkRun`：
  - `runId`
  - `goal`
  - `agentRole`
  - `status`: `queued | running | waiting_approval | blocked | completed | failed | canceled`
  - `tasks`
  - `currentStep`
  - `events`
  - `startedAt`
  - `updatedAt`
  - `summary`
- 支持前台任务转后台。
- 支持取消任务但保留已产生事件。
- 完成后生成摘要和可回看结果。
- 后台任务完成后发送 Desktop 通知或站内通知。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/tasks/LocalAgentTask/LocalAgentTask.tsx`
- `/Users/nolangrant/claudecode源代码/src/tasks/RemoteAgentTask/RemoteAgentTask.tsx`
- `/Users/nolangrant/claudecode源代码/src/components/tasks/BackgroundTasksDialog.tsx`
- `/Users/nolangrant/claudecode源代码/src/hooks/useSessionBackgrounding.ts`

Awaken 落点：

- `src/runtime/run_service.go`
- `src/runtime/session.go`
- `src/runtime/event_bus.go`
- `src/server/handlers.go`
- `apps/desktop/src/shared/runtime.ts`
- `apps/desktop/src/renderer/runtime/reducer.ts`

验收标准：

- 一个运行中任务能转后台。
- 后台任务完成后，UI 有通知和摘要。
- 取消任务不会导致 transcript 丢失。
- `GET /v1/runs/{run_id}` 能返回任务状态。

不搬：

- Remote CCR / worktree / git isolation 作为 v1 必需能力。

### 4. 计划、澄清、审批闭环

优先级：P0

为什么做：

work 任务最大的风险是做错方向。Awaken 应该先澄清、再计划、再审批、再执行。

具体内容：

- Agent 可以发出结构化澄清：
  - 单选
  - 多选
  - 短文本
  - 需要附件/连接器授权
- Planner 生成方案后进入审批状态。
- 用户可以：
  - 批准执行
  - 拒绝
  - 修改计划
  - 要求补充信息
- Operator 只能执行已批准计划中的写动作。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/tools/AskUserQuestionTool/prompt.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts`
- `/Users/nolangrant/claudecode源代码/src/components/permissions/ExitPlanModePermissionRequest/ExitPlanModePermissionRequest.tsx`
- `/Users/nolangrant/claudecode源代码/src/components/permissions/PermissionRequest.tsx`

Awaken 落点：

- Go：`src/agentcore/events.go`、`src/runtime/run_service.go`
- Protocol：`src/protocol/agui/events.go`、`src/protocol/agui/mapper.go`
- Desktop：`apps/desktop/src/renderer/App.tsx`、`apps/desktop/src/renderer/runtime/reducer.ts`

验收标准：

- Agent 能提出澄清问题，Desktop 可回答。
- 计划审批前不会执行外部写入工具。
- 审批事件可回放。
- 子任务等待审批时，UI 不丢主上下文。

不搬：

- Bash/File/Edit/patch 专用审批 UI。
- 代码 diff 作为默认审批表达。

### 5. Workbench 前端统一编排层

优先级：P0

为什么做：

Claude Code 的前端强点不是终端样式，而是统一状态编排。Awaken 如果把任务、审批、通知、输入分别写成孤岛，会反复出现焦点、打断、状态不同步问题。

具体内容：

主界面保留三栏，但语义调整为 work-first：

- 左侧：任务板、角色、连接器状态。
- 中间：当前 run 叙事流、关键事件、输入框。
- 右侧：Inspector，包括审批、子任务、外部工具调用、输出摘要。

需要统一的 renderer state：

- `activeRunId`
- `inputDraft`
- `isSubmitting`
- `focusedPanel`
- `queuedMessages`
- `approvalQueue`
- `backgroundRuns`
- `notifications`
- `selectedTaskId`
- `selectedWorkerId`

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/screens/REPL.tsx`
- `/Users/nolangrant/claudecode源代码/src/state/AppStateStore.ts`
- `/Users/nolangrant/claudecode源代码/src/state/AppState.tsx`
- `/Users/nolangrant/claudecode源代码/src/utils/messageQueueManager.ts`
- `/Users/nolangrant/claudecode源代码/src/components/PromptInput/PromptInput.tsx`

Awaken 落点：

- `apps/desktop/src/renderer/App.tsx`
- `apps/desktop/src/renderer/runtime/reducer.ts`
- `apps/desktop/src/renderer/workbenchPresentation.ts`
- `apps/desktop/src/renderer/styles.css`

验收标准：

- 输入中有审批事件到来时，不抢掉用户草稿。
- 排队消息能回填到输入框。
- 审批、任务、通知共享同一优先级策略。
- UI 可切换查看后台 run 和当前 run。

不搬：

- 终端 pager、alt-screen、scrollback dump、REPL 搜索。

### 6. 任务清单、后台任务与子任务视图

优先级：P0

为什么做：

work 任务不能靠模型“记得”。需要显式 task model，让用户看到 owner、blocker、进度和结果。

具体内容：

- Task 字段：
  - `id`
  - `title`
  - `status`: `todo | doing | blocked | done | canceled`
  - `owner`: `user | researcher | planner | operator | reviewer | connector`
  - `blockedBy`
  - `sourceRunId`
  - `summary`
  - `lastActivityAt`
- UI 中展示：
  - 当前任务列表。
  - 后台任务列表。
  - 子 Agent / worker 的过程视图。
  - 任务完成后延迟隐藏但可回看。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/components/TaskListV2.tsx`
- `/Users/nolangrant/claudecode源代码/src/hooks/useTasksV2.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/TodoWriteTool/TodoWriteTool.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/TaskCreateTool/TaskCreateTool.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/TaskUpdateTool/TaskUpdateTool.ts`
- `/Users/nolangrant/claudecode源代码/src/hooks/useBackgroundTaskNavigation.ts`

Awaken 落点：

- Go：新增 `src/agenttask` 或在 `src/runtime` 内先做窄模型。
- Desktop：新增任务面板组件，先可以在 `App.tsx` 内完成，再拆组件。

验收标准：

- Agent 能创建/更新任务。
- 用户能看到 blocked 原因。
- 子任务完成后回主任务汇总。
- 后台任务可停止、切前台、查看最近 activity。

不搬：

- `git worktree` worker 视图。
- “必须提交 commit 才算完成”的 coding 规则。

### 7. 通知中心与快捷键系统

优先级：P1

为什么做：

work 场景要减少打扰但不能漏掉关键动作。通知和快捷键都不能硬编码散落在组件里。

具体内容：

- 通知分层：
  - immediate：审批、失败、需要用户补充。
  - normal：任务完成、连接器已授权。
  - low：同步完成、信息提示。
- 相同 key 的通知可以折叠。
- 快捷键按上下文解析：
  - composer
  - task panel
  - approval panel
  - inspector
  - global

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/context/notifications.tsx`
- `/Users/nolangrant/claudecode源代码/src/services/notifier.ts`
- `/Users/nolangrant/claudecode源代码/src/keybindings/KeybindingProviderSetup.tsx`
- `/Users/nolangrant/claudecode源代码/src/keybindings/defaultBindings.ts`
- `/Users/nolangrant/claudecode源代码/src/keybindings/useKeybinding.ts`

Awaken 落点：

- `apps/desktop/src/renderer`
- `apps/desktop/src/main/index.ts` 用于系统通知桥接。

验收标准：

- 审批通知会抢占普通通知。
- 重复连接器同步通知会合并。
- 同一个按键在不同面板下行为不同且不冲突。

不搬：

- Claude Code 的终端 select/list navigation 原样交互。

## 伙伴负责：MCP、连接器、Skill、外部工具

### 1. 统一 MCP runtime

优先级：P0

为什么做：

外部工作发生在文档、表格、IM、CRM、数据库、知识库里。Awaken 不应该为每种外部系统单独写一套工具接入，应该统一成 MCP runtime。

具体内容：

- 支持 transport：
  - `stdio`
  - `http`
  - `sse`
  - `ws`
  - `in-process/sdk-like`
- 连接状态：
  - `connected`
  - `failed`
  - `needs-auth`
  - `pending`
  - `disabled`
- 支持超时、重连、401 重新授权、session expired 恢复。
- 把 MCP tools/prompts/resources 转换为 Awaken 工具池。
- 未授权 server 不隐藏，显示为可触发授权的 placeholder。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/services/mcp/types.ts`
- `/Users/nolangrant/claudecode源代码/src/services/mcp/client.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/McpAuthTool/McpAuthTool.ts`

Awaken 落点：

- 新增 `src/mcpclient`
- 新增 server API：`/v1/mcp/servers`、`/v1/mcp/servers/{name}/auth`、`/v1/mcp/tools`
- 统一注册到 `src/tool/registry.go`

验收标准：

- 同一工作流中能同时连接一个本地 stdio MCP 和一个远端 HTTP/SSE MCP。
- 远端 401 后进入 `needs-auth`，UI 可提示授权。
- MCP tool 的 readOnly/destructive/openWorld 标注能进入权限判断。

不搬：

- IDE 专用 `sse-ide` / `ws-ide` 作为 v1 重点。
- Claude.ai proxy 具体产品绑定。

### 2. Connector 来源层

优先级：P0

为什么做：

Connector 不应该是另一套工具系统。它应该是“受信来源生成 MCP config”的一层。

具体内容：

- Connector 定义：
  - `id`
  - `displayName`
  - `category`: `docs | sheets | chat | crm | calendar | storage | database | custom`
  - `provider`
  - `authType`
  - `mcpConfigFactory`
  - `requiredScopes`
  - `trustLevel`
- Connector 最终产出 MCP server config。
- 支持手工 MCP 与 connector MCP 去重。
- 支持 official / team / local connector 来源。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/services/mcp/claudeai.ts`
- `/Users/nolangrant/claudecode源代码/src/services/mcp/config.ts`
- `/Users/nolangrant/claudecode源代码/src/services/mcp/officialRegistry.ts`

Awaken 落点：

- 新增 `src/connectors`
- Desktop 左侧或设置页新增 Connector Center。

验收标准：

- 用户能看到 connector 列表。
- 未授权 connector 显示原因。
- 同一 Slack/Notion MCP 被手工和 connector 同时配置时，只保留一个有效连接。

不搬：

- Claude.ai 组织 connector 的具体字段作为硬依赖。

### 3. OAuth、凭据与信任边界

优先级：P0

为什么做：

work 工具会访问真实业务系统。没有统一凭据和审批，用户不会信任产品。

具体内容：

- OAuth discovery。
- loopback callback。
- 手工粘贴 callback URL fallback。
- token refresh。
- token revoke。
- 按 server key 隔离存储。
- 敏感配置进系统安全存储，普通配置进 settings。
- 项目级 MCP、headers helper、hook、脚本执行需要 trust。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/services/mcp/auth.ts`
- `/Users/nolangrant/claudecode源代码/src/utils/plugins/pluginOptionsStorage.ts`
- `/Users/nolangrant/claudecode源代码/src/utils/secureStorage/index.ts`
- `/Users/nolangrant/claudecode源代码/src/services/mcp/headersHelper.ts`

Awaken 落点：

- 新增 `src/oauthstate`
- 新增 `src/credentialstore`
- 新增 `src/truststate`
- Electron main 负责系统级安全存储桥接。

验收标准：

- 同名不同 URL 的 server 不串 token。
- 清除授权后本地和远端 token 都失效。
- 敏感配置不会落入 Git tracked 文件或明文设置。
- 未信任 workspace 不执行本地 helper。

不搬：

- XAA 复杂组织级认证可后置。

### 4. Skill 运行时与 `SKILL.md`

优先级：P0

为什么做：

Awaken 的差异化不是“能聊天”，而是“能按某类工作的方法做事”。Skill 是把方法沉淀下来的最小单元。

具体内容：

- 支持 `SKILL.md`：
  - `name`
  - `description`
  - `when_to_use`
  - `allowed-tools`
  - `arguments`
  - `model`
  - `effort`
  - `context: fork`
  - `agent`
  - `hooks`
- 支持本地 skills、项目 skills、team skills。
- 支持 inline 执行和 fork 执行。
- 支持参数替换。
- 支持 Skill 绑定特定 Agent。
- 首批内置 work skills：
  - 会议纪要整理与分发。
  - 客户跟进日报。
  - 表格巡检与异常汇总。
  - 文档转行动项。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/skills/loadSkillsDir.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/SkillTool/SkillTool.ts`
- `/Users/nolangrant/claudecode源代码/src/skills/mcpSkillBuilders.ts`
- `/Users/nolangrant/claudecode源代码/src/skills/bundled/*`

Awaken 落点：

- 新增 `src/skills`
- 新增 `src/skillruntime`
- Desktop 增加 Skill library。

验收标准：

- 新增一个 `SKILL.md` 后，Awaken 能发现并展示。
- Skill 可限制只能使用指定 connector/tool。
- 长流程 Skill 可 fork 到后台执行，完成后回主线程总结。

不搬：

- Claude Code 的 coding skills，如 verify code、commit、debug repo。

### 5. MCP Skill 与外部系统 SOP

优先级：P1

为什么做：

外部系统不只提供工具，也会提供流程。比如 CRM 可以提供“客户跟进 SOP”，表格系统可以提供“异常巡检 SOP”。

具体内容：

- MCP server 可以暴露 prompts/resources，Awaken 将它们注册为可用 Skill 或 command。
- Skill 可以声明需要某个 MCP server 才可用。
- 缺 server 时 Skill 不消失，而是显示“需要连接 X”。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/services/mcp/client.ts`
- `/Users/nolangrant/claudecode源代码/src/skills/mcpSkillBuilders.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/SkillTool/SkillTool.ts`

Awaken 落点：

- `src/mcpclient`
- `src/skills`
- Desktop Skill library。

验收标准：

- 一个 MCP server 同时提供 tool 和 SOP prompt。
- Skill 可见性随 connector 状态变化。
- MCP prompt 生成的 Skill 进入同一权限和观测链。

不搬：

- 动态 tool search 的复杂优化可以后置。

### 6. 工具权限与观测

优先级：P0

为什么做：

所有外部动作都要能解释、能审批、能追踪。权限不要散落在每个 connector 里。

具体内容：

- 统一工具元数据：
  - `source`: `builtin | mcp | connector | skill | plugin`
  - `readOnly`
  - `destructive`
  - `openWorld`
  - `requiresAuth`
  - `serverName`
  - `toolName`
- 统一权限结果：
  - `allow`
  - `ask`
  - `deny`
- 统一工具进度：
  - started
  - progress
  - completed
  - failed
  - needs_auth
- 记录审批来源：本地 UI、远程 channel、系统策略。

Claude Code 参考：

- `/Users/nolangrant/claudecode源代码/src/hooks/useCanUseTool.tsx`
- `/Users/nolangrant/claudecode源代码/src/components/permissions/PermissionRequest.tsx`
- `/Users/nolangrant/claudecode源代码/src/services/internalLogging.ts`
- `/Users/nolangrant/claudecode源代码/src/services/toolUseSummary/toolUseSummaryGenerator.ts`
- `/Users/nolangrant/claudecode源代码/src/services/mcp/client.ts`

Awaken 落点：

- `src/tool/permission.go`
- `src/agentcore/tool_runtime.go`
- `src/runtime/event_bus.go`
- Desktop Approval Center。

验收标准：

- 能区分工具超时、OAuth 过期、server down、session expired。
- 高风险工具调用必须出现审批。
- 完成后能生成用户可读摘要，例如“已同步客户跟进表，并向项目群发送提醒”。

不搬：

- Bash command classifier 和文件 diff 作为主要审批模型。

## 共同边界：两人必须先定的契约

### 1. `AgentDefinition`

Nolan 主导，伙伴消费。

必须先稳定字段：

- `id`
- `displayName`
- `whenToUse`
- `systemPrompt`
- `allowedTools`
- `allowedSkills`
- `requiredConnectors`
- `memoryScopes`
- `defaultRunMode`
- `maxTurns`

### 2. `ToolDefinition`

伙伴主导，Nolan 消费。

必须先稳定字段：

- `name`
- `displayName`
- `source`
- `inputSchema`
- `description`
- `annotations.readOnly`
- `annotations.destructive`
- `annotations.openWorld`
- `requiresAuth`
- `permissionPolicy`
- `progressEvents`

### 3. `ApprovalRequest`

共同设计。

必须表达：

- 谁请求审批。
- 请求执行什么动作。
- 会影响哪些外部系统。
- 是否写入/删除/通知他人。
- 用户可选动作：批准、拒绝、修改计划、补充信息。
- 审批结果如何回到 run loop。

### 4. `TaskState`

Nolan 主导。

必须表达：

- task id、title、owner、status。
- blockedBy。
- source run。
- last activity。
- summary。
- result refs。

### 5. `ConnectorState`

伙伴主导。

必须表达：

- connected / failed / needs-auth / pending / disabled。
- capabilities。
- auth status。
- last error。
- last sync。
- trusted source。

### 6. `SkillDefinition`

伙伴主导，Nolan 消费。

必须表达：

- name、description、whenToUse。
- allowed tools。
- arguments。
- required connectors。
- run mode。
- bound agent。
- hooks。

## 三个里程碑

### M1：Work Core

目标：

做出最小可用的 work agent 桌面工作台。

Nolan 做：

- 固定 4 个 Agent：Researcher / Planner / Operator / Reviewer。
- 分层上下文组装。
- WorkRun lifecycle。
- 任务板。
- 计划、澄清、审批闭环。
- 后台任务基础 UI。

伙伴做：

- 1 到 2 个核心连接器。
- 最小 MCP runtime。
- 最小 Skill runtime。
- 基础 OAuth/凭据存储。
- 工具权限元数据。

验收：

用户输入一个真实工作任务，例如“整理本周客户跟进并生成下一步行动”，Awaken 能走完：

1. Researcher 搜集信息。
2. Planner 生成计划并请求审批。
3. Operator 执行被批准的外部动作。
4. Reviewer 检查结果并输出复核摘要。
5. 全程任务状态可见，可后台，可取消，可回看。

### M2：Connector Platform

目标：

让伙伴新增 connector / Skill 时，不需要改 Nolan 的前端架构。

Nolan 做：

- Connector Center UI。
- Skill Library UI。
- Approval Center 完整化。
- 通知中心。
- 快捷键系统。
- 多后台任务切换。

伙伴做：

- MCP config 层级合并与去重。
- OAuth refresh/revoke。
- 官方连接器目录。
- MCP prompts/resources 转 Skill。
- project/local memory。
- tool observability。

验收：

新增一个外部系统连接器时：

- 能授权。
- 能显示能力。
- 能进入工具池。
- 能被 Skill 调用。
- 能在审批和日志中看到清晰来源。

### M3：Team Mode

目标：

做多 worker 可视化与团队协作。

Nolan 做：

- Worker transcript 视图。
- 多 agent 任务拓扑。
- blocker/owner 可视化。
- 远程任务视图基础。

伙伴做：

- team memory。
- team-level connector policy。
- remote/channel approval。
- org-level trust policy。

验收：

一个负责人能同时跑 3 个 worker：

- Researcher 查资料。
- Operator 更新外部系统。
- Reviewer 做复核。
- 用户能看到每个 worker 的状态、blocker、审批点和最终汇总。

## 当前不做清单

明确不进入 M1：

- Git / PR / commit / branch 工作流。
- LSP diagnostics。
- IDE RPC。
- worktree 隔离。
- code search / grep / edit / patch 作为主功能。
- 代码 diff 审批 UI。
- Claude Code 的 terminal REPL pager。
- remote ultraplan。
- XAA 组织级复杂认证。
- 自定义 Agent 编辑器。
- Skill marketplace。

可以后置评估：

- 隔离执行空间：适合“试一版但不污染主工作区”的文档/表格草稿，不使用 git worktree 语义。
- 动态 Skill 搜索：M2 后再看。
- 远程执行：等本地 worker + 审批 + connector/skill 跑顺再做。

## 建议的首批 work 场景

优先选高频、低歧义、可验收的场景：

1. 文档：会议纪要、周报、方案草稿、行动项提取。
2. 表格：数据巡检、异常汇总、进度表更新。
3. IM/协作：把结果发送到群、提醒负责人、收集审批回复。
4. 知识库：查 SOP、查项目资料、生成简明引用摘要。

先不要选：

- 需要复杂权限和大量写入的 CRM 全流程。
- 需要系统级 computer-use 的桌面自动化。
- 需要长期远程任务编排的组织级 workflow。

## 下一步拆工建议

第一周：

- Nolan：定义 `AgentDefinition`、`WorkRun`、`TaskState`、`ApprovalRequest` 草案。
- 伙伴：定义 `ToolDefinition`、`ConnectorState`、`SkillDefinition` 草案。
- 两人一起评审契约，确保前端和 MCP runtime 不互相猜字段。

第二周：

- Nolan：做固定 4 角色 + 最小任务板 + 计划审批 UI。
- 伙伴：做最小 MCP runtime + 一个读型 connector + 一个 `SKILL.md`。

第三周：

- Nolan：接入后台任务、通知、审批回放。
- 伙伴：接入 OAuth、写型工具审批、tool progress。

第四周：

- 用一个真实 work 流程端到端验收，并删除任何 coding-only 残留入口。
