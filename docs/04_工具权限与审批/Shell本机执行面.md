# Shell 本机执行面

## 权威范围

本文唯一定义内置 Shell 在本机执行什么、硬门禁、进程与退出状态、输出处理和安全限制。TaskScope 的 typed path 边界不适用于 Shell OS 隔离；文件改动的首选 typed 工具见 [Typed 文件工具与可恢复删除](Typed文件工具与可恢复删除.md)。

## 产品能力

Shell 使用当前 Mac/Windows 本机环境执行命令，可访问 OS 用户本来可访问的文件、网络、包管理器、Git remote、脚本和外部程序。它不是容器、VM 或内核 sandbox。execution profile、命令分类和 intent 用于策略与可见性，不能保证任意脚本被文件系统隔离。

代码 owner：[`src/tools/shell/shell.go`](../../src/tools/shell/shell.go)、[`src/tools/shell/policy.go`](../../src/tools/shell/policy.go)、[`src/tools/shell/executor.go`](../../src/tools/shell/executor.go) 以及 [`src/agentcore/executors/shell.go`](../../src/agentcore/executors/shell.go)。

## 当前硬门禁

执行前策略和执行器都复核：

- 直接删除命令 `rm`、`rmdir`、`unlink`、`del`、`erase`、`Remove-Item`、`ri`、`rd` 拒绝，要求使用 `trash_file`；
- 明显群发消息 CLI 先进入专用 mass-message review；当前执行器复核仍会拒绝该命令，因此 Shell 群发批准后执行尚未接通；
- 直接 `soffice`/`libreoffice` 及已知 wrapper 拒绝，要求使用受管 Office typed 工具；
- 明确诊断如 `command -v`、`rg`、`echo` 仍可执行，不能因字符串包含工具名称就误判为启动。

Shell 字符串解析能阻断已知直接形式，但不是完备的程序分析或 OS sandbox。编码命令、自定义解释器、动态下载脚本或未知 wrapper 可能绕过字符串识别；因此“禁止 direct delete/Office”不能被扩写为“绝对阻断任意脚本间接执行”。高保证能力必须迁移到 typed executor 或真正的 OS sandbox 后再宣称。

## 执行与环境

ShellExecutor 当前没有调用 `tool/sandbox.ApplySandbox`。它按 execution profile 选择最小/继承环境策略并记录 sandbox support，但不会把 TaskScope roots 施加为 OS 文件边界。命令 cwd、环境、timeout 和输出限制必须进入 canonical outcome。

命令分类/tier 是风险与观测元数据，不是最终授权。默认 policy 当前也不会自动把写文件 shell 变成 `text_edit`/`text_file_create`；模型应优先选 typed 工具，若仍选择 Shell，则执行真实 Shell 语义。

## 结果映射

- exit 0：process exited，通常 semantic success。
- 非零退出：通常 failure，并保存 exit code/stderr 摘要。
- timeout：process timed_out；`exec.CommandContext` 取消直接 shell 进程后返回 failure。当前没有显式 process-group 管理，不能保证所有后代进程同时退出。
- spawn 失败：process spawn_failed。
- 独立 `rg`/`grep`、`diff`、`test`/`[` 的 exit 1 可表示“未命中/有差异/条件 false”，映射为 neutral，而不是执行器故障。

stdout/stderr 有界截断；命令活动会对已识别的 `sk-`、`ak-`、`Bearer` 形式做启发式遮蔽，模型投影也会遮蔽结构化 secret-bearing key。但自由文本 stdout/stderr 当前不会做通用 secret scrub，canonical outcome/journal 仍可能保存命令打印出的敏感文本。不得执行会把凭据输出到终端的命令，也不能把现有遮蔽写成完整防泄漏保证。

是否产生文件变化只能以工具可观察证据记录，不能仅凭命令字符串推断全部 side effects。

## 异常与恢复

- 命令被硬门禁拒绝：不启动进程，返回 denied/对应专用 gate 状态和替代工具提示。
- 取消/timeout：取消直接 shell 进程；后代进程是否退出和其副作用保持 unknown。
- 非零退出：提供有界 stderr、exit code 和可行动 next action；FailureSupervisor 决定是否允许有限重试。
- 输出截断：显式 warning，不能展示截断内容却标为“完整”。

## 当前不支持与禁止表述

- 不支持 OS 级 workspace sandbox。
- 不保证网络隔离、外部目录只读或任意脚本绝对不可删除。
- 不支持可靠的 shell process-group 全树终止，也不提供自由文本 stdout/stderr 的完整秘密清洗。
- 不默认自动 transform 为 typed file chain。
- 不支持通过 Shell 完成群发消息；当前 review 可以收集决定，但 executor 仍 fail closed。
- `sudo` 能否成功取决于本机 OS/凭据；Awaken 不提供隐式提权。
- 不得把 profile 名称或 tier 当成安全证明。

## 测试与真实 Desktop 验收

自动化 owner：[`src/tools/shell/shell_test.go`](../../src/tools/shell/shell_test.go)、[`src/tools/shell/executor_test.go`](../../src/tools/shell/executor_test.go)、[`src/tools/shell/tier_test.go`](../../src/tools/shell/tier_test.go)、[`src/agentcore/shell_outcome_integration_test.go`](../../src/agentcore/shell_outcome_integration_test.go)、[`src/agentcore/shell_transform_test.go`](../../src/agentcore/shell_transform_test.go)。

验收使用隔离目录覆盖 exit 0/1/非零、stdout/stderr、timeout、取消、直接删除拒绝、Office direct 拒绝和诊断例外。群发审核只能使用不会真实发送的 fixture。真实 Desktop 应显示本机执行风险与真实终态；不得用测试字符串去碰用户消息、凭据或生产文件。
