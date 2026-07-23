# ACP 功能代码评审

## 1. 评审结论

当前分支已经搭出了较完整的 ACP Client 骨架，并新增了 Phase 5 ACP Agent bridge、runtime discovery/control route、lease、Desktop control client 和 `cmd/awaken-acp`。第三方 SDK 仍被限制在 `src/acp/protocol` 与 `src/acp/agentbridge`，总体分层方向合理。

初次至第四次复核问题均已有针对性修改。第五次复核使用 README 正式入口、真实 Desktop 和独立 stdio ACP client adapter 验收，发现入站 prompt 缺少模型配置来源。第六次复核已按“client 只选择 Awaken 已配置 profile ID，不得注入 API key；未选择使用默认/首个 ready profile”完成修复，并用 Desktop-owned Core 真实 stdio prompt 验证通过。当前代码评审结论为 **模型选择 P0 已解阻，未发现新的合并阻塞代码问题**；headless 与第三方 client 正式验收仍未完成。

本文件 §3–§13 保留前五次评审及修复说明，**当前复核结论以 §14 为准**。Phase 5 的 transport、session、runtime discovery、模型 selector 与控制面已经实现；compatibility suite、Zed/另一真实 client 和 headless 多进程验收完成前，仍不宣称 Phase 5 全部验收通过。

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

## 10. 二次复核结论（已修复基线）

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

## 11. 三次复核结论：Phase 5 当前实现

### Review 总结

- 评审基线：`origin/main...HEAD`，共 106 个文件，约 `+12522/-255`；其中 Phase 5（`65409fd0..HEAD`）为 34 个文件，约 `+3998/-29`。
- 整体结论：**request changes**。
- 二次复核的 run identity、env DTO、cancel/timeout 分类、UTF-8 边界和 replay projector 已修复；permission live gate 不再等待工具结束才出现。
- Phase 5 目前是可编译的骨架，不是可交付链路。生产 launcher 仍有 placeholder，bridge 使用了错误的 AG-UI event 名，permission 回写到错误的 Core subsystem，session close 不能保证取消 run，Desktop/headless Core 也没有实现方案要求的单实例接管。
- 当前发现：**5 个 P0、8 个 P1**。

### P0（必须修）

#### P0-1 launcher 的三个生产启动策略没有真正接通

位置：`src/acp/agentbridge/launcher.go:219`、`src/acp/agentbridge/launcher.go:279`、`src/acp/agentbridge/launcher.go:284`、`cmd/awaken-acp/main.go:40`、`scripts/build-binary.mjs:47`

当前生产路径存在以下阻塞：

1. `createLease` 默认返回硬编码的 `lease_placeholder`，`releaseLease` 是 no-op；测试还把该 placeholder 写成期望值。
2. `defaultSpawnDesktop` 在所有平台都直接返回 `desktop spawn not implemented`，所以默认 `--startup=desktop` 在没有 manifest 时必定失败。
3. `cmd/awaken-acp` 没有设置 `CoreBinaryPath`，因此 `--startup=core` 会在 `core binary path is required` 处失败。
4. 即使传入 binary，launcher 只设置 `AWAKEN_OWNER_MODE=headless`，但 Core 读取的是 `--owner-mode` flag，实际仍以 `desktop` owner 启动。
5. 构建和打包脚本只生成 `awaken-core`，没有生成或随安装包发布 `awaken-acp`，外部 client 没有稳定 command 可配置。

这意味着除“碰巧存在一个可读 manifest”的路径外，文档声明的 Desktop auto-start 和 managed headless Core 都无法使用；existing-manifest 路径也没有真实 lease。

建议修改：

1. 删除 hook 之外的 placeholder，实现真实 authenticated lease create/release，并先完成 identity endpoint 校验。
2. 为 Windows/macOS/Linux 实现正式 Desktop 启动器；为 packaged/dev identity 共享唯一的 executable/userData resolver。
3. 从 `awaken-acp` 自身安装位置解析配套 `awaken-core`，spawn 时传 `--owner-mode=headless`。
4. 构建、签名和打包两个 binary，补一个不替换 hook 的 packaged-layout 启动测试。

#### P0-2 bridge 使用不存在的 AG-UI event 名，文本/工具更新不会发送且失败 run 会伪装成成功

位置：`src/acp/agentbridge/session.go:78`、`src/acp/agentbridge/agent.go:215`、`src/acp/agentbridge/coreclient.go:35`

`handleFrame` 匹配 `TEXT_DELTA`、`TOOL_CALL_STARTED`、`TOOL_CALL_FINISHED`、`TOOL_RESULT`，但当前 wire 的真实类型是：

- `TEXT_MESSAGE_CONTENT`，文本位于 `frame.Delta`；
- `TOOL_CALL_START`；
- `TOOL_CALL_END`；
- `TOOL_CALL_RESULT`。

因此正常 Awaken run 的 assistant 文本与工具进度会全部落入 default 分支被静默丢弃。即使收到 `RUN_ERROR` 或失败 terminal，当前代码也不记录 terminal outcome；SSE 关闭后 `Agent.Prompt` 一律返回 `StopReasonEndTurn`。此外同一个带 30 秒总超时的 `http.Client` 被用于 SSE，合法的长 turn 会在 30 秒被主动断开。

工具状态还使用 `"running"`，而 ACP v1 的有效值是 `pending | in_progress | completed | failed`。

建议修改：

1. 直接使用 `agui.Event*` 常量匹配，文本读取 `Delta`，ACP 状态使用 SDK 常量。
2. 建立明确的 terminal accumulator：`RUN_FINISHED`、`RUN_ERROR`、cancelled 和 transport closed 分别映射对应 ACP stop reason/error，不能把 stream EOF 当成功。
3. SSE 使用没有整体 request timeout、但有 dial/header/idle watchdog 的独立 client。
4. 增加真实 `RunService -> router SSE -> bridgeSession -> AgentSideConnection` 集成测试，断言文本、工具、错误、cancel 和超过 30 秒的 turn。

#### P0-3 permission 选择被回写到错误的 Core route，真实 ACP option 也会被误判

位置：`src/acp/agentbridge/session.go:152`、`src/acp/agentbridge/session.go:207`、`src/acp/agentbridge/session.go:218`

bridge 监听的是 `AWAKEN_ACP_PERMISSION_REQUESTED`，它对应 ACP Client manager 中的 pending permission；但选择后调用的是通用 `/external-interactions/{id}/resolve`。这两个 pending map 完全不同，所以真实请求会得到 not found，原 ACP permission 继续等待直到超时。

同时 `permissionOptionKind` 只识别 `"allow"` / `"deny"`，ACP v1 实际 kind 是 `allow_once`、`allow_always`、`reject_once`、`reject_always`。当前测试使用了协议中不存在的简化 kind，掩盖了错误。

建议修改：

- 对 `AWAKEN_ACP_PERMISSION_REQUESTED` 调用现有 `/v1/runs/{run_id}/acp-permissions/{interaction_id}/resolve` 语义，携带 exact `option_id` 或 `cancelled=true`，由 Core pending record 校验 run/session/runner/option。
- 若 Phase 5 还要把 Awaken 自己的 finite-choice external interaction 转成 ACP permission，应监听 `AWAKEN_EXTERNAL_INTERACTION_REQUESTED` 并建立独立映射，不能混用两个 interaction namespace。
- 增加真实等待测试：Core pending goroutine 在 client allow/reject/cancel 后解除，错误 option 与迟到响应被拒绝。

#### P0-4 `session/close` 和 stdio EOF 不会可靠取消 active Core run

位置：`src/acp/agentbridge/session.go:29`、`src/acp/agentbridge/session.go:61`、`src/runtime/acp_session.go:128`、`src/runtime/acp_session.go:147`

`ACPSessionManager.SetCurrentRun/ClearCurrentRun` 没有任何生产调用，Core mapping 的 `CurrentRunID` 始终为空。bridge close 时先 cancel `s.ctx`；`Prompt` 收到 `s.ctx.Done()` 后直接返回，没有调用 `CancelRun`。随后 Core `CloseSession` 也因 `CurrentRunID == ""` 无法取消 run。

结果是 `session/close`、stdio EOF 或 bridge signal 可以让 client 认为会话已关闭，但模型和工具仍在 Core 中继续执行副作用。`Agent.Cancel` 读取 `session.currentRunID` 时也没有获取 session mutex，并发 prompt/cancel 存在 data race。

建议修改：

1. 用 session-specific authenticated start-run control route 原子建立 `session -> run` 关联，或在创建 run 后立即 `SetCurrentRun`、terminal 后 `ClearCurrentRun`。
2. `ctx.Done`、`s.ctx.Done`、close 和 EOF 都必须在有界 context 中调用同一个幂等 cancel-and-wait-terminal 流程。
3. 所有 `currentRunID` 读写统一受 `bridgeSession.mu` 保护，并防止两个并发 prompt 都通过空值检查。
4. 测试必须查询 Core run 状态，证明 close/EOF 后不是只清了 bridge map，而是真正形成 cancelled terminal。

#### P0-5 Desktop/headless Core 的单实例 ownership 与 lease 交接尚未实现

位置：`apps/desktop/src/main/runtime-supervisor.ts:391`、`apps/desktop/src/main/runtime-supervisor.ts:663`、`cmd/awaken-core/main.go:211`、`cmd/awaken-core/main.go:216`、`src/acp/agentbridge/launcher.go:20`、`src/acp/agentbridge/launcher.go:127`

当前 ownership 链路有多处互相冲突：

- Desktop supervisor 总是先 spawn 自己的 Core，之后才读取 manifest；它从未在 spawn 前发现并 attach 已有 headless Core。
- Desktop 计算的 expected identity 是规范化路径字符串，而 Go 是 `packaged:project_<sha256前12字节>`，校验必然失败；错误又被降级为 non-fatal。
- desktop-owned Core 把 `LeaseManager` 留为 nil，因此 bridge 的真实 create-lease route 会返回 503。
- 所有 bridge 共用固定 owner ID `awaken-acp-bridge`，会拿到同一个 lease；任一 bridge release 后，其余 bridge 的 heartbeat 失效。
- 启动 headless Core 的 launcher 在自己退出时直接 `Process.Kill`，无视其他 bridge lease、Desktop lease和 active run。
- 每个 Core 都无条件覆盖 manifest，退出时也无条件删除；两个 Core 竞争时，一个实例可以覆盖或删除另一个实例的 discovery record。

这违反 Phase 5 最核心的“同一 userData identity 只有一个可发现 Core”和 lease ownership，不仅会留下孤儿，还可能杀掉仍被其他 client 使用的 Core。

建议修改：

1. Desktop supervisor 启动前执行与 launcher 相同的 manifest + authenticated identity 校验，命中 headless Core 时进入明确的 `external-attached` 状态而不是 spawn。
2. 所有 Core 都提供 LeaseManager；只有 headless owner 根据 lease/active-run/idle budget决定自退，desktop owner 仅记账。
3. 每个 bridge 使用随机 UUID owner ID；launcher 退出只 release lease，不直接 kill 已发布 manifest 的 Core。
4. manifest publish/delete 使用 instance compare-and-swap：只能删除自己仍拥有的 manifest。
5. runtime identity 算法只由一处共享或以 manifest 值为准，Desktop 不手写另一套不等价算法。
6. 增加真实多进程测试：两个 bridge 并发、Desktop 接管 headless、任一 bridge 退出、Desktop 退出和 Core crash 都不产生重复 Core或误杀。

### P1（应该修）

#### P1-1 LeaseManager 会在读锁下修改 map，且没有实现 30 秒 idle budget

位置：`src/runtime/lease.go:162`、`src/runtime/lease.go:185`

`LeaseCount` 和 `shouldIdleShutdown` 持 `RLock` 后调用 `reapExpiredLocked` 删除 map entry，属于错误的锁模式；并发 heartbeat/create/reap 时可能 data race 或 map 并发访问。

`headlessIdleBudget` 常量没有被使用。manager 在第一次发现零 lease 时立即调用 callback 并退出：没有等待 30 秒；若 callback 因 active run 返回，reaper 仍永久退出，run terminal 后也不会重新开始 idle 倒计时。

建议使用单一写锁完成 reap + snapshot，并显式维护 `idleSince`/timer；新 lease 或 active run 重置 idle，只有连续 idle 满 budget 才 shutdown。

#### P1-2 visible/ephemeral、来源 metadata 和 client identity 没有按契约落盘

位置：`src/acp/agentbridge/agent.go:153`、`src/runtime/acp_session.go:126`、`src/runtime/acp_session.go:173`

- 方案定义 `_meta["awaken.ephemeral"]=true`，实现却读取字符串 `awaken_visibility`。
- `CloseSession` 明确没有删除 ephemeral thread，也没有 cleanup worker，Phase 5 checklist 却勾选了 ephemeral 生命周期。
- `InitializeRequest.ClientInfo` 没有保存；NewSession 又从非标准 meta 猜 `client_name/client_version`。
- `ensureProjectAndThread` 直接丢弃 request，thread 没有持久化 `source=acp_bridge`、client、ACP session、canonical cwd 等审计 metadata。

建议统一使用已确认 meta key，保存 initialize client info，在首次 run/thread 创建时写 durable metadata，并为 ephemeral close 增加 terminal/no-lease 条件和可重试清理任务。

#### P1-3 control token 没有覆盖 bridge 实际使用的 run/event/cancel/permission routes

位置：`src/acp/agentbridge/coreclient.go:247`、`src/acp/agentbridge/coreclient.go:273`、`src/acp/agentbridge/coreclient.go:296`、`src/acp/agentbridge/coreclient.go:316`

session mapping route受 Bearer token 保护，但 bridge 随后使用无 token 的普通 `/v1/runs`、SSE、cancel 和 external-interaction route。方案 §13.4 要求 bridge session/run/event/permission 全部进入 authenticated control surface；当前 control token 只保护发现和空 mapping，无法保护真正的控制动作。

建议新增窄的 `/v1/control/acp/sessions/{id}/runs...` surface，在 Core 内校验 session/thread/run ownership；bridge 不再直接调用 Desktop 公共 run route。

#### P1-4 Windows 默认 userData 与 Desktop identity 不一致

位置：`src/acp/agentbridge/launcher.go:247`、`apps/desktop/src/main/index.ts:129`

launcher 在 Windows 使用 `%LOCALAPPDATA%\@awaken\desktop`，Electron Desktop 明确使用 `app.getPath("appData")`，通常对应 `%APPDATA%`。默认 `awaken-acp` 因此不会读取 Desktop 发布的 manifest，而会操作另一套目录。

建议把 identity/userData 解析提取成跨语言 golden contract，并至少用 Windows packaged/dev fixture 验证 launcher 与 Desktop 路径完全一致。

#### P1-5 session 输入处理会静默丢失合法内容并拒绝常见 cwd

位置：`src/acp/agentbridge/agent.go:239`、`src/runtime/acp_session.go:215`

- 多个 text block 最终只返回 `parts[0]`，其余文本静默丢失。
- 非空 `additionalDirectories` 与 `mcpServers` 被静默忽略；首版不支持时应明确返回 unsupported/invalid params。
- `CanonicalizeCWD` 对本地文件路径调用 `url.ParseRequestURI`，带空格等合法文件名可能被 URI 规则拒绝；路径有效性不应由 URI parser决定。

建议拼接全部 text block；对未支持字段 fail closed；cwd 只做平台路径、存在性、目录和 symlink canonicalization。

#### P1-6 SSE 没有实现 cursor 重连

位置：`src/acp/agentbridge/session.go:52`、`src/acp/agentbridge/coreclient.go:319`

每个 prompt 只以 cursor 0 建一次 SSE；bridge 不记录 `streamCursor`，任何瞬时断线都直接结束 prompt。方案要求按 `(instance_id, run_id, seq)` 去重并从最后 cursor 重连，当前 checklist 将 event mapping 标成完成但缺少该恢复能力。

建议跟踪最后确认 cursor，对 retryable transport error 做有界重连；Core identity 改变或 run terminal 后禁止重放 prompt。

#### P1-7 ACP live event 仍不是“立即 durable”

位置：`src/agentcore/semantic_journal.go:340`、`src/agentcore/runner.go:1888`

二次复核的 UI 死锁已解除：ACP event 会立即 live publish，最终 batch 也避免重复 live。但 semantic record 仍要等 `delegate_external_agent` 整个工具结束才写 journal。Core 在 permission 等待或长 turn 中崩溃时，已展示的 session/turn/tool/permission fact 会从 replay 消失。

建议为 ACP live facts 提供独立 assigned append；最终 tool-result batch 不再重复持久化这些 record，并增加 crash-at-permission/restart replay 测试。

#### P1-8 当前支持文档互相冲突且多处超前声明

位置：`README.md:32`、`docs/02_主智能体闭环/索引.md:10`、`docs/02_主智能体闭环/ACP智能体协作设计与实施方案.md:21`、`docs/03_运行时与本地服务/Desktop本地服务生命周期.md:59`

README 和方案把 Phase 5 launcher、event projection、permission、cancel/close 写成当前支持；02 域索引仍明确写“Phase 5 目标，不在当前支持范围”。当前代码与测试支持后者，且方案 checklist 勾选了尚未实现的 Desktop attach、真实 lease、ephemeral lifecycle。

修复代码前应把权威文档恢复为“当前不支持/已知缺口”；代码与真实验收完成后再一次性切换为当前支持，不能用单元测试存在替代产品事实。

### 建议

1. 禁止 production 函数保留 `placeholder` 并让测试断言 placeholder；未实现 stage 应显式返回 typed not-implemented。
2. agentbridge 测试应复用 `agui.Event*` 与 ACP SDK enum，不手写 `"TEXT_DELTA"`、`"running"`、`"allow"` 等协议中不存在的字符串。
3. 增加一条无 hook 的进程级 smoke：构建两个 binary，启动 `awaken-acp --startup=core`，通过真实 stdio ACP connection 完成 initialize/new/prompt/text/tool/cancel/close，再断言进程树和 manifest。
4. `go test -race` 本机因 CGO 未启用无法执行；lease/session 并发修复后应在支持 race detector 的 CI 强制运行。

### 优点

- Phase 5 类型被集中在 `src/acp/agentbridge`，没有把 ACP SDK 泄漏进 runtime/server/Desktop。
- manifest 使用随机 loopback、实例 token、原子写和 identity endpoint，目标安全边界方向正确。
- cwd 明确采用“信任本地 client、无需 Desktop 授权”，并复用正常 project upsert，符合已确认产品决策。
- 二次复核的 env read/write DTO、run identity、cancel/timeout 分类、UTF-8 和 replay projector 均有对应测试。

### 本次验证

通过：

- `go test -count=1 -timeout 180s ./src/acp/...`：`src/acp`、`src/acp/agentbridge`、`src/acp/protocol` 通过。
- `go test -count=1 -timeout 60s ./src/runtime ./src/server -run 'Test(Lease|CanonicalizeCWD|StableCWDHash|ACP|Control|HandleCreateACP|HandleGetACP|HandleCloseACP)'`。
- `go test` 组合中的 `src/protocol/agui`、`delegate_external_agent`、`configure_acp_runner`、`cmd/awaken-core` 通过。
- `go build ./cmd/awaken-acp` 与 `go build ./cmd/awaken-core`。
- `npm.cmd --workspace @awaken/desktop test -- runtime-control runtime-supervisor ACPSettingsPanel ExternalActionGate run-narrative/projector runtime/reducer`：6 files，116 passed，2 skipped。
- `npm.cmd --workspace @awaken/desktop run typecheck`。

未通过或未执行：

- 扩大 Go 组合测试未全绿：`src/runtime` 有多项 Windows 文件句柄/权限及既有环境失败；`src/server` 包含 400 response、`pongpong` 与 compact smoke timeout；`src/agentcore` 包含 composer、CRLF diff 和本机 WSL/bash 失败。本轮没有足够证据把它们全部归因于 ACP，也不能给出全绿声明。
- `go test -race` 因本机未启用 CGO无法执行。
- `git diff --check origin/main...HEAD` 未通过：`src/acp/process_windows.go:39` 存在 EOF 多余空行。
- 未按 README 正式入口运行真实 Desktop，未使用 compatibility suite、Zed 或另一个真实 ACP client。

### 11.2 本次修复说明（当前工作区）

针对 §11.1 中 5 个 P0 与高优先级 P1 进行了实现修复，要点如下：

- **P0-1 launcher 生产策略**：删除 `lease_placeholder` 与相关 hook；`Launcher.attach` 先 `VerifyIdentity` 再通过 `CoreClient.CreateLease/ReleaseLease` 走真实 HTTP lease；`defaultSpawnCore` 传 `--owner-mode=headless` flag；`cmd/awaken-acp` 默认从同级目录解析 `awaken-core[.exe]`；`defaultSpawnDesktop` 支持 `AWAKEN_DESKTOP_PATH` 与相邻 Desktop 可执行文件搜索。
- **P0-2 bridge 事件名与终端状态**：`session.go` 改用 `agui.Event*` 常量；文本从 `frame.Delta` 读取；工具状态使用 SDK `pending/in_progress/completed/failed`；新增 terminal accumulator，区分 `RUN_FINISHED`/`RUN_ERROR`/`RUN_CANCELLED`；SSE 使用独立 `SSEClient`（`Timeout=0`，5s 连接超时）。
- **P0-3 permission 回写**：`resolvePermission` 改为调用 `/v1/control/acp/sessions/{session_id}/runs/{run_id}/permissions/{interaction_id}/resolve` 控制路由，携带 `option_id`/`cancelled`；支持 `allow_once/allow_always/reject_once/reject_always`。
- **P0-4 run cancel 与并发**：新增 `/v1/control/acp/sessions/{session_id}/runs` 创建 run 后 `SetCurrentRun`，`Prompt` 在 defer 中 `ClearCurrentRun`；`ctx.Done()` 统一走 `cancelAndWait`；所有 `currentRunID` 读写受 `s.mu` 保护。
- **P0-5 单实例 ownership**：Desktop supervisor 启动前先读取 manifest，对存活的 headless Core 进入 `external-attached` 并创建 desktop lease；desktop owner 也提供 `LeaseManager`；bridge owner ID 改为随机 UUID；manifest 使用 `CompareAndSwapManifest` 与 `DeleteManifestIfOwned` 防止误覆盖/误删。
- **P1-1 lease 锁模式与 idle budget**：`LeaseCount`/`shouldIdleShutdown` 改为写锁；实现 30s idle budget，`activeRunCountFunc` 可阻塞 shutdown，run terminal 后重新倒计时。
- **P1-2 metadata**：使用 `awaken.ephemeral` meta；`InitializeRequest.ClientInfo` 的 name/version 保存到 bridge 并在 `NewSession` 透传；thread 快照新增 `source/client_name/client_version/acp_session_id` 并在 create-run 时写入；`CloseSession` 对 ephemeral thread 调用 `DeleteThread`。
- **P1-3 control token 覆盖**：bridge 的 run/event/cancel/permission/current-run 全部迁入 `/v1/control/acp/sessions/...`，由 `ControlTokenMiddleware` 统一保护。
- **P1-4 Windows userData 路径**：`DefaultUserDataDir` Windows 路径改为 `%APPDATA%`，与 Desktop `app.getPath("appData")` 一致。
- **P1-5 输入处理**：`extractTextPrompt` 拼接全部 text block；非空 `additionalDirectories`/`mcpServers` 返回 invalid params；`CanonicalizeCWD` 移除 `url.ParseRequestURI`。
- **P1-8 文档冲突**：更新 README、`docs/02_主智能体闭环/索引.md`、`ACP智能体协作设计与实施方案.md`、`docs/03_运行时与本地服务/Desktop本地服务生命周期.md`，将 Phase 5 未验收的能力明确标为已知缺口，不再超前声明为当前支持。

### 11.3 仍待后续验证的缺口

- 真实第三方 ACP client（Zed、Codex CLI 等）的端到端 prompt/permission/cancel 闭环。
- 超过 30 秒的长 turn SSE 稳定性与 cursor 重连（P1-6）。
- ACP live event 的独立 durable append 与崩溃后 replay（P1-7）。
- 多 bridge 并发、Desktop 接管 headless、Core crash 后的多进程竞争测试（P0-5 已有单元测试，但缺少真实多进程验收）。

## 12. 四次复核结论：三次复核修复与 Phase 5 完整性

### 12.1 Review 总结

- 评审基线：`81eec3ad` 及其后的本地未提交修改；重点复核 `src/acp/agentbridge`、ACP control routes、`ACPSessionManager`、lease/launcher、Desktop runtime supervisor 与 Phase 5 文档。
- 三次复核列出的 launcher、AG-UI event、permission 回写、cancel/close、control token、Windows userData 与 Desktop attach 等问题已完成实质修复。
- 本轮新发现 3 个高优先级生命周期问题和 5 个中优先级完整性问题；均已直接在当前工作区修复并增加聚焦回归测试。
- 当前结论：**未发现新的合并阻塞代码问题**。仍缺真实第三方 ACP client、packaged Desktop auto-start/attach 和多进程竞争的本次验收证据，因此不能把这些未执行项写成“已通过”。

### 12.2 本轮发现并修复的问题

#### P0-1 headless Core 被创建它的 bridge 无条件杀死

位置：`src/acp/agentbridge/launcher.go`

`--startup=core` 的 bridge 在退出时先释放 lease，随后直接 `Process.Kill`。如果期间 Desktop 或第二个 bridge 已 attach 并持有 lease，第一个 bridge 仍会杀死共享 Core，违反“同一 userData identity 单 Core + lease owner”契约。

修复：bridge 退出只释放自己的 lease。headless Core 是否退出由 Core 内的 lease manager、active run count 与 idle budget 唯一决定；只有 spawn 后尚未完成 attach 的失败路径才允许 launcher 回收自己刚创建的进程。

#### P0-2 Desktop 无法接管 headless Core

位置：`apps/desktop/src/main/runtime-supervisor.ts`

Desktop 原实现把规范化 userData 路径本身拼成 `packaged:<path>`，而 Go Core 使用规范化路径的 SHA-256 前 12 字节生成 identity。两端算法必然不一致，`external-attached` 的真实接管会失败并尝试 spawn 第二个 Core。

修复：Desktop 使用与 Go `normalizeProjectWorkspaceRoot + stableProjectID` 相同的平台路径规范化和 SHA-256 截断算法；`runtime-supervisor.test.ts` 改为按同一契约构造 manifest fixture。

#### P0-3 session/run 绑定存在覆盖与越界窗口

位置：`src/runtime/acp_session.go`、`src/server/control_acp_run.go`、`src/server/router.go`

原实现先启动 run、再写 `CurrentRunID`，且 `SetCurrentRun` 可以覆盖已有 run；公开的 set-current route 也允许 bridge 写入任意 run ID。并发 prompt 可能同时穿过 busy check，迟到的 clear 还能清掉后续 run。create-run 请求也可携带另一个 thread/workspace。

修复：

- Core 新增 `BeginRun/AbortRun` 原子 reservation，同一 session 在 StartRun 窗口内也只能有一个 prompt；
- `SetCurrentRun` 禁止覆盖不同 active run，`GetSession` 返回快照而不是暴露可变内部指针；
- 删除 bridge 可调用的 set-current route，由 create-run handler 成为唯一绑定 owner；
- clear-current 必须携带精确 run ID，旧 run 的迟到清理不能影响新 run；
- thread ID 与 canonical workspace 必须匹配 session mapping，跨 session/scope 请求返回 400；
- StartRun 后的 mapping/commit 失败会主动 cancel 已创建 run，不留下未跟踪运行。

#### P1-1 event stream 提前 EOF 被误判为成功

位置：`src/acp/agentbridge/session.go`、`src/acp/agentbridge/coreclient.go`

原实现从已关闭 error channel 读到零值 `nil` 后调用 `terminalResult`，而没有 terminal 时该函数默认返回 `end_turn`。网络断线因此可能伪装成成功。

修复：只有收到 `RUN_FINISHED` 或 `RUN_ERROR` 才设置 `terminalSeen`；frame/error channel 均关闭且没有 terminal 时返回 transport error。SSE reader 也会处理 EOF 前未以空行结束的最后一个 data frame。

#### P1-2 SSE cursor 没有重连与去重

位置：`src/acp/agentbridge/session.go`

修复：记录最后确认的 `streamCursor`（兼容回退 `seq`），瞬时断线最多重连 3 次并从 `after_cursor` 继续；重复 cursor 不再二次投影。该能力只覆盖同一 Core/run，Core 重启后的 session 恢复仍明确不支持。

#### P1-3 bridge 本地并发 prompt 仍有竞态

位置：`src/acp/agentbridge/session.go`

原 busy check 在 Core create-run 前释放互斥锁，两个 goroutine 都可能看到空 `currentRunID`。

修复：增加覆盖整个 Prompt 生命周期的 `promptActive` reservation；Core 侧的 `BeginRun` 再提供跨请求第二层保护。

#### P1-4 permission 请求未绑定 run 生命周期

位置：`src/acp/agentbridge/session.go`

原 `RequestPermission` 使用 session 级 context，run cancel 或 terminal 后请求仍可能挂起；pending option 又在 Core resolve 成功前删除，失败后无法安全重试。

修复：permission 请求改用 run context，run 结束立即取消；pending option 仅在 Core 成功接受 resolution 后删除。

#### P1-5 ephemeral session 在 active run terminal 前删除 thread

位置：`src/runtime/acp_session.go`

原 close 发出 cancel 后立即调用 `DeleteThread` 并忽略错误；active run 尚未 terminal 时删除通常失败，但 mapping 已移除，导致临时 thread 泄漏且无法重试。

修复：session 进入 closing 状态，拒绝新 run；对 active run 发出 cancel 并等待 terminal，再删除 ephemeral thread。任何等待/删除失败都会保留 mapping 并解除 closing，允许 client 重试 close。visible thread 始终保留。

#### P1-6 packaged Desktop 默认路径解析错误

位置：`src/acp/agentbridge/launcher.go`

`awaken-acp` 被打包在 `resources/bin/<platform-arch>`，旧 resolver 却只检查 `<binary-dir>/Awaken/...`，无法找到安装根目录的 Desktop executable。

修复：从 ACP binary 所在目录向上有限搜索 packaged layout；保留 `AWAKEN_DESKTOP_PATH` 显式覆盖，并新增 packaged resources 布局测试。

#### P1-7 cwd 与 meta 契约过宽

位置：`src/runtime/acp_session.go`、`src/acp/agentbridge/agent.go`

修复：存在目录在所有平台都必须成功完成 symlink canonicalization；`awaken.ephemeral` 只接受 JSON boolean；client name/version 只信任 initialize 的 `ClientInfo`，不接受 session meta 覆盖审计来源。

#### P1-8 多轮 prompt 仍提交首轮的预留 thread ID

位置：`src/acp/agentbridge/coreclient.go`、`src/acp/agentbridge/session.go`

首轮 run 会把 Core 中的预留 thread ID 更新为实际 thread ID，但 bridge-local mapping 仍保留旧值；第二轮若再次提交旧 thread ID，会被正确的 scope 校验拒绝。

修复：create-run control 请求收窄为只提交 input；thread/workspace 完全由 Core session mapping 决定，bridge 不再复制或覆盖 Core-owned scope。另将 ACP cancel 绑定到本地 prompt context，使 cancel 即使发生在 create-run HTTP 尚未返回的窗口内也能终止请求。

### 12.3 Phase 5 完整性判断

代码层已具备以下闭环：真实 lease 与 heartbeat、Desktop/core/fail 启动策略、同 identity 单 Core、Desktop attach headless、trusted client cwd、project/thread/run 映射、text/tool/permission 投影、精确 option 回写、cancel/close、ephemeral terminal 后清理、同 Core cursor 重连和来源 metadata。

仍需外部环境完成的验收，不属于本轮可在纯单元测试中证明的代码修复：

1. 用 ACP compatibility suite、Zed 与另一个真实 ACP client 验证 initialize/new/prompt/permission/cancel/close。
2. 用实际安装包验证 Desktop 未运行时 auto-start，以及 `--startup=core` 后 Desktop 接管同一 Core。
3. 执行真实多进程测试：两个 bridge 并发、bridge 强杀后 TTL 回收、Desktop lease 存在时首个 bridge 退出不影响 Core。
4. 验证超过 30 秒的真实长 turn、Core crash 后“不自动重放”的明确终态，以及 token 不进入日志/UI。
5. ACP live semantic event 的独立 durable append 仍是已知缺口；当前 live 与正常 terminal replay 已有实现，但 Core 在 permission/长 turn 中崩溃时，最后一批尚未 append 的 live fact 可能丢失。

### 12.4 本次验证

通过：

- `go test -count=1 -timeout 180s ./src/acp/...`：`src/acp`、`src/acp/agentbridge`、`src/acp/protocol` 通过
- `go test ./src/runtime -run 'Test(CanonicalizeCWD|StableCWDHash|ACPSession)' -count=1 -timeout 60s`
- `go test ./src/server -run 'TestControlACP' -count=1 -timeout 60s`
- `npm.cmd --workspace @awaken/desktop test -- runtime-supervisor`：13 passed，2 skipped
- `npm.cmd --workspace @awaken/desktop run typecheck`
- `go build ./cmd/awaken-acp`、`go build ./cmd/awaken-core`
- `git diff --check`：通过，仅有工作区 LF/CRLF 转换提示

新增回归覆盖：

- session run reservation、active run 不可覆盖、mapping snapshot 隔离、精确 run clear；
- control create-run 拒绝跨 session thread；
- bridge create-run 不提交 Core-owned thread/workspace，支持预留 thread 被替换后的多轮 prompt；
- premature SSE EOF 失败；
- 断线后从最后 cursor 重连；
- packaged resources 到 Desktop executable 的路径解析；
- Desktop attach identity digest 与 manifest fixture 一致。

尚未执行或无法声明通过：

- 真实 Desktop/安装包与真实第三方 ACP client 验收；
- `go test -race`（当前 Windows 环境未提供可用 race detector toolchain）；
- 全仓与既有环境相关的长耗时测试不作为本轮聚焦验证的“全绿”证据。

## 13. 五次复核结论：真实 ACP + Desktop 验收

### 13.1 Review 总结

- 复核日期：2026-07-23。
- 复核范围：当前未提交 Phase 5 修改、README 正式 Desktop 启动链路、实际 runtime manifest、`awaken-acp` stdio initialize/new/prompt，以及重启后的真实 Desktop 界面。
- 验收方式：重新构建 `awaken-core.exe` 与 `awaken-acp.exe`，通过 `npm run dev:desktop` 启动当前工作区 Desktop；再使用基于仓库 ACP SDK 的独立 client adapter，以子进程方式连接真实 `awaken-acp.exe --startup=fail`。该 adapter 不是 HTTP mock，也不绕过 stdio bridge。
- 当前结论：**Phase 5 暂不具备可交付的真实 prompt 闭环，存在 P0 阻塞。**

### 13.2 已通过的真实链路

1. README 正式入口能够启动当前工作区 Desktop、Core 与随机 loopback runtime。
2. Core 在当前 Desktop userData identity 下发布 manifest；验收未记录或复制 control token。
3. `awaken-acp --startup=fail` 能发现并 attach Desktop-owned Core，没有启动第二个 Core。
4. ACP `initialize` 成功，`session/new` 能按 client cwd 创建 session mapping。
5. Computer Use 复核确认重启后的 Awaken Desktop 正常显示唯一窗口、Awaken project 与已配置默认模型 `deepseek-v4-flash`。

以上只证明 discovery、认证、attach 和 session 建立可用，不证明 Agent prompt 可用。

### 13.3 P0：入站 ACP run 没有服务端模型配置来源

位置：

- `src/server/control_acp_run.go`
- `src/runtime/run_service.go`
- `apps/desktop/src/main/model-profile-store.ts`
- `apps/desktop/src/main/index.ts`

真实 stdio 调用结果：

```text
initialize      -> success
session/new     -> success
session/prompt  -> HTTP 400 MISSING_MODEL_PROFILE
```

根因：

1. profile-aware `RunService` 明确要求 `StartRunRequest.ModelProfile` 非空；
2. Renderer 发起的普通 Desktop run 由 Electron Main 调用 `ModelProfileStore.resolveDefaultRunProfile()`，再把完整运行配置注入 `/v1/runs`；
3. ACP control handler 直接构造 `StartRunRequest`，没有任何模型 profile provider；
4. `awaken-acp` 也不应让任意 ACP client 在 prompt 中提交 provider、endpoint 或 credential。

这不是“缺少第三方 client 环境”，而是所有真实 client 都会稳定触发的产品代码缺口。即使 Desktop 已经配置且 UI 正在显示默认模型，ACP prompt 仍失败。

### 13.4 本轮已直接修复的问题

#### P0-1 Desktop runtime identity 少了 `project_` 前缀

真实 manifest 的 identity 形如 `packaged:project_<digest>`。第四次修复虽然让 TypeScript 改用与 Go 相同的路径规范化与 SHA-256 截断，却返回了裸 digest，Desktop 仍无法匹配真实 Core。

修复：

- `RuntimeSupervisor.digestUserDataDir` 返回 `project_<digest>`；
- 测试改用 Go 契约产生的固定 golden value，不再用 TypeScript 被测实现生成自身期望值。

#### P1-1 create-run 错误丢失 Core 响应体

原 bridge 只返回 HTTP status，第一次真实验收只能看到 400，无法定位 canonical error。

修复：

- `CoreClient.CreateRunForSession` 在非 201 时读取最多 8 KiB 响应体并加入错误；
- 增加 `MISSING_MODEL_PROFILE` 响应体回归测试；
- 上限避免异常 Core 返回无界内容。

### 13.5 Phase 5 解阻方案

第五次复核提出的 Electron/safeStorage broker 方案建立在错误的凭据存储假设上，已由 §14 的实现取代。当前权威方案是：client 通过标准 ACP selector 选择 Awaken-owned profile ID；Core 从 canonical `models.json + .env` 只读解析完整配置；未选择时使用默认/首个 ready profile；client 永远不能提交或读取 API key、Base URL 与 provider 参数。

### 13.6 本次验证状态

通过：

- 当前工作区 Desktop 正式启动与真实 UI 检查；
- 当前二进制构建；
- Desktop-owned Core discovery、identity、attach；
- ACP stdio `initialize`、`session/new`；
- 错误响应体诊断准确返回 `MISSING_MODEL_PROFILE`。

失败：

- ACP stdio `session/prompt`：模型执行前失败，无 text/tool/permission/terminal 成功证据。

未执行：

- `--startup=desktop` 在 Desktop 完全未运行时的安装包 auto-start；
- `--startup=core` 的真实 prompt；
- Zed/compatibility suite、permission/cancel、多 bridge 与长 turn。

## 14. 六次复核结论：模型选择 P0 解阻

### 14.1 决策与风险判断

采用的合同：

1. ACP client 只能通过标准 `session/set_config_option` 选择 Awaken 返回的 opaque profile ID；
2. selector 只包含 enabled + ready profile 的 ID、provider、展示名、model ID 和默认标志；
3. client 不能提交 provider、Base URL、运行参数、capability 或 API key；
4. 未选择时使用 session 建立时的默认 ready profile；没有可用默认时使用配置顺序中的首个 ready profile；
5. active prompt 中禁止切换；显式选择随后被删除、禁用或失去 key 时返回 `ACP_MODEL_PROFILE_UNAVAILABLE`，不静默切换其他 provider。

可接受风险：

- 本地 client 能看到用户配置了哪些 provider/model，并可能选择成本不同的已启用 profile；这是模型 selector 的必要可见性，且范围不超过当前用户主动启用的配置。
- profile ID 是稳定的本地配置标识；它不是 secret，但不写入共享验收报告。
- `models.json` 与 `.env` 是两个文件，不是跨进程事务；Core 在每次 prompt 前重新解析并 fail closed，避免使用半更新或已失效配置。
- Go 增加只读 parser 后必须与 Desktop 的文件格式保持 parity；写权限仍只属于 Electron Main，Go 不归一化写回。

不可接受并已阻断：

- client 直接提交模型名并借此构造任意 provider/endpoint；
- client 注入或读取 API key；
- 选择 disabled、missing-key、未知或配置读取失败的 profile；
- 默认项失效后静默切换到另一个付费 provider。

### 14.2 实现

- `src/modelprofile/user_store.go`
  - 只读解析 `~/.awaken/config/models.json` 与 `~/.awaken/.env`；
  - 复用 profile ID、enabled/default、文件顺序、credential revision 与内置 capability；
  - `.env` 损坏时整份 fail closed，不跳过坏行；
  - 只向 catalog 返回 ready profile，完整 key 只进入 Core 内存的 `ModelProfileConfig`。
- `src/server/control_acp_model_profiles.go`
  - 新增 authenticated `GET /v1/control/acp/model-profiles`；
  - 响应不包含 Base URL、auth scheme、参数、key 尾号或 API key。
- `src/server/control_acp_run.go`
  - create-run 只接受额外的 `model_profile_id`；
  - Core 本地解析并设置 `StartRunRequest.ModelProfile`，未知/失效 ID 返回 typed error。
- `src/acp/agentbridge`
  - `session/new` 返回 category=`model` 的标准 ACP selector；
  - `session/set_config_option` 只接受已广告 profile ID；
  - session prompt 提交 profile ID，不提交完整配置或 key；
  - 修复真实验收发现的 ephemeral text delta 与 durable assistant projection 重复输出。
- `src/acp/agentbridge/launcher.go`
  - `--startup=core` 移除继承的 `AWAKEN_HOME`/userData 覆盖，固定注入当前用户 canonical `~/.awaken` 与目标 userData；
  - 不再需要第五次方案中的 Electron/safeStorage credential broker。当前 key 权威本来就是 `.env` 明文文件。

### 14.3 本次真实验收

环境：Windows、README `npm run dev:desktop`、当前工作区重建的 `awaken-core.exe` 与 `awaken-acp.exe`、独立 stdio ACP client adapter。

Desktop-owned Core 与 `--startup=core` 两条路径结果均为：

```text
initialize      -> success
session/new     -> success，返回 model selector
session/prompt  -> success
assistant       -> ACP_MODEL_SELECTION_E2E_OK
stop_reason     -> end_turn
```

第一次成功 prompt 暴露 assistant 文本重复一次；journal 中模型结果只有一份，原因是 bridge 同时投影 ephemeral chunk 与相同 message ID 的 durable 完整消息。本轮已按 message ID 累积并压掉重复的 durable projection，第二次真实 stdio prompt 返回单一文本。

`--startup=core` 验收期间未启动 Electron/Desktop，prompt 后未留下 `awaken-acp`、`awaken-core` 或 Electron 进程。该证据证明 core-only 默认模型 prompt；尚未覆盖长 run lease、第二 bridge 和随后 Desktop attach。

### 14.4 自动化

通过：

- `go test ./src/modelprofile -count=1`
- `go test ./src/acp/agentbridge -count=1 -timeout 120s`
- `go test ./src/server -run 'TestControlACP' -count=1 -timeout 120s`
- `go test ./cmd/awaken-core -count=1`
- `npm run build:binary`

新增覆盖：

- 默认/显式 profile 解析、missing-key fallback、未知选择、损坏 `.env` fail-closed；
- 非敏感 catalog 不包含 API key/Base URL/auth scheme；
- ACP model selector 与 active session 选择；
- create-run 只提交 profile ID，不提交 key；
- headless 环境覆盖清理；
- live/durable assistant 文本去重。

未完成：

- `--startup=core` 的长 run lease、第二 bridge、TTL/idle budget 与随后 Desktop attach；
- compatibility suite、Zed 与另一个实际 ACP client；
- permission、cancel、多 bridge、长 turn 与 Core crash 的真实进程级验收。
