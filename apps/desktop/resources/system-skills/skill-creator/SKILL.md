---
name: skill-creator
version: 1.0.0
description: 创建、校验或修改 Awaken 的 project/global Skill 包；先在任务工作区制作完整候选包，再用 Core 的同源校验与安装合同安全落地，绝不直接编辑受保护的在线 Skill 目录。
when_to_use: 用户要求创建、编写、验证、修复、更新或修改 Awaken Skill，或询问某个候选 Skill 及其目标安装路径是否合规时。
user-invocable: true
context: inline
---

# Awaken Skill Creator

目标是交付一个能被 Awaken 当前运行时真实发现的 Skill 包。只写对任务有用的说明和资源；不要用冗长模板替代用户的真实工作流，也不要把一次性偏好扩展成普遍规则。

## 安全边界

- 创建和修改都在任务工作区的普通候选目录中进行，例如 `skill-authoring/<skill-id>/`；不要把候选目录建在 `.awaken` 下。
- 不直接用 `Write`、`Edit` 或 `Bash` 修改 `~/.awaken/skills`、项目 `.awaken/skills` 或 bundled/plugin/managed 根。
- 修改已安装 Skill 时，先调用 `skill_package_export` 导出完整包。不要只复制 `SKILL.md`，否则可能丢失 `scripts/`、`references/`、`assets/` 或其他资源。
- `skill_install` 是结构校验、目标路径解析和安装/替换的唯一写入入口。不要手工猜测安装路径。
- `validate_only=true` 不安装、不移动旧包、不 reload；它证明结构与本次目标路径合规，不证明工作流行为正确。

## 包结构

每个包必须只有一个入口 `SKILL.md`；仅在工作流确实需要时添加资源：

```text
<skill-id>/
├── SKILL.md
├── scripts/       # 可选：需要重复、确定性执行的脚本
├── references/    # 可选：按需读取的详细规则或资料
└── assets/        # 可选：交付物模板、图片等非指令资源
```

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

- `name` 与 `description` 必须非空；说明要同时写清“做什么”和“何时使用”。
- `context` 只可为 `inline` 或 `fork`；没有经过真实子 Agent 验收时优先保留 `inline`。
- 参数类型只使用 `string`、`number`、`integer`、`boolean`。
- 保留用户已有且 Awaken 能解析的 frontmatter；不要为了统一风格删除未知字段。
- supporting resources 必须从正文明确说明何时读取或执行；没有具体用途就不要建空目录、README、CHANGELOG 或占位文件。
- 新脚本要实际运行验证；脚本中的路径必须从 Skill 包根或明确输入解析，不写死作者机器路径。

## 创建流程

1. 从用户目标提炼 2–4 个真实触发示例、预期输出和不可越权的边界。缺少会改变实现的事实才询问；能安全推断时直接推进。
2. 选定 `project`（默认、只服务当前项目）或 `global`（用户明确要求跨项目复用）范围。
3. 用 `Write`/`Edit` 在普通候选目录创建最小完整包；需要资源时一并创建并检查相对引用。
4. 调用 `skill_install`，传同一候选目录、目标 scope、`overwrite_policy: "abort"`、`validate_only: true`。
5. 若校验失败，只修改候选包并重验；不得绕过错误直接落盘。若成功，向用户说明 Skill ID、文件数、总大小与 Core 返回的 proposed target。
6. 用户请求本身包含创建并安装时，使用完全相同的候选源再次调用 `skill_install`，保持 `overwrite_policy: "abort"`、`validate_only: false`。同名冲突时停止，不擅自改为 replace。

## 修改流程

1. 明确已安装 Skill ID、原来源和目标范围；只把 `project`/`user` 包当作可直接替换的用户 Skill。其他来源若需派生，说明它会成为新包或高优先级覆盖，而不是修改原包。
2. 调用 `skill_package_export`，把完整包导出到一个不存在的候选目录。
3. 用 `Read`/`Glob` 盘点候选包，用 `Edit`/`Write` 做最小修改；默认保留原 ID、未涉及的 frontmatter 和全部配套资源。
4. 用 `skill_install validate_only=true` 校验候选包及与原包相同的 scope/project target。
5. 对比用户要求、入口说明、资源引用和脚本验证结果。只有用户明确要求修改该同名已安装包且候选已通过校验，才以 `overwrite_policy: "replace"` 安装；旧包必须由安装器移入系统废纸篓。

## 行为复核

结构安装成功后，再做与风险相称的行为验证：

- 说明 dry-run 只证明包和路径合规；
- 实际执行新增或变更的脚本；
- 用一个真实触发请求检查 Skill 是否被正确选择、是否遵守范围、是否产出预期结果；
- 安装或替换后使用新 run 验证，不能用已开始 run 的旧工具/Skill 快照冒充新版本已生效；
- 报告已验证项、未验证项和仍需用户确认的业务事实。

## 停止条件

- 来源包不存在、已变化或无法完整导出；
- 候选包有多个 `SKILL.md`、路径逃逸、非法 frontmatter、资源缺失或校验错误；
- 同名已存在但用户没有授权修改/替换；
- 用户要求直接改受保护在线目录；
- 行为测试需要新的外部权限、账户、付费或生产写入。

遇到上述情况应说明具体阻塞并保留候选包，不以手工写路径、删除旧包或放宽校验收尾。
