## Why

Awaken 的差异化不是"能聊天"，而是"能按某类工作的方法做事"。Plan 把这一点说得很直白：Skill 是把方法（SOP）沉淀下来的最小单元。如果不能把"会议纪要整理 + 分发"、"客户跟进日报"这种重复工序固化下来，agent 每次都要从零理解任务，质量和效率都不可控。

Claude Code 的 `SKILL.md` 已经验证了"frontmatter + 自然语言 + 工具白名单"的最小可用形态：可发现、可参数化、可绑定 agent、可控制工具范围。Awaken 需要把同一套抽象 port 到 Go，并和 work 场景的 agent 角色对齐。

这一层不做出来，前面 connector 接入的外部工具只能被 agent 临时拼凑使用，没办法被打包成"开会后跑一下这个 Skill 就能产出会议纪要发到群里"这种确定性流程。

## What Changes

- 新增 `src/skills` 包：定义 `SkillDefinition`、`SkillRegistry`、`SkillLoader`、frontmatter 解析器。
- 新增 `src/skillruntime` 包：`SkillTool`（把 Skill 暴露给 agentcore 作为可调用的"工具"）+ `SkillExecutor`（inline / fork 两种执行模式）+ 参数替换。
- 支持 `SKILL.md` schema：`name`、`description`、`whenToUse`、`allowedTools`、`arguments`、`model`（可选 override）、`effort`（可选）、`context`（`inline | fork`）、`agent`（可选绑定到具体 work 角色）、`hooks`（最小集，pre/post execute）、`requiredConnectors`（可选）。
- 来源支持：`bundled`（编译进二进制的内置）、`user`（`~/.awaken/skills/`）、`project`（`<workspace>/.awaken/skills/`）；按 specificity project > user > bundled 取胜。
- 首批内置 4 个 work skill（具体 SKILL.md 内容由本 change 起骨架，文案在 follow-up 完善）：
  - `meeting-notes-distribute` — 会议纪要整理与分发
  - `customer-followup-daily` — 客户跟进日报
  - `sheet-anomaly-scan` — 表格巡检与异常汇总
  - `doc-to-actions` — 文档转行动项
- Skill 可声明 `requiredConnectors`，缺连接器时 Skill 仍然可见但状态为 `unavailable` 并解释原因（用户体验：不要让 Skill 消失，否则用户不知道接什么）。
- `SkillTool` 暴露给 agentcore，agent 可通过 tool call 触发 Skill；fork 模式时复用 `src/runtime` 的子 run 机制（依赖 Nolan 那边给出的子任务接口；本 change 用最小占位）。
- 新增 server API：
  - `GET /v1/skills` — 列出所有 Skill 与可用性。
  - `POST /v1/skills/{id}/execute` — 直接触发（mainly for testing 和未来 UI Library）。
- 不在范围：MCP prompts/resources → Skill 桥接（独立 change `add-mcp-prompts-as-skills`）；Skill Marketplace、动态搜索；Desktop Skill Library UI。

## Capabilities

### New Capabilities

- `skills`: SKILL.md 解析与加载、Skill 注册表、Skill 执行运行时（inline / fork）、Skill 与工具/连接器的依赖声明。

### Modified Capabilities

无（不修改 mcp-runtime 或 connectors；通过它们对外暴露的接口消费）。

## Impact

- 新增 Go 包：`src/skills`、`src/skillruntime`、`src/server/handlers_skills.go`。
- 修改 `src/tool/registry.go`：注册 `SkillTool`（把 Skill 作为一种特殊工具暴露给 agentcore）。
- 修改 `src/agentcore/runner.go`：调用 `SkillTool` 时附加 Skill metadata 到事件流。
- Skill fork 执行需要 `src/runtime` 子 run 能力（与 Nolan 协调；本 change 给出最小 stub 并标记 TODO，能跑通 inline 模式即可）。
- 新增内置 SKILL.md 文件到 `src/skills/bundled/`（编译期 embed）。
- 不引入新外部依赖（frontmatter 解析自实现或用 stdlib YAML）。
