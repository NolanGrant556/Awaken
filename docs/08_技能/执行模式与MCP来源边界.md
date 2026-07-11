# 执行模式与 MCP 来源边界

## 1. 权威范围

本文唯一拥有 Skill 的 inline/fork 当前行为、外部依赖可用性和 MCP 动态来源边界。MCP transport、认证和工具生命周期由 [MCP 运行时](../06_MCP运行时/索引.md) 拥有；本文不重新定义它们。

## 2. Inline 与 fork

`context` 省略时默认为 `inline`。

- inline：激活后的正文、参数和资源上下文进入当前 run，effective tool pool 仍受父 run 边界限制。
- fork：执行器存在 fork 分支和 `ChildRunStarter` 接口；如果注入了 child starter，可把激活计划交给子 run。
- 当前Desktop/production没有注入 `StartChild`。模型调用 `skill__*` 工具时会经过 `ForkExecutor`，记录warning后降级inline。
- composer显式Skill token走 `preActivateComposerSkills`，直接把 `plan.MainBody` 加入当前run ContextBlock，不检查 `context`，也不记录fork降级warning；这条路径是**静默inline**。

在真正接线前，不得把 fork 写成并行运行、独立上下文窗口、独立审批域或独立持久化线程。未来实现必须先定义父子 run 所有权、取消传播、工具权限继承、日志归属和 Desktop 投影。

## 3. Connector 与 MCP 依赖

registry 能表达 `requiredConnectors` 和 `requiredMcpServers`，两组条件取 AND：任一缺失都使 Skill `unavailable`。

当前接线并不对称：

- `SetConnectorAvailable` API 和单元测试存在，但 production 中没有连接器生命周期调用它。不能把 `requiredConnectors` 写成已完成的自动可用性闭环。
- MCP bridge 会调用 `SetMcpServerAvailable`，但当前 `syncForServer` 在 server 没有可桥接 prompt/resource 时把该 server 标为 unavailable。这会让只声明 `requiredMcpServers` 的本地 Skill 在 server 已连接时仍可能错误不可用。

因此这两项是已确认实现缺口，不是文档可以掩盖的用户配置错误。

## 4. MCP 动态 Skill 映射

MCP server 进入 connected 后，bridge 枚举：

- `prompts/list` 返回的 prompt；
- `resources/list` 中 MIME 为 `text/x-prompt` 的 resource。

条目以 `mcp__<server>__...` ID 动态注册，带 `requiredMcpServers` 和 `McpBinding`；断开、disable 或 reconcile 时注销。它们不写入project/user skills目录。设计上重启后应随MCP server重新发现，但当前bridge没有启动snapshot reconcile，连接事件早于订阅时会漏发现；请求级根因见06权威。

MCP请求、arguments/messages、resource URI/read、timeout和启动订阅竞态的唯一缺口清单在 [工具接入与技能桥接](../06_MCP运行时/工具接入与技能桥接.md)。本域只拥有其Skill后果：动态条目可能漏注册、正文可能不是真实prompt内容、resource无法可靠读取、依赖可用性可能错误，因此产品只能称“动态发现与部分桥接”。

## 5. 状态与恢复

```text
local load / MCP reconcile
  -> available | unavailable | shadowed
  -> activation preflight
  -> inline | fork requested
  -> current production: inline execution
```

- connector/MCP状态事件被bridge观察并触发registry重算时，可从unavailable恢复为available；启动前已丢失的connected事件当前不会自动补偿。
- MCP server 断开时，bridge 注销其动态 Skill；不能继续使用旧 body fetcher。
- 获取 prompt body 失败使该次激活失败，不回退到过期缓存正文。
- 模型Skill工具的fork降级必须保留可观察warning；composer token当前静默inline，必须按缺口呈现，不能声称已隔离。

## 6. 不变量

- Skill 来源状态不能替代 MCP server 的真实连接状态。
- MCP 动态 Skill 的名字、正文和生命周期由 server 驱动；本地包不得冒用 `mcp__`。
- 依赖检查只能收紧可用性；缺少 production 状态接线时必须记录缺口，不能默认 available。
- 修复 bridge 时不得绕过 06 的认证、transport、tool namespace 和 lifecycle 合同。

## 7. 验证依据

- 实现：`src/skillruntime/executor.go`、`src/skillruntime/fork.go`、`src/skills/registry.go`、`src/skills/mcp_bridge.go`、`cmd/awaken-core/main.go`。
- 测试：`src/skillruntime/executors_manager_test.go`、`src/skills/skills_test.go`、`src/skills/schema_v2_test.go`。现有测试覆盖 registry 状态和桥接条目转换，没有独立 bridge 端到端测试。
- Desktop：只读核对 MCP 来源分组；不得以 UI 列出条目推断 prompt body 已正确执行。
