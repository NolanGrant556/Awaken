# Hook 使用指南

## 1. Hook 是什么

Hook 是在固定产品生命周期边界自动触发的规则。普通工具由模型在当前任务中临时决定是否调用；Hook 在配置、启用并满足 matcher 后，由 Awaken 在对应事件边界确定性触发。

一个 Hook 由三部分组成：

1. **事件**：何时触发，例如 run 启动、工具执行前、最终回答形成前；
2. **matcher**：工具事件要匹配哪些 canonical tool name；非工具事件没有 matcher；
3. **执行器**：触发后运行 `prompt`、只读 `agent`、本机 `command` 或受控 `http`。

完整事件、字段、输出协议、安全和恢复规则只以 [Hook 产品与架构契约](Hook产品与架构契约.md) 为准。本文只提供当前能力的入门路径和可复用示例。

## 2. 推荐使用流程

1. 在 Awaken Desktop 选择目标项目；
2. 在普通对话中描述需求，并明确要求主智能体调用 `configure_hook`；
3. 新手优先选择 `project_local`，避免影响其他项目；
4. AI 写入配置后打开 `设置 > 钩子`，展开检查事件、matcher、执行器、timeout、输出上限和失败策略；
5. project/project-local 来源会显示“待信任”，确认内容安全后点击“信任当前配置”；
6. 新建任务验证行为。配置和信任变化只影响后续新 run，不热改当前 run；
7. 配置内容变化会使原摘要信任失效，需要重新检查和信任。

三个持久 scope 的用途：

| scope | 配置路径 | 适用场景 |
| --- | --- | --- |
| `project_local` | `<project>/.awaken/hooks.local.json` | 个人项目差异，推荐入门；如不提交 Git，应自行加入 `.gitignore` |
| `project` | `<project>/.awaken/hooks.json` | 团队共享并随仓库审查的规则 |
| `user` | `~/.awaken/config/hooks.json` | 对所有项目生效的本机通用规则 |

设置页是检查与信任界面，不是表单编辑器。除直接维护 JSON 外，当前推荐通过 `configure_hook` 完成查看、upsert、删除和整来源启停。

## 3. 推荐从这个 Hook 开始

### 3.1 最终回答质量检查

`BeforeFinish + prompt` 不运行本机命令，效果容易观察，是风险最低的入门组合。可以在项目对话中发送：

> 请调用 configure_hook，为当前项目创建一个 project_local Hook。ID 为 `review-before-finish`，事件为 `BeforeFinish`，使用 prompt 执行器。检查候选回答是否完整满足用户要求、是否存在未经验证的完成声明；满足则返回 ok=true，不满足则返回 ok=false，并通过 reason 和 additionalContext 给出简短修改建议。超时 10 秒，Hook 自身失败时继续。

检查不通过时，Awaken 会把反馈交回主模型再修改回答；单次 run 最多允许两次 `BeforeFinish` 阻止结束。

适合：

- 防止遗漏用户明确要求；
- 检查测试、Desktop 验收和剩余缺口是否如实报告；
- 避免未验证却声明完成。

## 4. 经典实用示例

### 4.1 run 启动时加载项目规则

使用只读 `agent` 检查项目文件，并把规则摘要作为当前 run 的额外上下文：

> 请调用 configure_hook，为当前项目创建一个 project_local Hook。ID 为 `load-project-rules`，事件为 `RunStart`，使用 agent 执行器。只读检查 README.md、AGENTS.md 和与当前项目相关的文档，总结必须遵守的项目约束；不要修改文件，始终返回 ok=true，并把摘要放进 additionalContext。maxTurns 设为 2，失败时继续。

`agent` 可以使用不宽于父 run 的只读工具，但每个新 run 都会增加模型调用、时延和用量。稳定且已经由 Awaken prompt 提供的规则不应重复加载。

### 4.2 写入前保护敏感文件

这是典型的 `PreToolUse + command + block_tool`。先由用户审查并创建项目脚本：

```javascript
// scripts/hooks/protect-sensitive-files.mjs
let raw = "";
process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => (raw += chunk));
process.stdin.on("end", () => {
  const event = JSON.parse(raw);
  const path = String(event.subject?.input?.path ?? "");
  const blocked = [".env", "production.json", ".pem", ".key"].some((value) =>
    path.includes(value),
  );

  process.stdout.write(
    JSON.stringify({
      protocolVersion: 1,
      action: blocked ? "block_tool" : "continue",
      reason: blocked ? `禁止自动修改敏感文件：${path}` : undefined,
    }),
  );
});
```

再发送：

> 请调用 configure_hook，为当前项目创建 project_local Hook。ID 为 `protect-sensitive-files`，事件为 `PreToolUse`，精确匹配 `Write` 和 `Edit`，使用 command 执行器，argv 为 `["node", "scripts/hooks/protect-sensitive-files.mjs"]`，cwd 使用 project_root，超时 5 秒，失败策略为 block。

Hook command 的 stdin 是 version 1 `InputEnvelope` JSON；`subject.input` 是原工具输入。command stdout 必须为空或只包含一个 version 1 `Response` JSON。command 只接受 argv，不接受隐式 shell 字符串。

### 4.3 修改文件后自动格式化

项目已经提供稳定格式化命令时，可以发送：

> 请调用 configure_hook，为当前项目创建 project_local Hook。ID 为 `format-after-edit`，事件为 `PostToolUse`，匹配 `Write` 和 `Edit`，使用 command 执行器，argv 为 `["npm", "run", "format"]`，cwd 为 project_root，超时 30 秒，失败时继续。

大型项目不宜在每次写入后全量格式化。更合适的做法是让脚本读取 `subject.input.path`，只格式化本次涉及的文件。`PostToolUse` 不能改写已经完成的工具 outcome。

### 4.4 任务结束后发送本机通知

macOS 用户可以创建对所有项目生效的本机通知：

> 请调用 configure_hook，创建一个 user Hook。ID 为 `notify-run-ended`，事件为 `RunEnd`，使用 command 执行器，argv 为 `["osascript", "-e", "display notification \"Awaken 任务已结束\" with title \"Awaken\""]`，超时 5 秒，失败时继续。

`RunEnd` 只观察已经耐久化的终态，失败不会改变任务结果。该命令是 macOS 专用示例；当前 Windows Hook 尚无实机准入结论。

### 4.5 向外部服务发送结束事件

需要团队通知或审计时，可以使用 `RunEnd + http`。目标服务会收到固定 POST JSON `InputEnvelope`，并且必须返回空 body 或合法 Hook `Response` JSON。例如：

```json
{
  "protocolVersion": 1,
  "action": "continue"
}
```

配置请求示例：

> 请调用 configure_hook，为当前项目创建 project_local Hook。ID 为 `notify-run-service`，事件为 `RunEnd`，使用 http 执行器，URL 为 `https://hooks.example.com/awaken/run-ended`，超时 10 秒，失败时继续。不要在 URL 或配置中写入 secret 明文。

HTTP 默认拒绝 loopback 和私网地址；只有确需本机服务时才显式请求 `allowLoopback`。企业 proxy-only、重定向后的 proxy/`NO_PROXY` 重算和完整真实代理矩阵仍是已知缺口，不应把关键交付只绑定在 HTTP Hook 上。

## 5. command 与 HTTP 输出协议

command stdout 和成功 HTTP response body 使用同一严格协议：

```json
{
  "protocolVersion": 1,
  "action": "continue",
  "reason": "optional bounded text",
  "additionalContext": "optional bounded text",
  "updatedInput": {}
}
```

常见 action：

- `continue`：继续原流程，可附加当前 run 上下文；
- `block_run`：仅用于 `RunStart`、`UserPromptSubmit`；
- `block_tool`、`update_input`：仅用于 `PreToolUse`；
- `block_compact`：仅用于 `PreCompact`；
- `block_subagent`：仅用于 `SubagentStart`；
- `block_finish`：仅用于 `BeforeFinish`。

空 stdout 等价于 `continue`。未知字段、错误 version、多个 JSON、非法 action 或超限输出都会使本次 Hook 形成 protocol failure。

## 6. 查看、修改、停用与删除

可以直接在普通对话中发送：

> 请调用 configure_hook，列出当前项目 project_local 中的所有 Hook。

> 请调用 configure_hook，把 project_local 中 ID 为 `review-before-finish` 的超时改为 15 秒，其余行为保持不变。

> 请调用 configure_hook，停用当前项目的整个 project_local Hook 来源。

> 请调用 configure_hook，删除 project_local 中 ID 为 `review-before-finish` 的 Hook。

项目配置每次变化都会产生新摘要并回到“待信任”。AI 不能替用户信任配置，也不能通过自然语言授权推导永久信任。

## 7. 安全建议

- 从 `project_local + prompt + onFailure=continue` 开始；确认收益后再考虑 command、HTTP 或阻断；
- command 以当前登录用户身份在沙盒外运行，配置前必须审查完整 argv 和脚本；
- `onFailure=continue|block` 描述执行器自身失败后的策略，不等同于 Hook 正常返回的 `block_*` 控制决定；
- matcher 是 canonical tool name 精确匹配或 `*`，不是正则表达式；
- user scope 会影响所有项目，通知之外的规则优先使用项目 scope；
- 不把 token、密码或 API key 写入 command、URL、prompt 或 header；HTTP header 只能引用 Core secret store；
- Hook 变更只影响后续新 run。测试时新建任务，并同时观察 Workbench Hook 回执和 `设置 > 钩子` 状态；
- Windows 当前只有交叉编译证据，POSIX mode 尚未替换为正确的 Windows 用户 ACL，不能视为已实机支持。
