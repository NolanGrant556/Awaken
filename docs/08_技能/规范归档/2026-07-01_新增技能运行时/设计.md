## Context

`sourcecode/src/skills/loadSkillsDir.ts` 是 Claude Code 的现成参考：frontmatter + Markdown 正文，靠 `name`、`description`、`whenToUse` 让 agent 选用，靠 `allowed-tools` 收紧权限。Skill 本质是"打了元数据的 prompt 模板"，但在 work 场景下，它要进一步声明"需要哪些 connector"和"绑定哪个 work 角色"（Researcher/Planner/Operator/Reviewer）。

执行模型上，Claude Code 用 `context: fork` 让长 Skill 跑到子上下文里再回主。这个 model 对 work 场景同样关键——例如"客户跟进日报"可能要遍历 30 个客户，不应污染主对话。

`AGENTS.md` 要求 runtime / agent 路径有 real-LLM smoke。Skill 执行属于 agent 路径，需要覆盖。

## Goals / Non-Goals

**Goals:**

- `SKILL.md` 单文件即可被发现、参数化、调用。
- 多来源（bundled / user / project）按优先级合并，同名时 project > user > bundled，被遮蔽方在 registry 里仍可见（带 `shadowedBy`）。
- Skill 可以声明 `requiredConnectors`，缺时 UI 能解释（`unavailable` 状态 + 原因）。
- Skill 可以以 `inline` 或 `fork` 模式执行；inline 在主 run 内拼上下文，fork 走子 run。
- 内置 4 个 work skill 作为首批样板，避免 v1 用户拿到空 library。
- Skill 暴露给 agentcore 的形态统一是"工具"——agent 不需要知道 Skill 概念，只需要在 tool list 里看到它，避免新的 mental model。

**Non-Goals:**

- 不做 Skill 搜索/排序优化（动态 ranker 留 M2 后）。
- 不做 Skill marketplace、远端拉取。
- 不做 Desktop Skill Library UI（产品侧）。
- 不实现 MCP prompts/resources → Skill 桥接（独立 change）。
- 不实现 Skill 编辑器；用户只能通过文件编辑 SKILL.md。
- 不引入 Skill 版本管理；同名覆盖即生效。

## Decisions

**Decision 1：Skill 在 agentcore 眼里是"工具"。**

注册一个 `SkillTool`（单一 Go struct，实现 `Tool` interface），把所有 enabled skill 作为该 tool 的 `subcommands` 暴露——`tools/list` 里要么列一个 `run_skill` 工具加 `skill_id` 参数，要么每个 skill 一个独立工具。**选独立工具：** 每个 Skill 一个独立 `Tool` 注册（命名 `skill__{id}`），让 LLM 在 tool selection 时直接看到名字和 description，比 `run_skill(skill_id=...)` 选择质量高，也符合 Claude Code 的做法。

代价：tool list 体积会涨。缓解：在 `tools/list` 时只列 `state == "available"` 的 skill；并在 agent 系统提示里说明"Skill 是固化方法，优先考虑"。

**Decision 2：`context: fork` 的执行模型。**

inline 模式 = Skill 正文直接作为 user message append 到主 run 上下文，沿用主 run 的工具池和模型；fork 模式 = 起子 run（复用 `src/runtime.RunService.StartChild` 接口，若不存在则本 change 起 stub），子 run 完成后把 summary 回主 run。

子 run 的工具池 = `allowedTools` 与主 run 工具池的交集。子 run 完成时发 `AWAKEN_SKILL_COMPLETED` 事件（新增 awaken 扩展事件类型）。

**Decision 3：参数替换格式。**

借鉴 Claude Code，使用 `${arg_name}` 占位符。`arguments` frontmatter 字段定义参数名、类型、是否必填、默认值。调用 Skill 时 LLM 通过 tool call 入参传入。运行时把 SKILL.md 正文中的占位符替换。

**Decision 4：来源优先级。**

`project > user > bundled`。理由：项目特化能 override 用户通用偏好；用户偏好能 override 内置默认。被遮蔽方仍保留在 registry 内，UI 显示 `shadowedBy`，避免"我明明加了 SKILL 为什么没生效"的困惑。

**Decision 5：`requiredConnectors` 失败行为。**

缺 connector 时 Skill **不消失**，状态为 `unavailable`，`tools/list` 不暴露给 LLM，但 `GET /v1/skills` 仍列出并解释"缺 notion connector"。这点和 Claude Code 一致，对产品体感很重要。

**Decision 6：首批内置 SKILL.md 文案的范围。**

本 change 只起骨架（合法 frontmatter + 一段 placeholder 正文 + 标注 TODO），不要求文案的 work 场景质量在本 change 里达标。文案打磨在产品侧 follow-up。理由：避免文案 review 拖住整个能力上线。

## Risks / Trade-offs

- [tool list 膨胀让 LLM 选错] → 通过 `state: available` 过滤、`requiredConnectors` 约束、以及未来的 ranker 缓解。
- [SKILL.md 模板执行被注入恶意 prompt] → 用户级 / 项目级 Skill 是用户自己写的，假设可信；后续接 team / remote 来源时必须加签名机制（留 follow-up）。
- [fork 模式依赖 Nolan 子 run 接口] → 接口未到位前 inline 模式先 ship，fork 退化为内联（同时记录 warning），文档说明。
- [frontmatter 解析容错] → 严格 YAML 解析失败时不让整目录 fail；记录单 Skill 错误并跳过，避免"一个写错全 library 不见"。

## Migration Plan

首次落地。bundled skill 编译期 embed；用户首次升级后即可在 `GET /v1/skills` 看到内置 4 个。

## Open Questions

- bundled SKILL.md 文案是否要双语？M1 仅中文；英文版留 M2。
- Skill 是否允许调用别的 Skill（嵌套 fork）？v1 不允许，避免循环。
- `hooks` 字段最小集是 `preExecute` / `postExecute` 两个 shell hook，还是更通用的事件订阅？v1 只接 `postExecute`（执行完发通知/落盘），其它留接口。
