# Awaken Tool Layer Design

本文件是用户指定的稳定入口。根据根 `AGENTS.md` 的上下文档案约束，完整且唯一的设计文档保存在：

- [Awaken Tool Layer v2 架构设计](./04_工具权限与审批/工具层深度优化/架构设计.md)
- [Tool Layer v2 迁移说明](./04_工具权限与审批/工具层深度优化/迁移说明.md)
- [Tool Layer v2 验收记录](./04_工具权限与审批/工具层深度优化/验收记录.md)

设计结论：以现有 `ToolDefinition` 与 `ExecutionOutcome` 做 additive v2 升级；definition 成为 capability 的单一事实源，result 采用统一 envelope 加工具专属 details，并用 baseline + observed delta 表达状态，不重写 Agent Loop 或 UI。
