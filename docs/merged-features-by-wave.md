# 已合并功能清单

当前本地只剩 `main`，远端 `origin/main` 与本地 `main` 一致。下面清单根据 `main` 的 merge history 整理，所有列出的 `trae/*` 功能分支都已经合并进 `main`。

## Wave 1

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `session-lite-reader` | `src/sessionlite` | 提供轻量会话读取能力，用于快速获取会话基础信息而不加载完整运行上下文。 |
| `session-state-reporter` | `src/sessionstate` | 汇总会话当前状态，便于 UI、诊断或恢复流程判断会话是否运行中、完成或异常。 |
| `session-tag-index` | `src/sessiontags` | 建立会话标签索引，让会话可以按标签检索、筛选和归类。 |
| `session-title-generator` | `src/titlegen` | 根据会话内容生成标题，提升历史会话列表的可读性。 |
| `session-insights-index` | `src/sessioninsights` | 聚合会话洞察信息，帮助统计会话摘要、关键事件和使用特征。 |
| `context-files-lister` | `src/contextfiles` | 列出当前上下文中涉及的文件，方便用户和系统理解当前工作范围。 |
| `context-usage-visualizer` | `src/contextviz` | 可视化上下文使用情况，展示 token/上下文占用与组成。 |
| `conversation-branching` | `src/sessionbranch` | 支持从现有会话创建分支，便于尝试不同方向而保留原始对话。 |

## Wave 2

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `transcript-entry-parser` | `src/transcriptjsonl` | 解析 transcript JSONL 条目，为恢复、搜索和分析提供结构化输入。 |
| `transcript-pairing-repair` | `src/transcriptrepair` | 修复 transcript 中消息、工具调用和工具结果之间的配对关系。 |
| `transcript-search-index` | `src/transcriptindex` | 为会话 transcript 建立搜索索引，支持快速查找历史内容。 |
| `conversation-recovery-filter` | `src/convrecovery` | 恢复会话时过滤无效或不适合重放的事件，降低恢复失败风险。 |
| `session-clear-core` | `src/sessionclear` | 提供清空会话数据的核心能力。 |
| `session-exporter` | `src/sessionexport` | 导出会话内容，便于备份、迁移或分享。 |
| `session-share-bundle-core` | `src/sessionshare` | 将会话整理成可分享包，包含必要元数据和上下文。 |
| `worktree-session-state` | `src/worktreestate` | 记录 worktree 与会话的关联状态，帮助区分不同工作区里的运行记录。 |

## Wave 3

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `busy-input-mode-policy` | `src/busyinput` | 定义运行忙碌时如何处理新输入，避免用户操作和后台任务互相干扰。 |
| `background-notification-policy` | `src/backgroundnotify` | 控制后台任务通知策略，决定何时提醒用户。 |
| `restart-drain-timeout` | `src/draintimeout` | 在重启前等待运行中任务收尾，并提供超时控制。 |
| `rate-limit-options-core` | `src/ratelimitoptions` | 提供限流相关配置选项，为模型请求和运行时策略提供基础。 |
| `model-name-sanitizer` | `src/modeldisplay` | 清理并规范模型显示名，避免 UI 或日志展示混乱。 |
| `provider-error-classifier` | `src/providererrors` | 分类模型供应商错误，区分限流、鉴权、临时故障等类型。 |
| `image-routing-policy` | `src/imagerouting` | 定义图片请求的路由策略，用于决定请求交给哪个能力或模型处理。 |
| `powershell-safety-policy` | `src/powershellpolicy` | 针对 PowerShell 命令做安全策略判断，降低危险命令误执行风险。 |

## Wave 4

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `async-hook-registry` | `src/asynchooks` | 注册和管理异步 hook，支持后台事件触发和异步处理。 |
| `hook-json-schema` | `src/hookio` | 校验 hook 输出 JSON，保证 hook 事件能被运行时可靠消费。 |
| `hook-matcher-router` | `src/hookrouter` | 根据事件和条件匹配 hook，并将事件路由到对应处理器。 |
| `hook-progress-events` | `src/hookprogress` | 标准化 hook 进度事件，让 UI 或日志能展示 hook 执行状态。 |
| `hook-timeout-policy` | `src/hooktimeout` | 规划 hook 超时策略，避免 hook 长时间阻塞运行流程。 |
| `cleanup-registry-core` | `src/cleanupregistry` | 管理清理任务注册，确保运行结束或异常时能释放资源。 |
| `command-lifecycle-state` | `src/commandstate` | 追踪命令生命周期，包括排队、运行、完成、失败等状态。 |
| `delegation-progress-events` | `src/delegationevents` | 规范委派任务的进度事件，便于父任务追踪子任务执行情况。 |

## Wave 5

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `shell-hook-runner` | `src/shellhooks` | 执行 shell hook，并强化 hook 执行边界。 |
| `shell-output-parser` | `src/taskoutput` | 缓冲并解析任务输出，为终端展示和后续处理提供稳定输入。 |
| `shell-wrapper-normalizer` | `src/shellwrap` | 规范 shell 包装命令，减少不同 shell 或平台之间的命令差异。 |
| `terminal-output-classifier` | `src/terminaltext` | 分类终端输出，区分普通文本、错误、提示和状态信息。 |
| `terminal-output-transformer` | `src/terminaltransform` | 提供终端输出转换链，用于清理、改写或结构化输出。 |
| `terminal-setup-planner` | `src/terminalsetup` | 规划终端初始化步骤，帮助构建可预期的执行环境。 |
| `structured-io-framing` | `src/structuredio` | 对结构化输入输出进行分帧，保证流式传输中边界清晰。 |
| `stream-delta-accumulator` | `src/streamaccumulator` | 累积流式 delta，并保留非文本事件载荷，生成完整响应状态。 |

## Wave 6

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `tool-policy-resolver-core` | `src/toolpolicy` | 解析工具使用策略，支持按工具或 MCP server 级别匹配权限规则。 |
| `tool-input-safety-classifier` | `src/toolsafety` | 对工具输入进行安全分类，识别潜在危险操作。 |
| `tool-preview-policy` | `src/toolpreview` | 控制工具调用预览策略，决定哪些调用需要先展示给用户确认。 |
| `tool-prompt-ordering` | `src/toolorder` | 管理工具相关提示的排列顺序，保证模型看到稳定的工具说明。 |
| `tool-result-storage` | `src/toolresults`, `src/agentcore` | 提供工具结果持久化句柄，并保持持久化为显式 opt-in。 |
| `tool-use-summary` | `src/toolsummary` | 生成工具使用摘要，便于会话总结、审计或 UI 展示。 |
| `tool-output-persistence-plan` | `src/outputpersist` | 规划工具输出持久化方式，决定哪些结果应保存以及如何引用。 |
| `schema-sanitizer-core` | `src/schemasanitize` | 清理和规范 schema，提升工具 schema 的兼容性与安全性。 |

## Wave 7

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `permission-policy` | `src/agentcore`, `src/runtime`, `src/tool`, `src/tools` | 添加运行级权限策略，统一控制工具和运行时权限。 |
| `permission-product-core` | `src/permissionproduct` | 提供权限请求产品层核心，包括待审批队列和状态清理。 |
| `permission-explanation-engine` | `src/permissionexplain` | 为权限请求生成解释，帮助用户理解为什么需要授权。 |
| `permission-prompt-stats` | `src/permissionstats` | 统计权限弹窗和审批行为，为体验优化提供数据。 |
| `permission-rule-shadowing` | `src/permissionrules` | 检测权限规则遮蔽，避免后写规则意外覆盖或失效。 |
| `policy-limits-core` | `src/policylimits` | 定义策略限制模型，用于统一表达权限、资源或安全边界。 |
| `sandbox-policy-plan` | `src/sandboxplan` | 规划沙箱策略，决定命令或工具应以何种隔离级别运行。 |
| `url-safety-policy` | `src/urlsafety` | 校验 URL 安全性，限制不安全协议、地址或跳转。 |

## Wave 8

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `provider-registry-core` | `src/providerregistry` | 建立模型供应商注册表，保存供应商能力和启用状态。 |
| `provider-model-management` | `apps/desktop`, `cmd/awaken-core`, `src/llm`, `src/server` | 管理模型 profile，并修复 profile key 与运行时信息解析。 |
| `provider-routing-preferences` | `src/providerrouting` | 管理用户的供应商路由偏好，并与供应商可用性分离。 |
| `provider-fallback-state` | `src/providerfallback` | 记录供应商 fallback 状态，支持主供应商不可用时切换。 |
| `llm-retry-backoff-errors` | `src/llm` | 定义 LLM 重试错误分类，并对临时传输故障做退避重试。 |
| `rate-limit-quota-tracker` | `src/ratelimit` | 跟踪限流和配额使用情况。 |
| `usage-limits-budget` | `src/agentcore`, `src/llm`, `src/usage` | 建立用量预算账本，用于控制 token、请求或成本预算。 |
| `effort-level-policy` | `src/effort` | 管理 effort 等级策略，影响模型推理强度或请求参数。 |

## Wave 9

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `mcp-runtime-client` | `src/mcpclient` | 添加 MCP 运行时客户端基础能力。 |
| `mcp-management-core` | `src/mcpmanagement` | 提供 MCP 管理能力，包括配置导入、状态隔离和敏感配置脱敏。 |
| `mcp-resource-content` | `src/mcpresources` | 处理 MCP resource 内容读取和结构化表示。 |
| `mcp-server-approval-core` | `src/mcpapproval` | 管理 MCP server 审批流程。 |
| `mcp-sampling-handler` | `src/mcpsampling` | 支持 MCP sampling 请求处理。 |
| `mcp-elicitation-handler` | `src/mcpelicitation` | 支持 MCP elicitation 请求、队列和相关 hook。 |
| `mcp-oauth-pkce` | `src/mcpoauth` | 添加 MCP OAuth PKCE 流程，并要求 OAuth endpoint 使用安全 URL。 |
| `mcp-skills-core` | `cmd/awaken-core`, `src/agentcore`, `src/mcp`, `src/server`, `src/skills`, `src/tool` | 建立 MCP 与 skills 能力注册表，使运行时能发现和暴露能力。 |

## Wave 10

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `gateway-delivery-router` | `src/gateway` | 添加网关投递路由，用于把消息或事件送到正确目标。 |
| `gateway-session-metadata` | `src/gatewaymeta` | 管理网关会话元数据。 |
| `remote-control-router-core` | `src/remotecontrol` | 添加远程控制路由核心，处理远程命令或控制事件。 |
| `remote-sdk-message-adapter` | `src/remotesdk` | 将远程 SDK 消息适配为内部可处理格式。 |
| `remote-session-ingress` | `src/sessioningress` | 提供远程会话入口客户端和存储。 |
| `remote-sse-cursor` | `src/remotesse` | 管理远程 SSE 游标，支持断点续传或事件恢复。 |
| `remote-user-pairing-core` | `src/remoteauth` | 支持远程用户配对和认证状态。 |
| `transport-batch-queue-core` | `src/transportbatch` | 提供传输批处理队列，提升远程事件投递效率。 |

## Wave 11

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `workspace-adddir-validation` | `src/workspaceadddir` | 校验添加到工作区的目录，避免无效或不安全路径。 |
| `workspace-trust-state` | `src/truststate` | 记录工作区信任状态，决定是否允许敏感能力。 |
| `worktree-isolation-policy` | `src/runtime`, `src/server`, `src/workspaceguard` | 添加 workspace guard 策略，约束 worktree 间访问边界。 |
| `file-history-state` | `src/filehistory` | 记录文件历史状态，支持变更追踪和恢复判断。 |
| `file-index-fuzzy-search` | `src/fileindex` | 建立模糊文件索引，支持按部分名称快速查找文件。 |
| `file-state-concurrency-guard` | `src/filestate` | 防止并发读写文件时发生状态冲突。 |
| `context-reference-resolver` | `src/contextrefs` | 解析上下文引用，并去重重复引用。 |
| `subdirectory-hint-tracker` | `src/subdirhints` | 追踪用户或系统给出的子目录提示，帮助限定工作范围。 |

## Wave 12

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `context-compaction-engine` | `src/compactengine` | 添加上下文压缩引擎，将长会话压缩为可继续使用的摘要。 |
| `context-resume-persistence` | `cmd/awaken-core`, `src/agentcore`, `src/context`, `src/llm`, `src/runtime`, `src/server` | 持久化 transcript 和压缩上下文，并强化事件恢复。 |
| `memory-foundation` | `cmd/awaken-core`, `src/memory`, `src/server` | 建立长期记忆基础能力，并加上开关和安全边界。 |
| `memory-extraction-pipeline` | `src/memoryextract` | 从会话中提取可保存记忆，并过滤敏感或不适合保存的信息。 |
| `session-memory-core` | `src/sessionmemory` | 管理会话级记忆，把记忆与具体会话关联。 |
| `system-prompt-section-cache` | `src/promptsections` | 缓存系统提示的分段内容，提高提示组装效率。 |
| `prompt-cache-metadata` | `src/agentcore`, `src/llm` | 记录 prompt cache 元数据，辅助缓存命中和调试。 |
| `thinkback-replay-core` | `src/thinkback` | 支持回放历史思考或执行过程，用于审计、恢复或体验展示。 |

## Wave 13

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `agent-definition-loader` | `README.md`, `src/agentdefs`, `src/server` | 发现并加载 agent 定义，提供对外查询接口。 |
| `agent-profile-runprofile` | `README.md`, `apps/desktop`, `src/agentcore`, `src/runtime`, `src/server` | 支持每次运行选择 agent profile，并处理默认权限模式。 |
| `agent-task-runtime-core` | `src/agenttask` | 添加 agent task 运行时，管理任务 ID、状态和执行数据。 |
| `agent-interrupt-controller` | `src/interrupt` | 提供 agent 运行中断控制。 |
| `subagent-run-delegation` | `apps/desktop`, `src/runtime`, `src/server` | 支持把任务委派给子 agent，并保留父子运行元数据。 |
| `assistant-core` | `src/assistantcore` | 添加 assistant 领域核心模型。 |
| `task-center-background` | `apps/desktop`, `src/runtime`, `src/server` | 添加桌面任务中心面板，用于展示后台任务。 |
| `progress-mode-resolver` | `src/progressmode` | 解析进度展示模式，决定何时显示详细进度或简化状态。 |

## Wave 14

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `github-actions-setup-core` | `src/githubactions` | 提供 GitHub Actions 设置核心，辅助创建或检查 CI 配置。 |
| `github-pr-flow` | `src/prflow` | 支持 GitHub PR 创建、更新或流程管理。 |
| `github-pr-review-comments` | `src/prreview` | 处理 GitHub PR review 评论能力。 |
| `diff-approval-review` | `src/agentcore`, `src/protocol`, `src/runtime`, `src/tool`, `src/tools` | 添加结构化 diff review 元数据，支持更清晰的变更审批。 |
| `release-notes-core` | `src/releasenotes` | 生成或组织 release notes。 |
| `feedback-bundle-core` | `src/feedbackbundle` | 打包反馈数据，便于问题上报和诊断。 |
| `vcr-event-recorder` | `src/vcrrecorder` | 录制事件流，用于复现、测试或调试。 |
| `verifier-skill-init` | `src/verifierinit` | 初始化 verifier skill，推荐或生成验证步骤。 |

## Wave 15

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `output-secret-redaction` | `src/agentcore`, `src/llm`, `src/redaction`, `src/runtime`, `src/server` | 对输出中的密钥和敏感值做脱敏。 |
| `privacy-settings-core` | `src/privacysettings` | 管理隐私设置。 |
| `security-audit-log` | `src/agentcore`, `src/runtime`, `src/server` | 记录安全审计事件，便于追踪敏感操作。 |
| `trusted-device-token-state` | `src/trusteddevice` | 管理可信设备 token 状态。 |
| `oauth-token-cache-state` | `src/oauthstate` | 管理 OAuth token 缓存状态。 |
| `credential-pool-rotation` | `src/credentialpool` | 支持凭据池轮换，降低单个凭据长期使用风险。 |
| `env-credential-passthrough` | `src/envguard` | 管控环境变量凭据透传，并识别凭据文件。 |
| `security-review-orchestrator` | `src/securityreview` | 编排安全审查流程，组织多项安全检查结果。 |

## Wave 16

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `config-source-layer` | `src/configlayers` | 建立配置来源分层，明确默认值、用户配置和覆盖项优先级。 |
| `feature-flag-overrides` | `src/featureflags` | 计算 feature flag 覆盖结果。 |
| `settings-migration-sync` | `src/settingssync` | 支持设置迁移和同步。 |
| `runtime-catalog-core` | `src/runtimecatalog` | 管理 runtime catalog，描述可用运行时能力。 |
| `runtime-footer-metadata` | `src/runtimefooter` | 生成 runtime footer 元数据，用于 UI 或响应尾部展示。 |
| `status-line-runtime` | `src/statusline` | 提供状态栏运行时数据。 |
| `doctor-diagnostics` | `src/doctor` | 添加 doctor 诊断基础，用于检查环境和配置问题。 |
| `telemetry-diagnostics` | `src/telemetry` | 添加 telemetry 诊断基础，用于排查埋点或遥测状态。 |

## Wave 17

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `text-tool-call-parser` | `src/texttoolparser` | 从文本中解析工具调用，并强化 fallback 行为。 |
| `tool-orchestration-runtime` | `src/agentcore` | 添加工具编排运行时，协调工具调用、结果和状态。 |
| `structured-clarify-tool` | `src/clarify` | 提供结构化澄清请求能力，让模型能用明确选项向用户追问。 |
| `todo-planning-tool` | `README.md`, `src/agentcore`, `src/tool`, `src/tools` | 添加 todo 规划工具，用于维护任务清单和进度。 |
| `slash-command-library` | `apps/desktop`, `src/runtime`, `src/server`, `src/slashcommands` | 支持 slash command 运行时，并允许命令引用绝对路径 prompt。 |
| `web-fetch-tool` | `src/agentcore`, `src/tools` | 添加安全网页抓取工具。 |
| `browser-control-runtime` | `src/agentcore`, `src/tool`, `src/tools` | 添加浏览器控制工具运行时，并对页面摘要和不可用工具做权限/可见性保护。 |
| `pty-process-runtime` | `src/processes`, `src/tools` | 添加进程控制运行时，支持 PTY/进程类工具。 |

## Wave 18

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `inbound-attachment-normalizer` | `src/attachments` | 对输入附件做归一化处理，统一附件元数据和内容表示。 |
| `prompt-suggestion-engine` | `src/promptsuggest` | 生成 prompt 建议，帮助用户更快构造请求。 |
| `magic-docs-core` | `src/magicdocs` | 提供 magic docs 核心能力，用于文档生成或增强。 |
| `notebook-edit-plan` | `src/notebookedit` | 生成 notebook 编辑计划，支持结构化修改 notebook。 |
| `keybinding-resolver-core` | `src/keybindingcore` | 解析快捷键绑定，处理不同命令和上下文下的按键映射。 |
| `deep-link-action-parser` | `src/deeplink` | 解析 deep link 动作，支持从链接触发应用内操作。 |
| `voice-input-session-core` | `src/voiceinput` | 管理语音输入会话状态。 |
| `project-onboarding-context` | `src/agentcore`, `src/onboarding`, `src/prompt` | 为新项目生成 onboarding 上下文提示，帮助模型理解项目。 |

## Wave 19

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `ndjson-safe-stringify` | `src/ndjsonsafe` | 安全序列化 NDJSON，避免特殊字符破坏行协议。 |
| `env-snapshot-manager` | `src/envsnapshot` | 管理环境快照，用于记录运行时环境变量和上下文差异。 |
| `read-state-write-guard` | `src/readstate` | 防止基于过期读取状态进行写入，降低覆盖用户变更的风险。 |
| `attribution-snapshot-tracker` | `src/attributionstate` | 追踪 attribution 快照，记录内容来源或生成归因。 |
| `cost-usage-tracker` | `src/costtracker` | 追踪成本和用量，为预算、报表和限额提供数据。 |
| `output-style-runtime` | `src/outputstyle` | 管理输出风格运行时，决定响应的格式和语气配置。 |
| `think-block-filter` | `src/thinkingfilter` | 过滤 reasoning/thinking block，避免内部推理块出现在不该出现的输出中。 |
| `thinking-token-budget` | `src/thinkingbudget` | 管理 thinking token 预算策略。 |

## Wave 20

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `message-queue-manager` | `src/commandqueue` | 管理命令/消息队列，控制排队、出队和状态。 |
| `sdk-event-queue` | `src/sdkevents` | 提供 SDK 事件队列基础。 |
| `capacity-wake-core` | `src/capacitywake` | 管理容量唤醒逻辑，在资源可用时恢复或触发任务。 |
| `stream-flush-gate` | `src/streamgate` | 控制流式输出 flush 时机，避免过早或过频繁刷新。 |
| `activity-timeout-notifications` | `src/activitynotify` | 对活动超时生成通知。 |
| `away-summary-prompt-core` | `src/awaysummary` | 生成离开期间摘要提示，用于用户回来后快速了解进展。 |
| `contextual-nudge-core` | `src/contextnudges` | 根据上下文生成提醒或轻量建议。 |
| `midrun-steering-notes` | `src/steering` | 记录运行中 steering notes，帮助后续步骤遵循用户调整。 |
| `autodream-consolidation-core` | `src/autodream` | 整合 autodream 相关内容或状态，为后续生成/规划提供基础。 |

## Wave 21

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `fast-mode-routing-core` | `src/fastmode` | 定义 fast mode 路由策略，让轻量请求走更快路径。 |
| `eval-acceptance-harness` | `src/evalharness` | 添加 eval 验收框架，用于运行和判断验收测试。 |
| `lsp-diagnostic-feedback` | `src/lspdiagnostics` | 收集 LSP 诊断反馈，帮助模型理解代码错误或警告。 |
| `lifecycle-hooks-runtime` | `README.md`, `cmd/awaken-core`, `src/agentcore`, `src/lifecyclehooks`, `src/runtime` | 添加生命周期 hook 运行时，并避免 after hook 阻塞运行完成。 |
| `scheduled-runs` | `README.md`, `src/runtime`, `src/server` | 添加定时运行 API，支持计划任务。 |

## Wave 22

| 功能分支 | 主要模块 | 功能与作用 |
|---|---|---|
| `bridge-poll-config` | `src/bridgepoll` | 管理 bridge 轮询配置。 |
| `bridge-token-refresh` | `src/bridgetoken` | 添加 bridge token 刷新基础能力。 |
| `plugin-extension-surface` | `src/pluginruntime` | 提供插件扩展运行时 surface。 |
| `plugin-trust-policy` | `src/plugintrust` | 检查插件信任策略，判断插件是否可安全启用。 |
| `team-mode-core` | `src/teamcore` | 添加团队模式核心能力。 |
| `team-memory-secret-sync-core` | `src/teammemorysync` | 同步团队记忆和密钥相关状态。 |
| `side-question-fork-core` | `src/sidequestion` | 支持旁路问题分支，让问题可以在不打断主线的情况下探索。 |
| `ultraplan-planner-core` | `src/ultraplan` | 添加 ultraplan 规划器核心，用于更复杂的计划生成。 |
| `webui-bridge-core` | `src/webuibridge` | 添加 WebUI bridge 核心，并强化客户端授权校验。 |
| `checkpoint-rollback` | `src/agentcore`, `src/checkpoints`, `src/tools` | 为 apply patch 增加 checkpoint，支持变更前保存和回滚。 |

