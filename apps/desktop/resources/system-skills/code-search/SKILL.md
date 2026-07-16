---
name: code-search
version: 1.0.0
description: 使用现有 WebSearch 检索代码、API 用法、具体符号与错误信息，并给出可复制的最小片段、版本约束、风险和来源。
when_to_use: 用户需要查找某语言、框架或库的代码示例、具体符号用法、版本差异或错误解决方案时。
user-invocable: true
context: inline
---

# Code Search

只使用现有 `WebSearch` 工具完成代码检索，不调用动态 MCP 工具，不调用已弃用的 `get_code_context_exa`，也不假设存在独立 `CodeSearch` 或 `WebFetch` 工具。

当前固定使用 `context: inline`，在主 Agent 上下文中执行。这是现阶段的产品选择：等 Awaken 的 subagent 产品入口与真实 Desktop 验收完整后，再评估迁移为 fork；迁移前不得只修改 frontmatter 而绕过父子上下文、工具继承和验收合同。

## 查询要求

每次查询尽量同时包含：

- 编程语言；
- 框架或库及其明确版本；版本未知时先说明；
- 具体 API、类型、函数、配置项或完整错误文本；
- 预期行为或运行环境。

优先搜索官方文档、官方仓库与 GitHub，其次才是维护良好的高质量技术来源。需要限定来源时使用 `allowed_domains`；排除低质量或重复站点时使用 `blocked_domains`。

## 结果整理

1. 合并重复 fork、镜像仓库、转载文章和内容相同的答案。
2. 先说明适用的版本与前置条件。
3. 提供解决问题所需的最小可复制代码片段，不拼接未经来源证明的大段实现。
4. 明确列出已知风险、弃用状态、平台差异或版本不兼容。
5. 每个关键结论都附上来源 URL；官方资料与社区经验冲突时，以官方资料为准并指出差异。
6. 搜索结果不足以确认结论时直接说明不确定性，不猜测不存在的 API。
