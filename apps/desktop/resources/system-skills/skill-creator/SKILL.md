---
name: skill-creator
version: 1.1.0
description: 创建、校验或修改 Awaken 的完整 project/global Skill 目录包，包括 SKILL.md、按需 references、scripts、assets 与其他资源；先在任务工作区制作候选包，再通过 Core 同源合同安全落地。
when_to_use: 用户要求创建、编写、验证、修复、更新或修改 Awaken Skill，或询问某个候选 Skill 及其目标安装路径是否合规时。
user-invocable: true
context: inline
---

# Awaken Skill Creator

目标是交付一个能被 Awaken 当前运行时真实发现并按需使用资源的完整 Skill 包。`SKILL.md` 是入口，不是整个 Skill；创建前必须先判断工作流需要哪些参考资料、确定性脚本、模板或其他资源。不要用冗长模板替代用户的真实工作流，也不要把一次性偏好扩展成普遍规则。

## 安全边界

- 创建和修改都在任务工作区的普通候选目录中进行，例如 `skill-authoring/<skill-id>/`；不要把候选目录建在 `.awaken` 下。
- 不直接用 `Write`、`Edit` 或 `Bash` 修改 `~/.awaken/skills`、项目 `.awaken/skills` 或 bundled/plugin/managed 根。
- 修改已安装 Skill 时，先调用 `skill_package_export` 导出完整包。不要只复制 `SKILL.md`，否则可能丢失 `scripts/`、`references/`、`assets/` 或其他资源。
- `skill_install` 是结构校验、目标路径解析和安装/替换的唯一写入入口。不要手工猜测安装路径。
- `validate_only=true` 不安装、不移动旧包、不 reload；它证明结构与本次目标路径合规，不证明工作流行为正确。

## 完整包结构

每个 Skill 都是一个目录包，并且必须只有一个根入口 `SKILL.md`。`scripts/`、`references/`、`assets/` 是通用约定，不是封闭白名单；工作流可以携带其他有明确用途的文件或目录：

```text
<skill-id>/
├── SKILL.md               # 必需：精简入口、路由、关键约束
├── scripts/               # 可选：重复且需要确定性执行的代码
├── references/            # 可选：按需读取的 API、schema、规则、示例
├── assets/                # 可选：模板、图片、字体、静态数据
└── <other-resources>/     # 可选：工作流确实需要的其他包内资源
```

不要为了“看起来完整”创建空目录。先做资源规划，再只创建有真实内容和调用方的目录。详细判断见 [references/package-design.md](references/package-design.md)。

### 资源规划门

写文件前先形成一份内部资源清单，逐项回答：

- 哪些事实每次都影响决策，必须留在 `SKILL.md`；
- 哪些 API 文档、字段表、错误码、业务规则或长示例只在特定分支使用，应进入 `references/`；
- 哪些逻辑会重复重写或必须确定性执行，应进入 `scripts/`；
- 哪些文件会被复制、修改或作为交付物底稿，应进入 `assets/`；
- 每个 supporting resource 由 `SKILL.md` 的哪一处明确路由到它。

满足以下任一条件时，不得在没有解释的情况下只交付一个 `SKILL.md`：多阶段业务流程、API/MCP 工具编排、字段/schema 较多、需要错误恢复、包含可复用模板、需要确定性校验或已有外部文档需要沉淀。若最终仍只需入口文件，交付摘要要说明为什么没有 supporting resources。

`SKILL.md` 必须以 YAML frontmatter 开始。新包优先使用简洁的英文小写连字符名称，使稳定 ID 可预测；不要以 `mcp__` 或 `plugin__` 开头。

```markdown
---
name: daily-followup
description: 汇总每日跟进记录并输出风险、责任人和下一步；当用户要求做日常业务跟进时使用。
user-invocable: true
context: inline
---

# Daily Followup

写入真正改变 Agent 决策的任务知识、顺序、边界和验收要求。
```

- `name` 与 `description` 必须在 frontmatter 中显式存在；不能依赖运行时从目录或正文推导。
- `name` 必须与父目录名完全一致，长度为 1–64，只使用小写 ASCII 字母、数字和单连字符，不能有首尾或连续连字符。
- `description` 长度为 1–1024，并同时写清“做什么”和“何时使用”。
- 标准可选字段中，`license` 必须是字符串，`compatibility` 必须是非空的 1–500 字符字符串，`metadata` 必须是 string->string map，`allowed-tools` 必须是空格分隔字符串；不要用 `null` 冒充省略。
- `context` 只可为 `inline` 或 `fork`；没有经过真实子 Agent 验收时优先保留 `inline`。
- 参数类型只使用 `string`、`number`、`integer`、`boolean`。
- 保留用户已有且 Awaken 能解析的 frontmatter；不要为了统一风格删除未知字段。
- supporting resources 必须从正文明确说明何时读取或执行；没有具体用途就不要保留空目录、README、CHANGELOG 或占位文件。
- 新脚本要实际运行验证；脚本中的路径必须从 Skill 包根或明确输入解析，不写死作者机器路径。

## 确定性脚手架

新建 Skill 时优先使用包内脚手架创建候选根和所需资源目录：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/init_skill.py" <skill-id> \
  --path <candidate-parent> \
  --description "<做什么，以及何时使用>" \
  --resources references,scripts,assets
```

`--resources` 只传本次资源规划实际需要的目录；脚手架不会覆盖已存在的目标。生成后必须填充或删除空目录，并按真实工作流改写入口正文。

Core dry-run 前先做一次无写入预检：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/validate_skill.py" <candidate-skill-root>
```

此预检检查 Agent Skills 的必填字段、名称/目录一致性、包大小、symlink、空资源目录和入口中引用的包内路径；最终权威仍是 `skill_install validate_only=true`。

## 创建流程

1. 从用户目标提炼 2–4 个真实触发示例、预期输出和不可越权的边界。缺少会改变实现的事实才询问；能安全推断时直接推进。
2. 选定 `project`（默认、只服务当前项目）或 `global`（用户明确要求跨项目复用）范围。
3. 完成资源规划；复杂工作流需要只有一个 `SKILL.md` 时记录具体理由。
4. 使用包内脚手架或 `Write`/`Edit` 在普通候选目录创建完整包，填充真实资源，并在入口中写清按需路由。
5. 运行零外部依赖的 `validate_skill.py`，修复包结构、基础入口与资源引用 error，并处置空目录、未引用资源等 warning。
6. 调用 `skill_install`，传同一候选目录、目标 scope、`overwrite_policy: "abort"`、`validate_only: true`。Core 以真实 YAML parser 按 Agent Skills 规范完整校验 `name`、`description`、`license`、`compatibility`、`metadata` 和 `allowed-tools`；脚本预检不能替代这道权威门禁。
7. 若校验失败，只修改候选包并重验；不得绕过错误直接落盘。若成功，向用户说明 Skill ID、完整文件树、文件数、总大小与 Core 返回的 proposed target。
8. 用户请求本身包含创建并安装时，使用完全相同的候选源再次调用 `skill_install`，保持 `overwrite_policy: "abort"`、`validate_only: false`。同名冲突时停止，不擅自改为 replace。

## 修改流程

1. 明确已安装 Skill ID、原来源和目标范围；只把 `project`/`user` 包当作可直接替换的用户 Skill。其他来源若需派生，说明它会成为新包或高优先级覆盖，而不是修改原包。
2. 调用 `skill_package_export`，把完整包导出到一个不存在的候选目录。
3. 用 `Read`/`Glob` 盘点完整候选树，用 `Edit`/`Write` 做最小修改；默认保留原 ID、未涉及的 frontmatter 和全部配套资源。不得把多文件包压回单文件。
4. 重新执行资源规划与 `validate_skill.py`，确认新旧资源都有明确调用方。
5. 用 `skill_install validate_only=true` 校验候选包及与原包相同的 scope/project target。
6. 对比用户要求、入口说明、资源引用和脚本验证结果。只有用户明确要求修改该同名已安装包且候选已通过校验，才以 `overwrite_policy: "replace"` 安装；旧包必须由安装器移入系统废纸篓。

## 行为复核

结构安装成功后，再做与风险相称的行为验证：

- 说明 dry-run 只证明包和路径合规；
- 实际执行新增或变更的脚本；
- 用 `skill_resource_list` 复核新 run 看到的完整包文件树，再用 `skill_resource_read` 读取至少一个新增或变化的文本资源；
- 用一个真实触发请求检查 Skill 是否被正确选择、是否遵守范围、是否产出预期结果；
- 安装或替换后使用新 run 验证，不能用已开始 run 的旧工具/Skill 快照冒充新版本已生效；
- 报告已验证项、未验证项和仍需用户确认的业务事实。

## 停止条件

- 来源包不存在、已变化或无法完整导出；
- 候选包有多个 `SKILL.md`、名称与目录不一致、路径逃逸、非法 frontmatter、资源缺失或校验错误；
- 同名已存在但用户没有授权修改/替换；
- 用户要求直接改受保护在线目录；
- 行为测试需要新的外部权限、账户、付费或生产写入。

遇到上述情况应说明具体阻塞并保留候选包，不以手工写路径、删除旧包或放宽校验收尾。
