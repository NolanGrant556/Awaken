# AGENTS 指令与运行快照

## 1. 当前支持

Awaken 在每次 `StartRun` 中读取两种用户指令：

- 全局来源固定为 `~/.awaken/AGENTS.md`；
- 项目来源固定为当前项目主文件夹的 `<primary-root>/.awaken/AGENTS.md`。

多文件夹项目只把主文件夹作为项目指令 authority。次文件夹的 `.awaken/AGENTS.md` 仍可在右侧文件工作区按普通 Markdown 查看和编辑，但不进入模型上下文。切换主文件夹不会改变已经开始的 run；下一次 `StartRun` 从新的主文件夹读取。

首版不发现根目录 `AGENTS.md`、`AGENTS.override.md`、嵌套目录文件，也不提供 `/init`。缺失来源是正常状态，不产生 warning。

## 2. 优先级与模型上下文

优先级固定为：Awaken system/安全规则 > 当前用户直接指令 > 项目 AGENTS > 全局 AGENTS。非冲突规则合并，项目规则覆盖全局冲突项。AGENTS 内容不能扩大 TaskScope、工具池、Hook/Computer policy、凭据或路径权限；ToolRuntime 和对应安全 owner 始终是最终执行边界。

`src/instructions` 按“全局在前、项目在后”生成不可变 `InstructionSnapshot`。`RunService` 在 TaskScope 和主文件夹确定后、Agent 执行前捕获快照，并同时写入 run 的安全摘要与 `AgentConfig.InstructionSnapshot`。Renderer 只提交项目/thread/用户输入，不提交 AGENTS 正文或来源路径作为运行参数。

RequestBuilder 使用 `ContextEnvelope.UserContextItems` 生成 request-only synthetic user prefix。该前缀位于 conversation messages 之前，当前直接 user message 仍具有更高优先级；正文不拼入 system prompt，不追加到 Conversation，也不进入 `user_message`、SemanticJournal 或 AG-UI。默认 system prompt 只包含稳定的作用域、优先级和冻结规则。

## 3. 边界、截断与诊断

每个来源最多向模型注入 16 KiB，两个来源合计最多 32 KiB。超限内容保留有界前缀并在 16 KiB 总预算内追加明确截断标记；摘要记录 original/applied bytes 和 `truncated=true`。非法 UTF-8 使用 U+FFFD 替换并产生 warning。

加载前后核对 regular-file identity、size 和 modification time；读取中变化会重试一次，仍变化则跳过。不可读、非普通文件或解析后的路径越出对应全局/项目根时跳过该来源。单一来源失败时继续使用另一来源并把 snapshot 标为 `partial=true`；两个来源都失败仍允许 run，但安全摘要和 Composer 必须显示 diagnostics。

## 4. 冻结、Context Epoch 与压缩

每个 `StartRun` 重新生成 snapshot ID；同一 run 的所有模型边界、provider retry、shared/shared_readonly child 和 git-worktree child 复用父 run 的同一快照。git worktree 不能因临时目录中的文件产生新的项目指令 authority。

Context Epoch schema v4 把 synthetic prefix messages 和 instruction digest 保存到 thread 的 epoch sidecar，与 conversation history 分离。指令 digest 变化、来源删除或切换主文件夹时冷旋转；指令变化的边界原因为 `agents_instructions_changed`，workspace identity 变化仍使用 `workspace_moved`。旧正文不会出现在新 Epoch 请求中。

compact 成功后以 `compact_succeeded` 创建的新 Epoch继续携带当前 run 的同一 prefix；compact 失败或取消不切 Epoch。Provider retry 使用同一个 prepared request，不重新读取文件。Context Epoch pointer/sidecar/digest 损坏时阻止模型请求并保留原数据，不回退到猜测的旧规则；唯一自动归一化例外是可由 current value digest 与同 sequence 的完整 append delta 精确证明的“晚加入 source 缺少空 baseline digest”旧 v4 writer 形态，其他不一致仍 fail-stop。

## 5. 耐久、断线与重启

`instruction_snapshot` SemanticKind 只保存 snapshot ID、captured time、partial、来源 scope/display path/scope root/digest/bytes/truncated/status 和 diagnostics，不保存正文。`Run.instruction_snapshot`、`AWAKEN_INSTRUCTION_SNAPSHOT`、start response、SSE 重连和 transcript replay 使用同一安全摘要。

Renderer/SSE 断线不影响 Core 中的活动 run 和冻结快照。Core 崩溃或 Desktop 完全重启后，未完成 run 按既有恢复合同成为 interrupted/error，不伪装续跑；历史保留当时摘要，下一次 run 重新读取当前文件。运行中保存、删除或切换主文件夹只影响下一次 run。

## 6. Owner 与验证

- 读取、校验、CAS 与快照：`src/instructions`。
- run 绑定与耐久摘要：`src/runtime/run_service.go`、`session.go`、`journal_store.go`。
- synthetic prefix 与 child 继承：`src/agentcore/request_builder.go`、`context_epoch.go`、`runner.go`、`spawner.go`。
- Epoch sidecar：`src/runtime/context_epoch_store.go`。
- API/事件：见 [09 协议域](../09_AGUI与A2UI协议/索引.md)。
- Desktop 交互：见 [01 AGENTS 指令交互](../01_桌面工作台/AGENTS指令交互.md)。

自动化至少覆盖 global/project/同时/缺失、主文件夹、截断、UTF-8、越界 symlink、request-only prefix、Epoch 旋转、journal 无正文和 live/replay 摘要。真实 Desktop 还必须验证运行中编辑只影响下一 run、compact、断线重连与完整重启。
