# PRD：Awaken 固定工作角色与智能体定义机制

## 1. 文档目的

这份 PRD 面向 0 上下文开发者。开发者只需要阅读 Awaken 当前仓库和本文档，就能理解并实现“Agent 注册表与固定工作角色”这一项能力。

本 PRD 覆盖 Nolan 负责的第一项能力：在 Awaken 中建立内置、固定、可观测、可约束的工作角色体系，并让 runtime、server、desktop 可以围绕这些角色组织 prompt、工具、权限、连接器、记忆、运行模式和用户可见状态。

本文档不要求开发者访问任何外部源码或历史上下文。

## 2. 背景

Awaken 面向 work 场景：资料调研、计划制定、外部系统执行、结果复核、团队协作。

一个真实工作任务通常不会只有“执行”一步。它至少包含：

1. 查清楚事实。
2. 拆解目标和风险。
3. 得到用户确认。
4. 调用工具执行。
5. 回读和复核结果。

如果所有阶段都由同一个通用 agent 处理，会出现几个问题：

- 调研阶段可能误调用写入工具。
- 计划阶段可能跳过确认直接执行。
- 执行阶段可能不清楚哪些动作已经批准。
- 复核阶段可能继续扩展任务，而不是检查结果。
- UI 无法解释当前 agent 为什么能做某些事、不能做某些事。

因此 Awaken 需要先定义“谁来做事”：也就是一套固定工作角色和对应的 AgentDefinition 注册表。

## 3. 产品结论

Awaken v1 不做任意自定义 agent 编辑器。v1 先提供 4 个固定工作角色：

- `Researcher`：调研员，负责搜集信息和整理事实。
- `Planner`：计划员，负责拆解目标、识别依赖、提出方案。
- `Operator`：执行员，负责执行已确认的动作。
- `Reviewer`：复核员，负责检查结果、发现遗漏、输出复核报告。

用户不需要理解底层 agent 机制。Desktop 可以用“任务模板 / 工作阶段 / 角色卡片”的方式表达这些能力：

- 查资料
- 做计划
- 执行已确认动作
- 复核结果

内部实现必须使用稳定的 AgentDefinition。也就是说，UI 文案可以是中文任务模板，但 runtime 里必须能明确知道当前 run 使用的是哪一个固定角色。

## 4. P0 范围

P0 要完成的是“固定工作角色能力”的第一版，不只是技术底座。

P0 必须包含：

- 内置 AgentDefinition 注册表。
- 4 个固定工作角色。
- 完整 AgentDefinition 字段契约。
- AgentConfig / StartRunRequest 能指定角色。
- RequestBuilder 能按角色追加 prompt、过滤 tools。
- 角色能声明所需连接器、允许 Skill、记忆范围、默认运行模式。
- Runtime / Server / Desktop 能识别当前角色。
- Desktop 至少能通过任务模板或角色入口启动固定角色任务。
- UI 能展示不同角色说明、能力边界和缺失依赖。
- Reviewer 角色可用，并默认只读。
- 测试覆盖注册表、工具过滤、角色透传、UI 显示和真实 LLM smoke。

## 5. 非目标

P0 不做：

- 任意自定义角色编辑器。
- 插件角色加载。
- 用户项目文件覆盖内置角色。
- 多 worker 团队协作。
- 远程执行。
- 工作区隔离。
- 自动后台化。
- 连接器市场。
- Skill Library 管理 UI。
- 代码开发专用能力，例如分支、提交、PR、代码 diff 审批。

P0 可以为这些能力预留字段，但不能把它们作为必须实现项。

## 6. 用户体验原则

### 6.1 用户看到的是工作任务，不是底层 agent

不要把 UI 做成“选择一堆 agent”。用户应该看到的是工作动作：

- 查清楚这件事
- 帮我做个计划
- 按计划执行
- 帮我复核结果

这些入口内部映射到固定角色。

### 6.2 可以有角色卡片，但不能变成技术控制台

Desktop 可以有角色卡片或任务模板，但卡片文案要是 work 语言：

- 调研资料
- 制定计划
- 执行动作
- 复核结果

卡片可以展示：

- 这个角色会做什么。
- 这个角色不会做什么。
- 需要哪些连接器或 Skill。
- 是否可能触发审批。

### 6.3 默认可以自动选择角色

用户直接输入自然语言任务时，Awaken 可以默认进入主工作流，由系统自动选择或串联角色。

但 P0 至少要支持显式指定角色启动 run，方便任务模板、测试和后续 WorkRun 编排。

## 7. 核心概念

### 7.1 工作角色

工作角色是产品层概念，用来解释“当前由谁负责这一步”。

固定工作角色：

- Researcher
- Planner
- Operator
- Reviewer

### 7.2 AgentDefinition

AgentDefinition 是 runtime 层概念，用来约束一个角色的行为。

它控制：

- 角色身份。
- 使用时机。
- system prompt。
- 可见工具。
- 可用 Skill。
- 所需连接器。
- 记忆范围。
- 默认运行模式。
- 最大轮次。
- 权限和审批规则。

### 7.3 WorkRun

WorkRun 是后续完整工作流概念。它可以串联多个角色：

```text
Researcher -> Planner -> Approval -> Operator -> Reviewer
```

P0 不要求完成完整 WorkRun 编排，但 AgentDefinition 必须能支撑这条链路。

### 7.4 工具过滤

工具过滤指在构造 LLM request 前，根据当前角色决定模型能看到哪些工具。

工具过滤不是唯一安全边界。真实执行仍必须经过：

- tool 自身 permission check。
- workspace boundary。
- runtime approval。
- 审计事件。

### 7.5 连接器依赖

连接器依赖指某个角色完成任务所需的外部能力，例如知识库、日历、表格、IM、邮箱、CRM。

P0 只需要能声明和展示依赖状态，不要求实现完整连接器系统。

## 8. AgentDefinition 字段契约

新增包建议：

`src/agentdefs`

建议文件：

- `src/agentdefs/definition.go`
- `src/agentdefs/registry.go`
- `src/agentdefs/registry_test.go`

建议 Go 类型：

```go
package agentdefs

type AgentRole string

const (
	RoleResearcher AgentRole = "researcher"
	RolePlanner    AgentRole = "planner"
	RoleOperator   AgentRole = "operator"
	RoleReviewer   AgentRole = "reviewer"
)

type RunMode string

const (
	RunModeForeground RunMode = "foreground"
	RunModeBackground RunMode = "background"
)

type MemoryScope string

const (
	MemoryScopeUser    MemoryScope = "user"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeThread  MemoryScope = "thread"
	MemoryScopeRun     MemoryScope = "run"
)

type AgentDefinition struct {
	ID                 string
	Role               AgentRole
	DisplayName        string
	ShortDescription   string
	WhenToUse          string
	SystemPrompt       string
	AllowedTools       []string
	DisallowedTools    []string
	AllowedSkills      []string
	RequiredConnectors []string
	MemoryScopes       []MemoryScope
	DefaultRunMode     RunMode
	MaxTurns           int
	ReadOnly           bool
	CanWriteExternal   bool
	RequiresApproval   bool
}
```

字段说明：

- `ID`：稳定机器 ID，例如 `work-researcher`。
- `Role`：固定角色枚举。
- `DisplayName`：中文展示名。
- `ShortDescription`：UI 卡片短说明。
- `WhenToUse`：用于自动路由和开发者理解。
- `SystemPrompt`：追加到基础 system prompt 后的角色约束。
- `AllowedTools`：非空时，只允许这些工具进入模型请求。
- `DisallowedTools`：从可见工具中排除。
- `AllowedSkills`：该角色可加载的 Skill ID。P0 可先定义字段，不做完整 Skill runtime。
- `RequiredConnectors`：该角色需要的连接器能力 ID。P0 可先定义字段和 UI 展示。
- `MemoryScopes`：角色可以读取的记忆范围。
- `DefaultRunMode`：默认前台或后台。
- `MaxTurns`：最大 agentic turn 数。
- `ReadOnly`：角色是否只读。
- `CanWriteExternal`：是否允许外部系统写入。
- `RequiresApproval`：是否默认会触发审批。

## 9. 注册表需求

### FR-1：内置注册表

注册表必须提供：

```go
func Get(id string) (AgentDefinition, bool)
func MustGet(id string) AgentDefinition
func List() []AgentDefinition
func Default() AgentDefinition
```

要求：

- `List()` 顺序稳定。
- 未知 ID 不 panic。
- 内置角色不可在运行时被普通项目配置覆盖。
- P0 不从磁盘加载自定义角色。
- P0 不接插件角色。

### FR-2：active list

即使 P0 只有内置角色，也要在概念上区分：

- `allDefinitions`：全部已知定义。
- `activeDefinitions`：当前环境可用定义。

P0 中 active 判断至少考虑：

- 固定角色是否存在。
- 所需连接器是否已配置或可用。
- 所需 Skill 是否已安装或可用。

连接器或 Skill 不可用时，不一定要隐藏角色，但 UI 必须展示能力受限原因。

### FR-3：失败降级

如果后续扩展到可配置角色，坏定义不能拖垮内置角色。

P0 先要求：

- 内置角色构造失败时测试必须失败。
- 未知角色 ID 在 runtime 层返回明确错误，或按文档定义回退默认角色。

推荐 P0 策略：

- API 显式传未知角色 ID：返回 400。
- 内部未传角色 ID：使用默认角色。

## 10. 固定角色定义

### 10.1 Researcher：调研员

ID：

`work-researcher`

展示名：

`调研员`

角色目标：

搜集信息、读取连接器资源、整理事实、标注未知项。

何时使用：

- 用户问“现在是什么情况”。
- 用户需要汇总资料。
- 用户需要从多个来源找证据。
- Planner 需要先补充上下文。

输入：

- 用户问题。
- 当前 workspace / thread context。
- 可用连接器资源。
- 可用 Skill 指南。

输出：

- 已确认事实。
- 来源或证据摘要。
- 未确认问题。
- 下一步建议。

行为约束：

- 只读。
- 不创建、修改、删除文件。
- 不写入外部系统。
- 不发送消息。
- 不替用户做最终决策。
- 对不确定内容必须标注为“待确认”。

工具边界：

建议允许：

- `file_read`
- `list_files`
- `grep_search`
- `web_search`
- `calculator`

未来允许：

- 知识库查询。
- 邮件只读搜索。
- 日历只读查询。
- 表格只读查询。

Skill 边界：

- 可加载调研、摘要、检索类 Skill。
- 不加载写入类 Skill。

连接器依赖：

- 可选：知识库、邮箱、表格、日历、网页搜索。
- 缺失时仍可运行，但要说明信息来源受限。

记忆范围：

- `user`
- `project`
- `thread`

默认运行模式：

`foreground`

最大轮次：

建议 `4`

审批：

默认不触发审批。

失败处理：

- 连接器不可用：输出缺失来源。
- 资料不足：输出澄清问题。
- 工具失败：保留已成功来源并说明失败项。

### 10.2 Planner：计划员

ID：

`work-planner`

展示名：

`计划员`

角色目标：

把用户目标拆成可执行计划，识别依赖、风险、审批点和验收方式。

何时使用：

- 用户说“帮我规划一下”。
- 执行前需要先确认步骤。
- 任务涉及多个外部系统。
- 任务可能有风险或需要审批。

输入：

- 用户目标。
- Researcher 输出。
- 当前资源和连接器状态。
- 可用 Skill。

输出：

- 目标理解。
- 执行步骤。
- 每一步依赖。
- 每一步风险。
- 需要审批的动作。
- 验收方式。
- 可交给 Operator 的执行计划。

行为约束：

- 只读。
- 不执行计划。
- 不调用写入类工具。
- 不声称任务已经完成。
- 如果目标模糊，先提出澄清问题。

工具边界：

建议允许：

- `file_read`
- `list_files`
- `grep_search`
- `web_search`
- `calculator`

未来允许：

- 连接器状态查询。
- Skill 元数据查询。
- 任务模板查询。

Skill 边界：

- 可加载计划、项目管理、风险识别类 Skill。
- 不加载直接执行写入类 Skill。

连接器依赖：

- 可选：日历、任务系统、表格、知识库。
- 缺失时计划中必须标记“无法自动执行，需要人工处理”。

记忆范围：

- `user`
- `project`
- `thread`

默认运行模式：

`foreground`

最大轮次：

建议 `5`

审批：

Planner 自身不执行写动作，但它输出的计划应该标记审批点。

失败处理：

- 信息不足：进入澄清状态。
- 缺连接器：计划降级为人工步骤。
- 风险过高：要求用户确认后再进入执行。

### 10.3 Operator：执行员

ID：

`work-operator`

展示名：

`执行员`

角色目标：

按已确认的计划执行动作，例如发消息、改表格、创建任务、更新业务系统。

何时使用：

- 用户明确要求执行。
- Planner 计划已确认。
- WorkRun 已进入执行阶段。
- 用户指定“按这个方案做”。

输入：

- 已确认计划。
- 用户审批结果。
- 可用连接器和工具。
- 当前 run 状态。

输出：

- 已执行动作。
- 每个动作的结果。
- 失败或跳过的动作。
- 需要用户补充确认的问题。
- 可供 Reviewer 复核的结果摘要。

行为约束：

- 只能执行已确认或低风险动作。
- 高风险动作必须走审批。
- 不绕过权限、连接器授权或 workspace 边界。
- 不执行计划外写入。
- 不伪造外部系统结果。

工具边界：

允许：

- 已注册的执行类工具。
- 已授权连接器写入工具。
- 只读查询工具。

禁止：

- 未授权外部写入。
- 绕过审批的系统命令。
- 与当前任务无关的破坏性操作。

Skill 边界：

- 可加载执行 SOP 类 Skill。
- 只能调用当前任务允许的 Skill。

连接器依赖：

- 常见依赖：IM、表格、任务系统、日历、邮箱、CRM。
- 缺失必需连接器时，不应假装完成，应返回阻塞状态。

记忆范围：

- `project`
- `thread`
- `run`

默认运行模式：

`foreground`

后续可在长任务中支持 `background`，但 P0 默认前台。

最大轮次：

建议 `8`

审批：

默认需要审批。具体规则：

- 外部写入：需要审批。
- 删除、覆盖、群发、高影响动作：高风险审批。
- 只读查询：不需要审批。
- 已在同一计划中被用户批准的低风险动作：可按策略复用审批，但必须记录来源。

失败处理：

- 审批拒绝：停止对应动作并记录原因。
- 连接器授权失败：进入 `blocked`，提示需要授权。
- 外部系统失败：记录错误、回读状态、建议重试或人工处理。

### 10.4 Reviewer：复核员

ID：

`work-reviewer`

展示名：

`复核员`

角色目标：

检查执行结果是否符合用户目标，发现遗漏、失败、风险和不一致。

何时使用：

- Operator 执行完成后。
- 用户要求检查结果。
- 任务涉及外部写入。
- 任务结果需要交付或汇报。

输入：

- 原始用户目标。
- Planner 计划。
- Operator 执行日志。
- 审批记录。
- 外部系统回读结果。

输出：

- 复核结论。
- 已完成项。
- 未完成项。
- 风险和异常。
- 建议补救动作。
- 是否可向用户报告完成。

行为约束：

- 默认只读。
- 不执行外部写入。
- 不替 Operator 补做动作。
- 不把“未验证”说成“已完成”。
- 必须区分“已回读确认”和“仅根据日志推断”。

工具边界：

建议允许：

- run 结果读取。
- 审批记录读取。
- 任务状态读取。
- 外部系统只读回读。
- `file_read`
- `list_files`
- `grep_search`

禁止：

- 外部写入。
- 修改任务状态。
- 删除或覆盖结果。

Skill 边界：

- 可加载验收、复核、质量检查类 Skill。
- 不加载写入类 Skill。

连接器依赖：

- 可选：与 Operator 执行相同的连接器，但只读权限即可。
- 缺少回读能力时，复核结果必须标记为 `partial`。

记忆范围：

- `thread`
- `run`
- `project`

默认运行模式：

`foreground`

最大轮次：

建议 `4`

审批：

默认不需要审批，因为默认只读。

失败处理：

- 无法回读外部系统：输出部分复核。
- 发现不一致：输出失败项和补救建议。
- 发现审批缺失：标记为合规风险。

复核输出格式建议：

```text
复核结论：通过 / 未通过 / 部分通过

已确认完成：
- ...

未完成或异常：
- ...

证据：
- ...

建议补救：
- ...
```

## 11. 角色选择与路由

### FR-4：显式角色启动

Server API 和 Desktop bridge 应支持在启动 run 时传入角色 ID。

建议字段：

```go
AgentDefinitionID string `json:"agent_definition_id,omitempty"`
```

或命名为：

```go
RoleID string `json:"role_id,omitempty"`
```

推荐使用 `AgentDefinitionID`，因为它直接对应 runtime 定义。

### FR-5：默认角色

不传角色时，系统应使用默认角色。

推荐默认：

`work-planner`

原因：work 任务应该优先理解目标和计划，而不是直接执行。

如果用户输入明显是查询类任务，后续可由自动路由转到 Researcher；P0 可先不做复杂自动路由。

### FR-6：未知角色处理

推荐策略：

- API 显式传未知角色：返回 400，错误中包含可用角色列表。
- 内部未传角色：使用默认角色。
- 历史 thread 中角色已不存在：回退默认角色，并记录诊断事件。

### FR-7：角色卡片 / 任务模板

Desktop P0 应提供最小入口，让用户或测试能启动 4 个固定角色。

入口形式可以二选一：

方案 A：任务模板按钮

- 查资料
- 做计划
- 执行已确认动作
- 复核结果

方案 B：角色卡片

- 调研员
- 计划员
- 执行员
- 复核员

推荐方案 A，因为更贴近用户工作语言。

每个入口至少展示：

- 名称。
- 一句话说明。
- 是否只读。
- 是否可能触发审批。
- 缺失连接器或 Skill 的提示。

## 12. Prompt 组装

### FR-8：角色 prompt 追加

基础 system prompt 必须保持稳定。角色 prompt 只能追加，不应替换基础 prompt。

推荐顺序：

```text
基础系统提示

## 当前工作角色
角色名称、职责、限制

## 当前可用能力
工具、Skill、连接器摘要

## 当前运行状态
run/thread/approval/task 状态
```

### FR-9：角色 prompt 内容

每个角色 prompt 必须包含：

- 当前角色目标。
- 明确禁止事项。
- 输出格式要求。
- 权限和审批规则。
- 遇到缺失依赖时如何处理。

### FR-10：上下文顺序

后续 WorkRun 应按固定顺序组装上下文：

1. 基础系统提示。
2. Work 场景边界。
3. 当前工作角色。
4. 用户任务。
5. 项目上下文。
6. 连接器状态。
7. Skill 状态。
8. 记忆上下文。
9. 当前 run 状态。
10. 审批和任务状态。

P0 至少实现 1、3、4、9 的最小路径。

## 13. 工具、Skill、连接器约束

### FR-11：工具过滤

RequestBuilder 必须按当前角色过滤工具。

规则：

```text
all registered tools
-> if AllowedTools not empty, keep only AllowedTools
-> remove DisallowedTools
-> remove tools blocked by connector/permission status
-> build LLM tool definitions
```

要求：

- 过滤结果顺序稳定。
- 不存在的工具名不导致 panic。
- tool summary 必须反映过滤后的工具。
- 只读角色不得看到写入类工具。

### FR-12：执行前二次校验

工具过滤只能减少模型可见能力，不能替代执行安全。

ToolRuntime 执行前仍必须检查：

- workspace boundary。
- tool permission。
- connector authorization。
- approval requirement。
- run cancellation。

### FR-13：Skill 过滤

P0 可先只定义 `AllowedSkills` 字段并透传到 runtime info。

如果当前仓库还没有 Skill runtime，则 P0 不需要真实加载 Skill。

但 UI 和测试应能看到：

- 当前角色允许哪些 Skill。
- 缺失 Skill 时能力受限。

### FR-14：连接器依赖

P0 可先使用字符串 ID 表达连接器依赖：

```text
knowledge_base
mail
calendar
table
im
task_system
crm
```

要求：

- 注册表能声明 `RequiredConnectors`。
- Runtime info 或 Server API 能返回连接器状态占位。
- Desktop 能展示缺失依赖。
- 缺失连接器时，角色仍可启动或被阻止，必须有一致策略。

推荐策略：

- Researcher / Planner / Reviewer：缺失连接器可启动，但显示能力受限。
- Operator：缺失必需写入连接器时阻止执行，并提示授权或配置。

## 14. 权限与审批

### FR-15：只读角色最小权限

Researcher、Planner、Reviewer 默认只读。

要求：

- 不暴露写入类工具。
- 不调用外部写入。
- 不生成“已执行”口吻。
- 如果模型请求写入类工具，runtime 必须拒绝并记录异常。

### FR-16：Operator 审批

Operator 是唯一默认可执行外部写入的固定角色。

要求：

- 外部写入必须走 approval。
- 高风险写入必须使用 warning/danger 风险级别。
- 审批结果必须回到 run loop。
- 审批拒绝后不得继续执行该动作。

### FR-17：后台与无交互安全

P0 不要求实现后台任务。

但如果某角色后续以后台模式运行，权限策略必须 fail-closed：

- 有规则或 hook 明确允许才执行。
- 无法弹审批 UI 时，不得默认放行。
- 后台结果必须结构化回传主 run。

## 15. Runtime / Server / Desktop 改动

### FR-18：AgentConfig

修改：

`src/agentcore/types.go`

新增：

```go
AgentDefinitionID string
```

要求：

- 空值走默认角色或兼容路径。
- 不破坏现有字段语义。

### FR-19：RequestBuilder

修改：

`src/agentcore/request_builder.go`

职责：

- 读取 AgentDefinition。
- 追加角色 prompt。
- 过滤 tools。
- 更新过滤后的 tool summary。
- 对未知 ID 按策略返回错误或回退。

### FR-20：RunService

修改：

`src/runtime/run_service.go`

`StartRunRequest` 增加角色字段。

Run snapshot 建议增加：

```go
AgentDefinitionID string `json:"agent_definition_id,omitempty"`
AgentRole         string `json:"agent_role,omitempty"`
```

要求：

- 创建 run 时保存角色。
- 事件流中可展示当前角色。
- 旧请求不传字段仍能运行。

### FR-21：Server handlers

修改：

- `src/server/router.go`
- `src/server/handlers.go`

要求：

- `/v1/runs` 接收角色字段。
- 可选新增 `/v1/agent-definitions` 返回固定角色列表。
- 未知角色返回明确错误。

### FR-22：Desktop shared runtime

修改：

- `apps/desktop/src/shared/runtime.ts`

要求：

- `StartRunRequest` 支持角色字段。
- `RuntimeRun` 支持角色字段。
- 类型转换保持兼容。

### FR-23：Desktop UI

修改范围由实现者结合现有 App 结构决定，候选：

- `apps/desktop/src/renderer/App.tsx`
- `apps/desktop/src/renderer/workbenchPresentation.ts`
- `apps/desktop/src/renderer/runtime/*`

P0 UI 要求：

- 能通过任务模板启动 4 个固定角色之一。
- 显示当前角色或当前工作阶段。
- 显示角色说明和能力边界。
- 显示缺失连接器或 Skill 的提示。
- 不使用代码开发术语包装这些角色。

## 16. 事件与可观测性

### FR-24：角色启动事件

run 开始时应产生可观测事件，包含：

- run ID。
- thread ID。
- agent definition ID。
- role。
- display name。
- default run mode。

### FR-25：能力受限事件

当角色因为缺失连接器、Skill 或权限而受限，应记录事件：

- missing connector。
- missing skill。
- denied tool。
- approval required。

### FR-26：复核事件

Reviewer 输出应作为可回放事件或 artifact：

- 复核结论。
- 已完成项。
- 未完成项。
- 证据。
- 补救建议。

## 17. 数据与接口草案

### AgentDefinition JSON

```json
{
  "id": "work-researcher",
  "role": "researcher",
  "display_name": "调研员",
  "short_description": "读取资料、整理事实、标注未知项",
  "when_to_use": "需要先查清楚事实或汇总资料时使用",
  "allowed_tools": ["file_read", "list_files", "grep_search", "web_search", "calculator"],
  "disallowed_tools": [],
  "allowed_skills": ["research", "summarize"],
  "required_connectors": ["knowledge_base"],
  "memory_scopes": ["user", "project", "thread"],
  "default_run_mode": "foreground",
  "max_turns": 4,
  "read_only": true,
  "can_write_external": false,
  "requires_approval": false
}
```

### StartRunRequest JSON

```json
{
  "input": "帮我查一下这个客户最近的跟进情况",
  "agent_definition_id": "work-researcher",
  "workspace": {
    "workspace_root": "/Users/nolangrant/Awaken",
    "writable_roots": []
  },
  "client": {
    "session_id": "...",
    "request_id": "...",
    "entrypoint": "desktop"
  }
}
```

## 18. 测试需求

### TR-1：注册表测试

覆盖：

- `List()` 包含 4 个固定角色。
- 顺序稳定。
- `Get()` 能查到每个角色。
- 未知 ID 返回 false。
- 每个角色字段完整。
- 每个角色 `SystemPrompt` 非空。
- 每个角色 `DisplayName` 非空。

### TR-2：角色字段测试

覆盖：

- Researcher / Planner / Reviewer 为只读。
- Operator 可写但默认需要审批。
- 4 个角色都有 `DefaultRunMode`。
- 4 个角色都有 `MaxTurns`。
- 4 个角色都有 memory scopes。
- Operator 的 `CanWriteExternal` 为 true。
- Reviewer 的 `CanWriteExternal` 为 false。

### TR-3：工具过滤测试

覆盖：

- Researcher 不暴露写入工具。
- Planner 不暴露写入工具。
- Reviewer 不暴露写入工具。
- Operator 可以看到执行类工具。
- 不存在的 allowlist 工具不会 panic。
- tool summary 与过滤结果一致。

### TR-4：prompt 组装测试

覆盖：

- 每个角色 prompt 都追加到基础 prompt 后。
- 未指定角色时走默认角色或兼容路径。
- system prompt 中包含当前角色职责。
- system prompt 中包含禁止事项。

### TR-5：RunService / Server 测试

覆盖：

- `/v1/runs` 接收角色 ID。
- Run snapshot 返回角色 ID。
- 未知角色返回明确错误。
- 旧请求不传角色仍能启动。

### TR-6：Desktop 类型测试

覆盖：

- shared runtime 类型包含角色字段。
- normalize 函数正确处理角色字段。
- 缺字段时兼容旧数据。

### TR-7：UI smoke

覆盖：

- 4 个任务模板可见。
- 点击模板能启动对应角色 run。
- 当前 run 显示角色或阶段。
- 缺依赖时 UI 显示提示。

### TR-8：真实 LLM smoke

必须至少跑 4 个真实 LLM smoke：

- Researcher：给一个调研问题，输出事实和未知项，不调用写入工具。
- Planner：给一个目标，输出步骤、风险、审批点，不执行动作。
- Operator：给一个已确认动作，触发审批或执行低风险动作。
- Reviewer：给执行结果，输出复核结论，不执行写入工具。

### TR-9：端到端验收

至少跑一个完整 work 场景：

```text
用户目标
-> Researcher 汇总事实
-> Planner 输出计划
-> 用户确认
-> Operator 执行动作
-> Reviewer 复核结果
-> UI 可回看状态和结果
```

P0 如果尚未实现完整 WorkRun 编排，可以用手动分段启动 4 个角色完成验收。

## 19. 验收标准

功能验收：

- 4 个固定角色存在。
- Desktop 能发起每个固定角色的任务。
- 当前 run 能显示角色或阶段。
- 不同角色展示不同说明和能力边界。
- Researcher / Planner / Reviewer 默认只读。
- Operator 写入动作必须走审批。
- Reviewer 能读取任务结果并输出复核报告。
- 缺连接器或 Skill 时有明确提示。

技术验收：

- `go test ./...` 通过。
- 相关 Desktop 类型测试通过。
- 新增测试覆盖注册表、工具过滤、prompt 组装、run 透传。
- 真实 LLM smoke 有记录。

兼容验收：

- 不传角色的旧入口不崩溃。
- 未知角色返回明确错误。
- 现有 approval 逻辑不被绕过。
- 现有 run event / SSE 不被破坏。

## 20. 分阶段实施建议

### PR 1：AgentDefinition 注册表

交付：

- `src/agentdefs` 包。
- 4 个固定角色。
- 字段完整性测试。

### PR 2：agentcore 接入

交付：

- `AgentConfig.AgentDefinitionID`。
- RequestBuilder prompt 追加。
- tools 过滤。
- tool summary 更新。
- 单元测试。

### PR 3：runtime / server 透传

交付：

- `StartRunRequest` 支持角色字段。
- Run snapshot 返回角色字段。
- Server handler 校验角色。
- API 测试。

### PR 4：Desktop 任务模板

交付：

- 4 个任务模板或角色卡片。
- 启动 run 时传角色。
- 当前 run 展示阶段或角色。
- 缺依赖提示。

### PR 5：Reviewer 与最小验收链路

交付：

- Reviewer 输出格式。
- 手动分段四角色验收。
- 真实 LLM smoke。

## 21. 给 0 上下文开发者的执行提示

先读：

- `src/agentcore/types.go`
- `src/agentcore/request_builder.go`
- `src/runtime/run_service.go`
- `src/server/handlers.go`
- `src/tool/registry.go`
- `src/tool/permission.go`
- `apps/desktop/src/shared/runtime.ts`
- `apps/desktop/src/renderer/App.tsx`

然后按 PR 1 到 PR 5 分步实现。

开发约束：

- 只改本需求相关文件。
- 不接外部参考源码。
- 不做任意自定义角色编辑器。
- 不绕过现有 approval。
- 不改无关 UI。
- 不删除无关文档或历史文件。

完成后输出：

- 改动文件。
- 已实现角色。
- 工具过滤规则。
- API 字段变化。
- UI 入口变化。
- 测试命令和结果。

## 22. 后续版本

后续再做：

- 自动角色路由。
- 完整 WorkRun 编排。
- 后台任务回注协议。
- 连接器健康状态。
- Skill runtime 深度接入。
- 团队多 worker。
- 远程执行。
- 自定义角色和策略覆盖。

