---
name: llm-provider-management
description: 新增、修改或排查 Awaken 模型供应商接入、协议能力与用量查询时使用。
---

# Awaken LLM 供应商管理

把供应商官方合同、Awaken 当前代码和实测结果分层核对，维护可追踪、可验证、不会跨协议猜测的模型接入。

## 先读什么

1. 遵守根 `AGENTS.md` 的任务范围与工作区保护；已读取的项目背景不重复加载。
2. 修改 provider、profile、adapter、缓存、推理或用量前，先读 [Awaken 功能与调用面](references/Awaken功能与调用面.md)。
3. 涉及协议或能力判定时，再读 [协议与能力判定](references/协议与能力判定.md)。
4. 涉及余额、组织成本、套餐额度或用量查询时，必须再读 [用量查询合同](references/用量查询合同.md)。
5. 只加载本次涉及的供应商文件，不一次性读完所有供应商资料。

## 工作流

### 1. 确定事实层

把每条结论标成以下一种，不能混写：

- **Awaken 当前支持**：当前代码、正式文档、测试或 Desktop 行为可证明。
- **供应商官方支持**：当前官方文档明确写出，但 Awaken 可能尚未接入。
- **当前不支持或缺口**：官方没有合同，或 Awaken adapter/能力表尚未落地。
- **待实测**：只有兼容性猜测、旧记忆、社区文章或连接测试，不能晋升为支持。

### 2. 固定接入身份

在判断能力前固定以下 identity：

```text
provider_id + access_type + region + base_url + protocol
+ auth_scheme + credential_scope + model_id/endpoint_id
```

API、Coding Plan、Token Plan、Agent Plan 使用不同凭据、网关和计费合同。即使厂商相同，也不得互换或合并推断。

### 3. 按改动选择核对范围

仅完整接入新供应商或用户要求全面审计时使用以下全矩阵；局部任务只选择涉及的协议、能力与实际调用面，不因同属 LLM 功能而检查全部供应商或调用链：

- Chat Completions、Responses、Messages 三协议；
- endpoint、Header、Key 类型和区域；
- 模型目录、精确模型 ID、上下文窗口、最大输出；
- 图片、原生 PDF、视频、工具调用与流式事件；
- 提示词缓存模式、请求字段、usage 字段、缓存失效条件；
- thinking 开启/关闭与 reasoning effort，二者分开；
- 余额、组织用量、成本、速率限制、套餐额度；
- Awaken 的标题、压缩、Subagent、定时任务、Read、Memory、Dream、连接测试等调用面。

用量查询还必须先判定凭据类别：推理 Key、套餐 Key、组织 Admin Key、云账号 OAuth/IAM/AKSK，或仅控制台。不得因为同属一家厂商就复用凭据或拼接管理接口。

同一厂商同时提供普通 API 与 Coding/Token/Agent Plan 时，先按用户配置入口的 `provider_id + access_type + credential_scope` 选择查询合同，再使用该入口保存的凭据。以 MiniMax 为例：`minimax` 的普通按量 API Key 只能进入请求 usage/账户余额合同，`minimax-token-plan` 的订阅 Key 只能进入 5 小时与周额度合同；不能根据模型名、推理 Base URL、请求成功与否或 UI 选中状态互相回退，也不能把 401/403 解释成零额度。

### 4. 修改正确 owner

- 供应商入口、协议、Base URL、鉴权、目录策略：`apps/desktop/src/main/model-profile-store.ts`。
- 模型能力声明：`src/modelprofile/registry.go` 与 `apps/desktop/src/main/model-capabilities.ts`。
- Awaken effort 到供应商原生值的映射：`src/llm/request_policy.go`；三协议 wire lowering：`src/llm/openai_chat.go`、`src/llm/openai_responses.go`、`src/llm/anthropic_messages.go`。
- 缓存、推理开关、请求策略和 usage：`src/llm/` 对应 capability/adapter 文件。
- 产品事实：`docs/05_模型与供应商配置/模型配置与调用.md`；缓存与成本事实：`docs/02_主智能体闭环/提示词缓存与成本治理.md`。

不要把 vendor route、协议 adapter、模型 capability 和套餐计费写进同一个不可拆分判断。

### 5. 验证后交付

运行最接近改动的测试，按代码影响选择 typecheck/Go test；Skill 资料变更执行对应结构校验与 `git diff --check`。涉及真实产品路径时按正式入口做 Desktop 验收。连接测试只证明当前 endpoint/key/model 能响应，不证明图片、PDF、工具、缓存、effort 或套餐规则。

## 硬约束

- 不从 Base URL、SDK 名称或“OpenAI compatible”推断三协议全支持。
- 不把 Responses 的 `previous_response_id`、服务端存储或原生 compaction 冒充 Awaken 当前行为；先检查 adapter 是否使用。
- 不把 thinking 开关当 reasoning effort，也不从其中一个推导另一个。
- 不把供应商原生多模态能力当作当前协议能力；`Read` 必须经过 exact profile capability。
- 不把 cached token 缺失解释为 0，不把套餐 token 折算成按量美元成本。
- 不把推理响应中的 token usage、账户余额、组织成本和套餐剩余额度混成同一指标。
- 不让同厂商的普通 API、Coding Plan、Token Plan 或 Agent Plan 互相试探用量端点；历史 profile 无法确定凭据类型时要求用户重新选择入口，不通过遍历 endpoint 猜测。
- 不用推理 Key 试探组织/IAM 管理接口，也不用高权限管理凭据执行模型推理；管理凭据必须单独存储、最小授权且不得进入 Renderer 或日志。
- 官方只提供控制台、IAM action 名或一方插件实现时，必须标明稳定性；没有公开请求合同就不能写成可直接接入的稳定 API。
- 不把价格、额度或模型列表长期抄死；保留官方链接和复核日期，易变数字必须注明日期。
- 不用社区文章替代官方合同。官方未确认时写“未确认”，并保留测试证据和差异。
- 修改供应商资料时，从仓库根运行 `python3 .codex/skills/llm-provider-management/scripts/check_provider_docs.py`。

## 供应商路由

- [OpenAI](references/providers/openai/供应商资料.md)
- [Anthropic](references/providers/anthropic/供应商资料.md)
- [Google Gemini](references/providers/google-gemini/供应商资料.md)
- [Kimi / Moonshot](references/providers/kimi/供应商资料.md)
- [MiniMax](references/providers/minimax/供应商资料.md)
- [GLM / 智谱](references/providers/glm/供应商资料.md)
- [Qwen / 阿里云百炼](references/providers/qwen/供应商资料.md)
- [火山引擎 / Doubao Ark](references/providers/volcengine-doubao/供应商资料.md)
- [DeepSeek](references/providers/deepseek/供应商资料.md)
- [自定义 API / 兼容网关](references/providers/custom/供应商资料.md)

## 更新供应商资料

1. 只打开本次改动涉及的供应商官方页面；全面更新时才覆盖协议、模型、缓存、推理、计费／套餐、用量／余额与限流。
2. 更新供应商文件顶部的“官方资料复核日期”。
3. 对用量查询记录凭据类别、endpoint、鉴权、区域、返回指标、权限、稳定性和 Awaken 当前接入状态；若只有控制台也要明确写出。
4. 同时核对当前 `BUILTIN_MODEL_PROVIDERS`、能力表、adapter 和测试，不只改文档链接。
5. 把官方新增能力写成“供应商官方支持”；只有 Awaken 落地并验证后才改成“Awaken 当前支持”。
6. 记录官方与当前实现的差异；若差异影响用户行为，同任务更新对应正式权威文档。
7. 运行资料校验脚本和改动相关测试。

## 验证命令

从 Awaken 仓库根目录运行：

```bash
python3 .codex/skills/llm-provider-management/scripts/check_provider_docs.py
python3 "${CODEX_HOME:-$HOME/.codex}/skills/skill-creator/scripts/quick_validate.py" .codex/skills/llm-provider-management
git diff --check
```

第一条校验供应商目录、当前 builtin provider ID、顶层 reference 与 `agents/openai.yaml`；第二条校验 Skill frontmatter、命名和基础结构。代码或正式文档发生变化时，还要运行对应 Go/TypeScript 测试，不能用 Skill 结构校验代替产品验证。
