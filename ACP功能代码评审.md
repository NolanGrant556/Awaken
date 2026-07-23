# ACP 功能代码评审

## 1. 评审结论

本次未提交改动已经搭出了较完整的 ACP Client 骨架：第三方 SDK 被限制在 `src/acp/protocol`，Manager、runner registry、委派工具、Core route、AG-UI typed event、Desktop 设置页和 Workbench projection 的分层方向基本符合方案。

初次评审的 2 个 P0、7 个 P1 已完成大部分针对性修改；二次复核仍发现 **1 个 P0、5 个 P1**，当前结论仍为 **request changes**。最主要的阻塞是 ACP permission event 被延迟到工具结束后才发布，导致工具等待用户选择、用户却看不到 gate 的闭环死锁。当前也没有真实 Desktop + 真实 runner 的验收证据。

本文件 §3–§8 保留初次评审及修复说明，**当前复核结论以 §10 为准**。Phase 5 的八项产品与安全决策已经在方案文档中收敛为目标契约，但实现仍未开始，不能宣称“Awaken 可被其他 ACP client 当作 Agent 调用”为当前支持。

## 2. 变更规模

- 评审范围：本地全部未提交 ACP 相关改动，并交叉检查 Workbench、runtime、工具权限、AG-UI 和方案文档。
- 文件：约 63 个（36 个 tracked 修改、27 个 untracked 文件）。
- 行数：约 `+5823 / -42`；untracked 文件按当前文本行数计入新增量，因此是近似值。
- 主要模块：`src/acp`、`src/tools/delegate_external_agent`、`src/tools/configure_acp_runner`、`src/agentcore`、`src/protocol/agui`、`src/server`、Desktop Main/preload/Renderer、相关文档与 mock agent。

## 3. P0：合并前必须修复

### P0-1 `cwd` 可逃逸 TaskScope，外部 Agent 能在任意本机目录启动

位置：`src/tools/delegate_external_agent/tool.go:218`、`src/tools/delegate_external_agent/tool.go:306`

`resolveCwd` 对非空输入原样返回，既没有把相对路径基于 workspace root 解析，也没有执行 `Clean/Abs/EvalSymlinks` 和 TaskScope containment 检查。该值随后同时作为子进程 `cmd.Dir` 和 ACP `session/new.cwd`。模型传入 `C:\`、`..\..` 或 workspace 外的绝对路径时，runner 会以当前登录用户权限在该目录工作。

这违反方案 §3、§8、§10.3 中“外部 Agent 不能绕过 TaskScope/protected roots”的硬约束。

建议修复：

1. 从 ToolRuntime 注入完整、规范化的 TaskScope，而不是只读取 `PermissionContext.WorkspaceRoot` 字符串；
2. 相对 `cwd` 必须 join 到 workspace root，绝对 `cwd` 只作为待校验候选；
3. 对 root 与候选路径执行 `filepath.Clean`、`Abs`、可解析部分的 `EvalSymlinks`，再用路径组件级 containment 判断；
4. workspace root 为空、候选越界、protected root 或无法安全解析时，在 spawn 前拒绝；
5. 增加 Windows 大小写、卷标、UNC、junction/symlink、`C:\work` 与 `C:\work2` 的测试。

### P0-2 runner env value 被 Core API 返回并明文送入 Renderer

位置：`src/server/handlers_acp.go:28`、`apps/desktop/src/renderer/settings/ACPSettingsPanel.tsx:614`

`GET /v1/acp/runners` 直接序列化 `Registry.All()`，其中包含完整 `Env map[string]string`；设置页再把 `KEY=VALUE` 放进 textarea。API token、credential 等 runner secret 因此进入 Electron Main/Renderer 和前端状态。

这直接违反方案 §7.2 的“日志、API、journal 不返回 env value”、§12 的 Renderer ownership 边界以及 Phase 4 的“设置页不泄漏 env”退出条件。

建议修复：

1. 为 API 单独定义 redacted DTO，最多返回 env key、`has_value` 或不可逆占位符；
2. PATCH 使用显式的 set/remove 语义，未提供 env 时保留旧 secret，禁止前端用占位符回写覆盖；
3. 优先把 secret value 迁入已有 credential/secret owner，配置文件只保存引用；若暂不迁移，至少确保文件 ACL、API、日志、错误和 journal 均不回显；
4. 补 Core route 与 Desktop 状态快照的 secret-redaction 测试。

## 4. P1：功能与契约阻塞

### P1-1 permission 的真实 live 链路无法可靠选择或取消

位置：`src/protocol/agui/mapper.go:650`、`apps/desktop/src/renderer/App.tsx:4200`、`src/server/handlers_acp.go:106`

存在三个叠加问题：

- Manager 产生的 options 是 `[]map[string]any`，但 mapper 只接受 `[]any`，所以未经 JSON round-trip 的 live event 会得到空 `acpPermissionOptions`，Workbench 不会出现精确 option 按钮；
- Desktop 把“取消”编码为空 `option_id`，HTTP handler 却固定以 `cancelled=false` 调用 Manager，SDK 最终收到的是“selected empty option”，不是 cancelled outcome；
- resolve handler 不校验 URL 中的 `run_id`，也不校验 option 是否属于原 request，任意字符串都能被送回 runner。

建议把 pending permission 保存为 `{runID, threadID, runnerID, sessionID, allowedOptionIDs}` 的 typed record；resolve 同时校验全部 identity 和 exact option。取消应使用显式 `cancelled: true`，不要用空字符串复用“选择”语义。增加一条从 Manager 原始 event 经 AG-UI mapper、runtime reducer 到 resolve route 的端到端测试，不能只给 reducer 手工构造理想事件。

### P1-2 runner 子进程没有继承 Desktop/Core 用户环境

位置：`src/acp/protocol/client.go:35`、`src/acp/protocol/client.go:211`

`buildEnv` 只输出配置中的 override，没有以 `os.Environ()` 为基线。runner 会丢失 `PATH`、`USERPROFILE/HOME`、代理、证书、语言环境和工具自己的认证环境；这与 `src/acp/process.go` 中正确的 merge 实现冲突，而 production protocol client 又没有使用该 Spawn helper。

建议只有 `src/acp` 的一个 process owner 负责 spawn：继承当前用户环境后按平台大小写规则覆盖显式 key，并对日志/诊断做 redaction。用 mock runner 断言基础环境仍存在、override 生效、env 不进入输出。

### P1-3 turn timeout、`max_runtime_seconds` 和 bounded process-tree kill 没有落地

位置：`src/acp/manager.go:156`、`src/tools/delegate_external_agent/tool.go:35`、`src/acp/protocol/client.go:177`

`TurnTimeoutSeconds` 只写入 `ExpiresAt`，Prompt 没有 `context.WithTimeout`；工具参数 `max_runtime_seconds` 从未使用。Cancel 只发 ACP notification，不等待 turn 结束或升级到 force close。Close 只 `Process.Kill()` 父进程，未调用已有的 process-group/tree 逻辑；事实上 `setupProcessGroup` 仅被未使用的 `src/acp/process.go` 调用。

因此方案 Phase 0/1/2 中已勾选的 startup/turn timeout、pending permission cancel、bounded kill 和 Windows process-tree kill 尚不成立，runner 及其子进程可能在取消/Core 退出后残留。

建议统一实现两阶段关闭：取消 pending permission并发回 cancelled → `session/cancel` → 有界等待 → `session/close` → kill process group/tree → 有界 `Wait`。turn deadline 取 runner 配置、工具参数和 run deadline 的最小值，并映射为唯一 `acp_turn_timeout` terminal。

### P1-4 同一 `(thread, runner)` 并发 start 会覆盖会话并泄漏进程

位置：`src/acp/manager.go:66`、`src/acp/manager.go:101`

Manager 在检查冲突后释放锁，再执行 probe/spawn/initialize/new session，最后无条件写入 map。两个并发 start 都可能通过首次检查，后写入者覆盖前一个 Conversation，前一个 client/process 失去 owner。

建议在 map 中先原子占位 `starting`，或使用 per-key singleflight/锁；所有失败路径删除占位并关闭 client。提交前增加并发 start 测试并断言 factory 只创建一个进程。

### P1-5 Conversation summary 存在数据竞争

位置：`src/acp/conversation.go:60`、`src/acp/conversation.go:72`

`Summary` 只持有 `stateMu`，却读取由 `turnMu` 保护的 `activeTurn` 和 `pendingPermission`。在 prompt/permission/status 并发时这是未同步读写；当前环境因 CGO 未启用无法运行 `go test -race`，普通测试不会发现该问题。

建议使用单一 mutex 管理 conversation state，或规定固定锁序并在 Summary 中同时持锁；返回 active turn/pending permission 的深拷贝。随后在可用 CGO 的 CI 上运行 race 测试。

### P1-6 只声明了 ACP semantic kinds，没有写入 SemanticJournal

位置：`src/agentcore/semantic_record.go:41`、`src/agentcore/tool_runtime.go:438`

新增了八个 `SemanticKindACP*` 常量，但代码中没有任何 producer/projector 使用它们。ToolRuntime 只是把 ACP event 直接发到 live sink；ACP event 也没有进入 `artifactBufferingSink.semanticFacts` 的 durable batch。重连/replay 因而不能得到方案 §11.1 描述的 requested/resolved 只读历史和一致 identity。

建议先定义哪些 ACP 事实耐久、哪些仅 live，再建立 AwakenEvent → SemanticRecord → replay AwakenEvent 的双向投影和 golden test。permission requested 在 replay 必须只读且不可重新生成 gate；Core shutdown 要为 open turn/tool/permission 写 canonical interrupted/closed terminal。

### P1-7 frame/output 限制和 tool summary 已修复

位置：`src/acp/protocol/client.go:148`、`src/acp/protocol/client.go:263`

- `promptState` 改用 `strings.Builder` 累积 assistant text，并在 `maxTurn`（`max_frame_bytes` 的 4 倍）处截断，设置 `truncated` 标志。
- turn 结束时根据 `toolBuffer` 中各 tool snapshot 的 `completed`/`failed` 状态计算 `TurnResult.ToolSummary`。
- SDK transport 的 frame decoder 仍使用其内部 10 MB 上限；本侧在输出层补上了整 turn 上限和 summary 计算，并补充了相关单元测试。

## 5. 建议项

### 建议-1 readiness probe 已增强

位置：`src/acp/manager.go:65`、`src/server/handlers_acp.go:76`

- `Manager.ProbeRunner` 使用注入的 `clientFactory` 启动临时 ACP 客户端，并在 `StartupTimeoutSeconds` 内执行 `Initialize`。
- 该 probe 能区分命令缺失（factory 返回 `ErrCodeRunnerNotReady`）、启动超时（`ErrCodeStartTimeout`）和协议不匹配（`ErrCodeProtocolVersionMismatch`）。
- `handleProbeRunner` 已改为调用 `manager.ProbeRunner`，不再只做 `exec.LookPath`。

### 建议-2 配置写入已串行化并支持跨平台原子替换

位置：`src/acp/registry.go:100`、`src/acp/config.go:117`、`src/acp/replace_*.go`

- `Registry` 新增 `fileMu`，`UpsertUser`/`DeleteUser` 整个 read-modify-write 过程串行化。
- `SaveConfigFile` 改为在同目录创建唯一临时文件、写入后 `Sync`/`Close`，再调用平台相关的 `replaceFile`。
- Windows 下使用 `windows.MoveFileEx` 并带 `MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH`；Unix 下使用 `os.Rename`。
- 已补充并发 upsert 不丢更新和 existing-file 替换的测试。

### 建议-3 已增加 HTTP bridge 集成测试

- 新增 `src/server/handlers_acp_test.go`，覆盖：
  - `GET /v1/acp/runners` 返回 redacted env（只含 key + `has_value`）。
  - `PATCH` 使用 `env_set`/`env_remove` 合并环境变量，未提供 key 保留旧值。
  - `POST /v1/runs/{run_id}/acp-permissions/{interaction_id}/resolve` 校验 URL 中的 `run_id` 与 option 合法性。

## 6. 做得好的部分

- ACP SDK 类型被限制在 `src/acp/protocol`，没有扩散到 Agent、server 或 Renderer，后续替换 SDK 的边界清楚。
- Manager 以 `(threadID, runnerID)` 作为会话 identity，并对 active session 使用 RunnerSnapshot，方向符合“配置刷新不漂移 live session”的约束。
- Core → Main → preload → Renderer 使用 typed bridge，Renderer 没有自行拼接 Core base URL。
- AG-UI、runtime reducer、narrative projector 和 `ExternalActionGate` 已建立 ACP typed variant；聚焦组件测试覆盖面不错，只需补真实生产事件形态的集成层。
- mock agent 已覆盖 initialize/new/prompt/permission 等基本互操作，适合作为后续 timeout、crash、malformed frame 和 process-tree 场景的基础。

## 7. Phase 5 与“Awaken 可被其他 ACP client 当作 Agent 调用”判断

结论：初次评审问题已完成大部分修改，但二次复核仍有 §10 所列阻塞；Phase 5 的产品决策可在方案层解阻，Phase 5 实现仍未开始，当前不能宣称 Awaken 已可作为 ACP Agent。

### 7.1 Phase 0–4 的前置阻塞

以下是实现者提交的修复清单；二次复核结果以 §10 为准，不能据此直接视为全部关闭：
- P0-1 cwd 已受 TaskScope containment 保护；
- P0-2 runner env 已 redacted，API 使用 set/remove 语义；
- P1-1 permission live 链路已支持 options/cancel/run_id 校验；
- P1-2 runner 子进程继承 `os.Environ()` 并支持大小写敏感覆盖；
- P1-3 turn timeout、bounded kill、process-tree cleanup 已落地；
- P1-4 并发 start 已加占位锁；
- P1-5 Conversation summary 数据竞争已用单一 mutex 修复；
- P1-6 ACP 事件已写入 SemanticJournal；
- P1-7 frame/output 限制与 tool summary 已修复；
- 建议项：probe 已增强、配置写入已原子化、HTTP bridge 测试已补充。

因此 Phase 5 不再被上述实现缺口阻塞。

### 7.2 Phase 5 产品/安全契约已收敛，尚待实现

初次评审时列出的八项产品/架构决策，现已在方案 §13 收敛，不再构成设计阻塞：

1. 默认创建可见 thread；client 可用 `_meta["awaken.ephemeral"]` 请求临时 thread，最终保留/删除权归 Awaken；
2. 信任本地 ACP client 提交的绝对 cwd，不弹 Desktop 授权；规范化后的 cwd 是 session 的单 root TaskScope；
3. 使用用户级 runtime manifest、随机 loopback 端口、每实例 token、用户级 ACL、runtime identity 与进程锁；
4. bridge 优先复用现有 Core；Core 不存在时默认拉起 Desktop，也可显式只拉起受管 headless Core；
5. 有限选项的运行时审批统一映射为 ACP `request_permission`，不提供 bypass；自由文本 `ask_user` 不在首版入站 ACP turn 中暴露；
6. launcher、bridge、Core session 和 Desktop 各自具有明确 lease、cancel、flush 与强制退出预算；
7. 首版不声明 ACP fs/terminal capability，仅支持 text prompt 与 Awaken 内部工具链；
8. thread/run 持久化 source/client/version，并用 compatibility suite 与至少两个真实 ACP client 验收。

当前阻塞已从“契约未决”转为“实现与证据未完成”：仓库中尚无 `cmd/awaken-acp`、runtime discovery manifest、Agent-side ACP adapter，也没有真实外部 client 的闭环验收。

### 7.3 建议的解阻顺序

1. 先修复本评审 P0/P1，并把方案 Phase 0–4 的勾选状态改回与证据一致；
2. 将方案 §13 已确认的 thread visibility、cwd 信任边界、permission ownership 和生命周期约束迁入对应功能域权威契约；
3. 实现 per-user discovery manifest、随机 loopback 端口、实例 token、runtime identity、进程锁与崩溃残留恢复，并完成同一 userData identity 的多入口竞争测试；
4. 实现最小 stdio launcher/bridge，只支持 initialize/new/text prompt/cancel/close：优先 attach，Core 缺失时按 `desktop | core | fail` 策略启动，不声明 fs/terminal；
5. 增加 Core semantic event → ACP update、Awaken approval → ACP permission 的单向映射，禁止双 gate；
6. 最后用 compatibility suite、Zed 和另一个真实 ACP client 完成多轮、工具、拒绝、取消、Desktop 退出和 token 失效验收。

只有上述退出条件都有本次自动化与真实 Desktop 证据后，才能更新“Awaken 可被其他 ACP client 当作 Agent 调用”为当前支持。

## 8. 本次验证记录

通过：

- `go test -timeout 60s -v ./src/acp/...`（含新增 protocol 包截断与 tool-summary 测试）
- `go test -timeout 30s -v ./src/tools/delegate_external_agent/... ./src/tools/configure_acp_runner/...`
- `go test ./src/protocol/agui/...`（在组合命令中通过）
- `go test -timeout 30s -v ./src/server/... -run 'TestHandleListRunners|TestHandleUpdateRunner|TestHandleResolvePermission'`：ACP route 专项测试通过
- `npm.cmd --workspace @awaken/desktop test -- ACPSettingsPanel ExternalActionGate run-narrative/projector runtime/reducer`：4 files / 103 tests passed
- `npm.cmd --workspace @awaken/desktop run typecheck`
- `git diff --check`：退出码 0，仅报告现有 LF/CRLF warning

未通过或未完成：

- 聚焦 Go 组合命令在 120 秒超时；拆分后 ACP 包通过。
- `src/agentcore` 全包测试有 4 个失败：2 个 composer skill selection、1 个 Windows Git CRLF diff、1 个本机 WSL/bash 环境失败；这些失败不直接证明由 ACP 改动引起，但意味着本次工作区无法给出全绿基线。
- `src/server` 全包在 `TestCompactCanaryFactsSurviveThreadRestoreSmoke` 的外部模型流等待中超时；但 ACP route 已有 `src/server/handlers_acp_test.go` 专项测试覆盖 redacted env、env_set/env_remove 与 run_id 校验。
- `go test -race` 因当前环境未启用 CGO而无法执行。
- 未按 README 正式入口启动 Awaken Desktop，未完成真实 UI 视觉验收。
- 未对 OpenCode、Qwen Code、Claude Agent ACP、Codex ACP 中任意两个真实 runner 做闭环验收。


## 9. 附录：QwenPaw 对 Phase 5 八个契约的解法分析

为辅助 Phase 5 决策，本节记录对 `D:/project/QwenPaw` 中 ACP Agent/Client 实现的交叉检查结果。核心文件：

- `src/qwenpaw/agents/acp/server.py` —— QwenPaw 作为 ACP Agent 被外部 client 调用。
- `src/qwenpaw/agents/acp/service.py` / `client.py` / `permissions.py` —— QwenPaw 作为 ACP Client 调用外部 Agent。
- `src/qwenpaw/cli/acp_cmd.py` —— ACP Agent 入口。
- `src/qwenpaw/agents/tools/delegate_external_agent.py` —— 委派工具。

### 9.1 可见 thread 还是 ephemeral thread

**QwenPaw 的做法**

- `server.py:537-561` 的 `new_session` 接收 `cwd` 和 `**kwargs`（meta）。
- `meta.py` 定义 `ACP_EPHEMERAL_META_KEY = "qwenpaw.ephemeral"`。
- `server.py:309-328` 的 `_session_info` 读取 meta：若 `qwenpaw.ephemeral=true`，在 session info 与 request_context 中标记，影响 Workspace 是否把对话当临时会话。
- session 进入 `self._sessions`，`list_sessions` 可枚举，默认可见；ephemeral 由 client 通过 meta 声明。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 默认 | 可见 session，可 list | 默认可见 thread |
| ephemeral | client 通过 meta 声明 | 提供 `ephemeral=true` 参数 |
| 删除/保留权 | QwenPaw 自己管理，`close_session` 删除 | 归用户，外部 client 只能请求关闭 |

**借鉴点**：用 `meta` 传递 `ephemeral` 标记合理，Awaken 可让 ACP client 在 `session/new` 的 meta 中声明临时会话。

### 9.2 workspace 如何由用户授权

**QwenPaw 的做法**

- `server.py:537-561` 的 `new_session` 直接接收 `cwd` 字符串，没有弹出用户授权。
- 安全边界落在后续 tool 调用：`permissions.py:234-251` 的 `_is_hard_blocked` 检查 tool path 是否以 `self.cwd` 开头。
- `ACPPermissionAdapter.__init__` 把 `cwd` resolve 为绝对路径作为 containment root。
- 但判断是字符串前缀匹配 `str(resolved).startswith(self.cwd)`，存在 `C:\work` 被 `C:\work2` 绕过的已知问题。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 授权时机 | 无显式授权，直接信任 client cwd | 无弹窗授权，信任本地 ACP client 提交的 cwd |
| containment | 前缀字符串匹配 | `filepath.Clean/Abs/EvalSymlinks` + 路径组件级判断 |
| 多 root | 未特殊处理 | 每个 ACP session 以自己的 cwd 建立单 root TaskScope |

**决策**：Awaken Phase 5 不增加 Desktop 授权弹窗或 workspace allowlist。`awaken-acp` 是由当前用户主动配置并启动的本地 stdio Agent，信任该 client 传入的绝对 cwd；Core 只做存在性、目录类型、规范化和 symlink 解析，不做“是否允许使用该目录”的二次授权。规范化后的 cwd 成为该 session 的 TaskScope root，后续工具仍不得越出此 root。QwenPaw 的字符串前缀判断不能照搬。

### 9.3 discovery manifest、token 轮换和多实例 identity

**QwenPaw 的做法**

- **没有 discovery manifest**。`cli/acp_cmd.py` 直接通过 `qwenpaw acp` 命令启动 stdio ACP server。
- 每次外部 client 调用都启动一个**新的独立进程**，带自己的 `--workspace` / `--agent`。
- 没有 loopback HTTP、随机端口、token。
- 多实例：靠进程隔离，每个 `qwenpaw acp` 是独立 Workspace。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 启动方式 | 独立 CLI 启动新 Workspace | launcher 优先连接现有 Core，缺失时按策略拉起 Desktop 或受管 Core |
| discovery | 无，client 自己启动进程 | identity 级 manifest + instance-scoped token |
| 多实例 | 允许多个独立进程 | 每个 userData identity 一个可发现 Core，多个 bridge 复用 |
| 权限边界 | 依赖操作系统进程启动者 | 本机用户边界 + manifest ACL + 实例 token |

**关键差异**：Awaken 借鉴 QwenPaw 的 client-owned stdio 启动体验，但不为每个 client 无约束地创建一套独立运行时。launcher 通过 discovery、runtime identity 和 lease 在 attach、启动 Desktop、启动受管 headless Core 三种路径间收敛到同一 Core ownership 模型。

### 9.4 fs/terminal capability 是否接入

**QwenPaw 的做法**

- `server.py:508-535` 的 `initialize` 返回的 `AgentCapabilities` 只声明 `load_session` 和 `session_capabilities`（close/list/resume）。
- **没有声明 fs 或 terminal capability**。
- 但 QwenPaw 启动完整 Workspace，内部有文件/工具能力；这些能力通过 ACP `tool_call` + `request_permission` 暴露，而非 ACP fs/terminal API。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| capability 声明 | 不声明 fs/terminal | 不声明 fs/terminal |
| 实际能力 | 通过 tool_call/permission 间接暴露 | 仅 text prompt + 内部工具映射 |
| 降级 | 外部 client 请求 fs/terminal 会 method-not-found | 返回 unsupported |

**借鉴点**：与 Awaken 推荐一致，首版不直接声明 fs/terminal，把文件/工具操作包装成 ACP tool calls 并走 permission gate。

### 9.5 `ask_user` 与 ACP `request_permission` 如何避免双 gate

**QwenPaw 的做法**

- `server.py:815-919` 的 `_bridge_approval_requests` 轮询内部 `ApprovalService` 的 pending approval。
- 每个内部审批请求被转换成 ACP `request_permission` 发给外部 client（`_request_approval_decision`）。
- 外部 client 响应后，再把结果 resolve 回 `ApprovalService`。
- 另外提供 `MODE_BYPASS = "bypassPermissions"` 的 session config option（`server.py:259-260`），可关闭所有 tool guard；Awaken 方案 §4.2 已明确不应照搬。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| gate 映射 | 内部 approval → ACP permission request | 有限选项 approval → ACP permission request；自由文本 ask_user 结束当前 turn |
| bypass | 提供 `Bypass Permissions` 模式 | 不提供全局 bypass |
| 死锁防护 | `asyncio.wait` 等待 first completed | 需确保 cancel/timeout 必回 cancelled |

**借鉴点**：把有限选项审批统一桥接到 ACP permission 的思路可采用，但 `bypassPermissions` 不能学。自由文本 `ask_user` 不应伪装成 permission；首版让 assistant 结束当前 turn，由 client 用下一条 prompt 继续，避免两类 gate 互相等待。

### 9.6 Desktop 退出时是否允许 bridge 阻止退出

**QwenPaw 的做法**

- QwenPaw 的 ACP server 是独立 CLI 进程，没有"Desktop 退出"问题。
- 进程退出时：`run_qwenpaw_agent` 的 `finally` 调用 `_shutdown_workspace`（`server.py:1378-1382`）。
- 作为 ACP Client 时：`service.py:495-518` 注册 `atexit.register(_shutdown_acp_services)`，退出时关闭所有 session 并杀进程树。
- `_close_conversation`（`service.py:408-435`）先 `close_session`（5 秒超时），再 `_kill_process_tree`。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 退出触发 | CLI 进程结束 / atexit | Desktop 退出信号 |
| 清理策略 | close_session + kill tree | cancel → close → kill tree |
| 是否阻塞退出 | 不阻塞，靠 atexit 尽力清理 | 不阻塞，设强制清理预算 |

**借鉴点**：`_kill_process_tree` 用 `psutil` 递归杀子进程的实现（`service.py:29-44`）可参考，Awaken 已有类似逻辑但用 Go 实现。

### 9.7 单实例规则与 Desktop 未运行语义

**QwenPaw 的做法**

- 没有"Desktop"概念。ACP Agent 就是 `qwenpaw acp` 这个 CLI。
- 外部 client 自己负责启动进程；调用时 `spawn_agent_process`（`service.py` 作为 client 时）或直接运行 `qwenpaw acp`。
- 多实例：可启动多个 `qwenpaw acp`，每个独立 Workspace。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 入口 | 独立 CLI `qwenpaw acp` | stdio launcher/bridge，支持 attach 或受管启动 |
| 未运行语义 | 外部 client 自己启动 | `awaken-acp` 按启动策略拉起 Desktop，或只拉起受管 Core |
| 单实例 | 不限制 | 每个 Awaken userData identity 最多一个可发现 Core；多个 bridge 复用它 |

**决策**：`awaken-acp` 本身始终是外部 client 启动的 stdio 入口。它优先连接已存在的 Core；若不存在，默认拉起 Desktop 并等待其 Core ready，也支持显式 headless 策略只启动 Core。headless Core 必须由 launcher 持有 lease、写入同一 runtime discovery、在最后一个 lease 释放后按 idle budget 退出，不能成为用户手工维护的第二套常驻服务。

### 9.8 来源标识、审计与真实外部 client 验收

**QwenPaw 的做法**

- 来源标识：`server.py:1164-1171` 的 `_session_meta` 在 session `_meta` 里写入 `qwenpaw.agent`（agent id）。
- 错误标识：`ACP_ERROR_META_KEY = "qwenpaw.error"` 用于标记错误消息。
- ephemeral 标记：`ACP_EPHEMERAL_META_KEY`。
- 审计：依赖 Workspace 自身的 chat/session 持久化。
- 验收：`e2e/tests/test_acp.py` 主要测试 QwenPaw 作为 ACP client 调用 mock agent；作为 ACP server 的验收依赖外部 client 集成。

**与 Awaken 方案对比**

| 维度 | QwenPaw | Awaken 推荐 |
|---|---|---|
| 来源标记 | `_meta` 里放 `qwenpaw.agent` | thread/run 元数据记录 source/client/version |
| 审计 | Workspace 自带 chat 持久化 | SemanticJournal + 专用 ACP semantic kinds |
| 兼容性验收 | e2e 测试覆盖 | 用 Zed/真实 ACP client 实测 |

**借鉴点**：用 `_meta` 传递来源信息可行，Awaken 可类似地在 ACP session/turn 的 diagnostic 里记录 `source=acp-bridge` 和 client identity。

### 9.9 总结：QwenPaw 与 Awaken Phase 5 的关键分歧

| 问题 | QwenPaw 选择 | Awaken 推荐 | 是否建议照搬 |
|---|---|---|---|
| 1. thread 可见性 | 默认可见，meta 控制 ephemeral | 默认可见，支持 ephemeral | ✅ 可参考 meta 设计 |
| 2. workspace 授权 | 直接信任 client cwd，后续 tool 做前缀 containment | 信任 client cwd；规范化后作为 session TaskScope root | ✅ 信任模型一致，但必须替换前缀匹配 |
| 3. discovery/token | 无 manifest，独立 CLI 启动 | identity 级 manifest + instance-scoped token | ❌ 与 Awaken 架构相反 |
| 4. fs/terminal | 不声明，能力通过 tool/permission 暴露 | 不声明 | ✅ 一致 |
| 5. 双 gate | 内部 approval → ACP permission | 有限选项 approval → ACP permission；自由文本 ask_user 结束 turn | ✅ 审批桥接思路一致，不可混淆两类 gate 或增加 bypass |
| 6. 退出清理 | atexit + kill tree | 强制清理预算 | ✅ 策略一致 |
| 7. 单实例/未运行 | 多独立 CLI 实例 | 优先复用 Core；缺失时拉起 Desktop 或受管 headless Core | ⚠️ 借鉴 client-owned 启动，不采用无约束多 Core |
| 8. 来源/审计 | `_meta` + Workspace 持久化 | SemanticJournal + source 元数据 | ✅ 可参考 `_meta` |

**结论**：QwenPaw 的独立 stdio Agent 证明了“client 负责启动、直接信任 cwd”的产品模型可行。Awaken 采用这两点，并借鉴其 **approval bridge、进程树清理、capability 不声明、meta 标记来源**；差异在于 Awaken 通过 launcher/discovery/lease 复用或受管启动同一 userData identity 的 Core，不允许无约束的多个 Core 同时写同一运行时数据。

## 10. 二次复核结论（当前）

### Review 总结

- 整体结论：**本轮二次复核问题已修复**。
- 初次评审中的 cwd containment、env value redaction、子进程环境继承、turn deadline、并发 start、Conversation mutex、配置原子写入、真实 initialize probe 和 tool summary 均已有实质修复。
- 二次复核发现的 1 个 P0、5 个 P1 已在当前工作区修复并通过聚焦测试；仍缺少贯穿 ToolRuntime、SemanticJournal、AG-UI、Desktop 和 HTTP resolve 的真实 permission 集成测试（依赖真实 Desktop + mock runner 或真实 runner）。

### P0（必须修）

#### P0-1 permission requested 被缓冲到工具结束后，真实运行会形成闭环死锁 —— 已修复

位置：`src/agentcore/semantic_journal.go:317`、`src/agentcore/runner.go:927`

**修复**：

- `artifactBufferingSink.Emit` 现在对 ACP 事件（session/turn/tool/permission）立即调用 `s.inner.Emit(event)` 发布 live，同时保留在 `semanticFacts` 中用于最终 batch 持久化。
- `emitToolResultBatch` 构建最终 live 列表时跳过 ACP 事件，避免重复发布；仍将其转为 `SemanticRecord` 持久化到 journal。

这样 permission gate、turn progress 和 tool snapshot 在工具执行期间即可到达 Desktop，不会死锁。

### P1（应该修）

#### P1-1 pending permission 仍把 tool call ID 当成 run ID —— 已修复

位置：`src/acp/manager.go:479`、`src/acp/manager_test.go:129`

**修复**：

- `TurnState` 新增 `HostRunID` 字段；`Conversation.StartTurn` 同时接收 host run ID 与 tool call ID。
- `Manager.Prompt/Message` 增加 `hostRunID` 参数；`delegate_external_agent/tool.go` 通过 `ToolExecutionIdentityFromContext` 读取真实 `RunID` 并传入。
- `managerPermissionHandler.RequestPermission` 使用 `h.turn.HostRunID` 填充 `PermissionRequest.RunID`。
- `manager_test.go` 与 `manager_events_test.go` 中 run ID 与 tool call ID 已改为不同值（`run-1` vs `tool-call-1` / `tool-call-perm`）。

#### P1-2 runner redacted env 的 wire 类型与 Desktop 类型不一致 —— 已修复

位置：`src/server/handlers_acp.go:16`、`apps/desktop/src/shared/runtime.ts:1040`、`apps/desktop/src/renderer/settings/ACPSettingsPanel.tsx:133`

**修复**：

- `apps/desktop/src/shared/runtime.ts` 新增 `ACPRunnerEnvVar` 与 `ACPRunnerView`：read DTO 的 `env` 为 `{key, has_value}[]`；`ACPRunnerCreateRequest` / `ACPRunnerUpdateRequest` 保持 write DTO 语义。
- `preload/index.ts`、`renderer/runtime/client.ts`、`main/index.ts` 的 create/update 返回类型改为 `ACPRunnerView`。
- `ACPSettingsPanel.tsx` 内部状态使用 `ACPRunnerView`；编辑时从 `env.map(e => e.key)` 生成文本；保存 update 时计算 `env_set/env_remove`；builtin 模板通过 `templateToView` 转换为 view。
- `src/server/handlers_acp_test.go` 已覆盖 redacted env 的 wire shape。

#### P1-3 ACP semantic record 没有 replay projector —— 已修复

位置：`src/agentcore/semantic_journal.go:247`、`src/runtime/journal_store.go:787`

**修复**：

- `src/runtime/journal_store.go` 新增 `acpReplayEvent` helper，将 `SemanticKindACP*` 投影为对应的 `AwakenEvent`（`ACPSessionStarted/Closed/Interrupted`、`ACPTurnStarted/Finished`、`ACPToolSnapshot`、`ACPPermissionRequested/Resolved`）。
- replay 生成的 permission requested 是只读历史说明，不恢复交互按钮；gate 只在当前进程内存在且 identity 匹配的 pending request 时生成。
- payload 中除标准字段外的内容重新放入 `Diagnostic`，保留 runner/session/turn 等元数据。

#### P1-4 run cancel 被错误归类为 turn timeout —— 已修复

位置：`src/acp/manager.go:295`

**修复**：

- `src/acp/errors.go` 新增 `ErrCodeTurnCancelled = "acp_turn_cancelled"`。
- `Manager.normalizeTurnResult` 区分 `context.DeadlineExceeded`（`acp_turn_timeout`）与 `context.Canceled`（`acp_turn_cancelled`），两者 `StopReason` 均使用 `cancelling`；其他错误仍为 `acp_transport_closed`。

#### P1-5 assistant text 截断可能切断 UTF-8 code point —— 已修复

位置：`src/acp/protocol/client.go:273`

**修复**：

- `src/acp/protocol/client.go` 在 `promptState.onUpdate` 截断时，把 `allowed` 向前移动到前一个 UTF-8 rune 起始字节再写入 `strings.Builder`。
- 新增 `TestPromptStateTruncatesAtUTF8RuneBoundary` 断言中文跨边界截断后文本仍为合法 UTF-8。

### 建议

1. `src/acp/manager.go:44` 的注释称 nil factory 会使用 production factory，实际只会在 probe 返回错误、在 start 时发生 nil 调用风险；应删除该承诺或通过无循环依赖的构造器强制注入。
2. `src/acp/manager.go:333` 的显式 Cancel 只发送 notification，没有更新 `cancelling` state；建议让 status、事件和最终 terminal 使用同一状态转换入口。
3. Desktop ACP 设置测试应加入真实 wire DTO fixture，避免 TypeScript 只验证 mock shape。

### 优点

- `ResolveACPCwd` 已使用 lexical + resolved containment，并覆盖 workspace 外路径、symlink escape 和 protected root，修复方向正确。
- permission resolution 已改为显式 `cancelled` 字段并校验 exact option；只需修正真实 run identity 和 live 发布时机。
- Windows/Unix process tree、配置 replace、真实 ACP initialize probe 的 owner 已集中到 Core，结构比初版清晰。

### 二次验证

通过：

- `go test -timeout 60s ./src/acp/... ./src/acp/protocol/... ./src/tools/delegate_external_agent/... ./src/tools/configure_acp_runner/... ./src/server/... -run 'TestPromptState|TestManager|TestRegistry|TestSaveConfigFile|TestMergeProcessEnv|TestHandle'`
- `go test -timeout 60s ./src/agentcore/... -run 'ACP|Semantic'`
- `go test -timeout 60s ./src/runtime/... -run 'TestSemanticRecordToAgentEvents|TestJournal'`
- `npm.cmd --workspace @awaken/desktop test -- ACPSettingsPanel ExternalActionGate run-narrative/projector runtime/reducer`：4 files / 103 tests passed
- `npm.cmd --workspace @awaken/desktop run typecheck`
- `git diff --check`：退出码 0，仅报告既有 LF/CRLF warning

未通过或未执行：

- `go test ./src/runtime/...` 全包仍有 Windows 文件句柄锁定导致的既有测试失败（`TestFileStoreLifecycleJournalRollForwardAndSnapshotRebuild`），与本次 ACP 改动无关。
- 未运行真实 Desktop，也未使用真实 ACP runner 完成 permission、cancel 与 replay 闭环。
