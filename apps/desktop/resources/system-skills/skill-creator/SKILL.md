---
name: skill-creator
version: 1.2.0
description: 创建、修改或校验 Awaken Skill 目录包时使用。
user-invocable: true
context: inline
---

# Awaken Skill Creator

交付准确触发、按需提供知识的 Skill。只写能改变 Agent 决策的非显然知识；description 简短说明何时使用，不列穷尽能力或吸引无关任务。SKILL.md 保留目标、路由、共同不变量与完成条件。

## 共享执行环境

执行 helper 前，通过 `tool_find` 找到 `load_workspace_dependencies`，以 `{}` 取得整套办公环境。每次 Bash 显式传入本 Agent 的 `environment_ref`，使用返回的解释器与启动参数；切换 Python/Node 或项目复用底层安装，各任务取得自己的引用。

缺失或损坏时由同一入口准备或恢复。失败如实报告原因，不改用系统软件、`ensure_environment` 或项目/全局安装补齐办公包。环境、文件内容、结构、视觉与发布分别验收。

## 按需选择

- 需要选择资源边界时读 [包设计](references/package-design.md)：条件性知识进入 references/，确定性重复逻辑进入 scripts/，实际交付底稿进入 assets/；简单技能无需额外目录。
- 新建候选或修改 schema／脚手架时读 [包结构与脚手架](references/包结构与脚手架.md)，使用 `scripts/init_skill.py` 和 `scripts/validate_skill.py`。
- 安装、替换或验证已安装版本时读 [安装与修改](references/安装与修改.md)。
- 窄修改只检查受影响文件与路线，不重新初始化已存在候选，也不强制通读所有参考。

## 写入与身份

候选放当前任务工作区普通目录，不建在 `.awaken`。修改已安装 Skill 先 `skill_package_export` 导出完整包，保留 ID、未涉及元数据和所有资源。安装／替换只经 `skill_install`，不直接写 user/project/bundled/plugin/managed 安装根。

新候选显式包含合法 `name` 与 `description`；name 与父目录一致。兼容扩展由 Core 解析，不为统一样式删除未知字段。用户 Skill 仅 project/user 可替换；其他来源派生须明确目标身份，不冒充改原包。

## 验证与完成

通过 `${AWAKEN_SKILL_DIR}` 的绝对路径执行预检；它只负责包结构与资源引用，不能代替 `skill_install validate_only=true`。新增／变更脚本需实际运行，已有未受影响脚本不重复全部执行。

用户已要求创建并安装或修改准确同名包时，候选验证后继续对应安装／replace；不因首次候选完成再等待无必要批准。同名冲突、来源漂移或新增外部副作用超出授权时暂停相关动作，保留候选。

安装后在新 run 用 `skill_resource_list` / `skill_resource_read` 核对实际需要的资源，并以代表性请求验证触发与结果。已开始 run 的旧快照不能证明新版本生效。只交付候选或只做结构检查时如实报告该范围，不强制安装或声称真实行为通过。
