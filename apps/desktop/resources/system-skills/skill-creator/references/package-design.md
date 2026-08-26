# Skill 包设计与渐进加载

只在创建或大幅修改 Skill、需要决定入口与 supporting resources 边界时读取本文件。

## 三层信息结构

1. `name` 与 `description`：目录阶段始终可见，只负责准确触发。
2. `SKILL.md`：Skill 被激活时加载，负责目标、路由、关键边界和共同流程。
3. supporting resources：仅在当前分支需要时列出或读取，负责条件性细节与确定性执行。

`SKILL.md` 应告诉模型“什么时候读哪个资源”，而不是只说“参考 references”。例如：

```markdown
- 创建采购入仓单前读取 `references/purchase-receipt.md`。
- 返回 409 或重复单号时读取 `references/recovery.md`，不要盲目重试。
- 需要生成固定 payload 时运行 `scripts/build_payload.py`，不要重新手写字段映射。
```

## 资源归属

| 内容 | 推荐位置 | 原因 |
| --- | --- | --- |
| 共同流程、决策点、安全边界 | `SKILL.md` | 每次激活都需要 |
| API 字段、schema、状态表、错误码 | `references/` | 只在相应接口或异常分支读取 |
| 大型示例、业务规则、供应商差异 | `references/` | 避免挤占所有任务上下文 |
| 重复转换、校验、打包、解析 | `scripts/` | 提高确定性并避免重复写代码 |
| Word/PPT/JSON 模板、图片、字体、静态数据 | `assets/` | 用于交付物，不作为默认指令加载 |
| 其他领域资源 | 自定义目录 | 必须有清楚调用方和相对路径 |

## API 与 MCP Skill 最低设计检查

复杂 API/MCP Skill 至少逐项判断是否需要：

- `references/tool-map.md`：用户意图到工具/接口的映射；
- `references/input-contracts.md`：必填字段、主数据来源、禁止猜测字段；
- `references/workflows.md`：多阶段顺序、幂等、确认点；
- `references/recovery.md`：错误分类、重试、补偿和停止条件；
- `references/examples.md`：脱敏的请求/返回样例；
- `scripts/`：只有 MCP 工具无法提供而又会重复执行的确定性辅助逻辑。

MCP server 继续拥有认证、实时数据和受控写入；Skill 不复制第二套 API client。若 MCP 工具已经稳定完成某项执行逻辑，Skill 只记录选择、编排和验证规则。

## 完成标准

- 入口能准确路由到每个资源；
- 没有未引用或空的资源目录；
- 没有重复维护的同一事实；
- 脚本从包根或明确参数解析路径，并已实际运行；
- `name` 与父目录完全一致；
- 预检与 Core dry-run 均通过；
- 安装后新 run 能列出文件树并读取所需资源。
