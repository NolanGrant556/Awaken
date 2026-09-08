# Awaken 功能与调用面

> 当前代码基线复核日期：2026-08-11

## 目录

- [1. 权威入口](#1-权威入口)
- [2. 模型请求与管理请求](#2-模型请求与管理请求)
- [3. 按需功能面](#3-按需功能面)
- [4. 受影响时核对的辅助调用面](#4-受影响时核对的辅助调用面)
- [5. 当前三协议适配边界](#5-当前三协议适配边界)
- [6. 修改检查表](#6-修改检查表)

## 1. 权威入口

- 供应商、profile、协议和模型选择：`docs/05_模型与供应商配置/模型配置与调用.md`
- 上下文窗口与压缩：`docs/02_主智能体闭环/上下文窗口与压缩.md`
- 提示词缓存、用量与成本：`docs/02_主智能体闭环/提示词缓存与成本治理.md`
- Subagent：`docs/02_主智能体闭环/原生Subagent产品与架构契约.md`
- Read 与模型媒体：`docs/04_工具权限与审批/Typed文件工具与可恢复删除.md`
- 定时任务：`docs/12_定时任务/定时任务设计契约.md`
- 长期记忆：`docs/11_长期记忆/长期记忆与Dream设计契约.md`

先以这些权威确定产品行为，再用代码和本次测试证明“当前支持”。Skill 资料不取代正式功能域文档。

## 2. 模型请求与管理请求

### 2.1 会调用 LLM 推理的请求

`src/llm/types.go` 当前定义以下 `RequestPurpose`：

| purpose | 场景 | 模型来源 | 供应商关注点 |
| --- | --- | --- | --- |
| `agent_turn` | 主会话 Agent 循环 | thread 固定 profile | tools、多模态、缓存、effort、上下文、usage |
| `thread_title` | 左侧栏会话标题 | 默认继承 thread；可有 host route | 辅助请求禁用/收窄 reasoning、独立缓存作用域 |
| `compact_summary` | 上下文压缩摘要 | 默认继承 thread；可有 host route | 8192 输出上限、禁用/收窄 reasoning、独立缓存作用域 |
| `native_compaction` | exact OpenAI Responses 原生压缩 | 固定使用当前 thread profile，不走用途 route | official endpoint/capability、opaque output、独立 usage、失败回落本地 V2 |
| `subagent` | 原生 Subagent | 默认继承 parent；可有 host route | 子线程上下文、工具能力、独立 usage 与缓存 identity |
| `scheduled_task` | 定时任务运行 | 任务显式 pin；否则用途 route/默认 profile | 无前台选择器、失效 pin fail closed、套餐可用性 |
| `memory_recall` | Memory recall/选择 | 辅助配置 | 结构化输出、低成本、与主调用隔离 |
| `memory_maintenance` | Dream/记忆维护 | 辅助配置 | 严格 JSON、独立任务/usage、失败恢复 |
| `connection_test` | 配置页连接测试 | 待保存 profile | 只证明最小请求可达，不证明全部 capability |

主调用、标题、压缩、Subagent 和定时任务不是同一个缓存作用域。`prompt_cache_key` identity 会纳入 purpose、profile、凭据 revision、模型、reasoning policy 和 endpoint contract。

### 2.2 不属于 LLM 推理的管理 API

以下请求不得记成一次模型生成：

- 模型目录同步：`/models` 或供应商原生 Models API；
- 余额查询：例如 DeepSeek `/user/balance`、Kimi balance；
- 组织用量与成本查询：OpenAI/Anthropic 管理 API；
- 套餐席位、额度和账单查询；
- API Key、账户和 endpoint 的控制台操作。

它们使用不同 endpoint、权限和错误语义，管理密钥与推理密钥必须分开。

余额、用量和套餐查询的统一分层、各 provider ID 当前接入状态与候选管理接口见 [用量查询合同](用量查询合同.md)。该合同区分推理响应 usage、同 Key 账户查询、组织 Admin API、云账号 IAM/AKSK 与仅控制台五类来源。

## 3. 按需功能面

| 功能点 | Awaken 当前行为 | 供应商资料必须回答 | 主要 owner |
| --- | --- | --- | --- |
| 1. 左侧栏会话标题 | `thread_title` 辅助模型请求；默认继承 thread，可由 Main 注入用途 route | 是否允许关闭 thinking；最小输出字段；usage/缓存如何记 | `src/runtime/thread_title.go`、`src/runtime/run_service.go`、`src/llm/request_policy.go` |
| 2. 上下文大小调节 | profile 保存 exact model 的窗口、输出预留和用户可降低的有效窗口；UI 不得凭供应商名猜窗口 | 模型总窗口、最大输出、协议差异、API 是否可查询 | `src/modelprofile/registry.go`、`apps/desktop/src/main/model-capabilities.ts` |
| 3. 上下文压缩 | proactive/reactive compaction；摘要走 `compact_summary`；成功边界持久化后才换 epoch | 上下文过长错误、summary 输出上限、缓存失效、服务端 compaction 是否实际使用 | `src/agentcore/context_*` |
| 4. Subagent 调用 | child 默认继承 parent profile；用途 route 可覆盖；child usage 单独记录 | tools、并发/限流、上下文、套餐是否允许 agentic 使用 | `src/agentcore/spawner.go`、`src/agentcore/runner.go` |
| 5. 定时任务 | 任务持有明确 profile；失效显式 pin fail closed；也有 `scheduled_task` route | 无 UI 在线时的凭据、套餐有效期、限流、模型下线 | `src/scheduledtask/executor.go` |
| 6. 多模态 Read | 图片进入模型媒体；PDF 走原生整份或 Poppler 页面回退；Office/Notebook 不作为模型媒体 | exact model + exact protocol 是否支持图片/原生 PDF/视频；大小和格式限制 | `src/tools/read`、`src/tool/model_content.go`、protocol adapter |
| 7. token/提示词缓存 | 按 exact provider/base/protocol 合同注入字段并解析 cached usage；没有合同不注入私有字段 | 自动/显式/`prompt_cache_key` 路由亲和、最小前缀、TTL、usage 字段、失效条件 | `src/llm/prompt_cache_capability.go` |
| 8. 额度查询 | DeepSeek、Kimi API、Kimi Coding Plan、MiniMax Token Plan 可用对应 Key 查询；OpenAI/Anthropic 另存组织管理密钥；Google、Qwen、豆包/火山管理查询需要独立云账号授权，UI 提供说明和官方入口；其余候选路径见统一用量合同 | 区分推理 Key、套餐 Key、组织 Admin Key、云账号 IAM/AKSK 与仅控制台；记录稳定性、区域、权限和指标 | `apps/desktop/src/main/model-balance-fetchers.ts`、`src/runtime/execution_journal.go` |
| 9. 模型选择 | profile identity 固定 provider/base/model/protocol；默认只影响新 thread；旧 thread 保持选择 | 精确模型 ID、展示名、目录 API、协议支持、区域和下线策略 | `apps/desktop/src/main/model-profile-store.ts`、`src/modelprofile/registry.go` |

## 4. 受影响时核对的辅助调用面

- **主会话调用**：这是最大调用面，决定工具、多模态、reasoning、缓存和上下文能否工作。
- **连接测试**：应尽量低成本、禁用 reasoning；不得把成功响应提升成完整能力证明。
- **Memory recall**：与主会话共享供应商可能性，但必须有独立 purpose、预算和缓存 identity。
- **Dream/Memory maintenance**：后台模型请求，需要独立耐久 usage 和恢复语义。
- **重试**：同一次逻辑调用的 retry index 必须留在账本中；不得因重试重复制造不可恢复副作用。
- **模型目录同步**：影响可选择模型，不是模型推理；失败时区分实时 API、版本化清单和手填。
- **能力探测**：只在 exact identity 作用域缓存，不把一个 Key/Base URL 的结果传播给另一 profile。
- **usage/cost 归一化**：Chat、Responses、Messages 返回字段不同，cached、uncached、cache-write、reasoning 不能互相重复计数。

## 5. 当前三协议适配边界

| Awaken protocol | adapter | 常见 endpoint | 当前关键策略 |
| --- | --- | --- | --- |
| `openai_chat` | `src/llm/openai_chat.go` | `/chat/completions` | messages 历史重放、tools、SSE、provider 扩展字段白名单 |
| `openai_responses` | `src/llm/openai_responses.go` | `/responses` | 当前 `store:false`，Awaken 自己重放历史；不要假设 `previous_response_id` |
| `anthropic_messages` | `src/llm/anthropic_messages.go` | `/v1/messages` 或兼容路径 | content blocks、thinking/signature、cache_control、Messages usage |

未知非空协议 fail closed。空/`auto` 仅是旧 Chat 默认兼容，不得用于新 provider 自动猜测。

## 6. 修改检查表

### Provider route

- provider ID、access type、region、Base URL、protocol、auth scheme；
- API/套餐 key 是否分开；
- endpoint 后缀是否由 adapter 拼接且只拼一次；
- 模型目录与推理 endpoint 是否不同。

### Model capability

- exact model ID 和展示名；
- context/output limits；
- image/native PDF/video/tools；
- thinking toggle 与 effort 档位；
- 协议级能力收窄。

### Request/response

- 非流式和 SSE；
- tool call 增量和终态；
- reasoning text/signature 重放；
- usage 与 cached token 归一化；
- context-too-long、rate limit、auth、model-not-found 错误。

### Usage management

- 先区分 per-request usage、余额、成本、套餐窗口和云账号账单；
- 记录查询使用的凭据类别、权限、区域、endpoint、鉴权和返回指标；
- 401/403 表示凭据类型或权限问题，429 表示管理接口受限，均不能显示成“余额为 0”；
- 当前未接入的官方管理 API 只能写成候选能力，不能展示为 Awaken 当前支持。

### Product acceptance

- 新 thread 与旧 thread 模型选择；
- 标题、压缩、Subagent、定时任务；
- 图片与 PDF Read；
- 上下文面板和上次请求缓存/成本；
- 重启后 profile、凭据引用、usage 和 transcript 恢复。
