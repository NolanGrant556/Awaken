# Shell 本机执行面设计

## 文档定位

本文档定义 Awaken 当前 shell 工具的正式产品契约。它补充：

- `docs/04_工具权限与审批/当前规范/全量无审核工具运行时落地方案.md`
- `docs/04_工具权限与审批/当前规范/工具平台规范.md`

当前结论：shell 是“本机执行面”，不是“项目文件夹沙箱”。Agent 可以通过 shell 读取、写入和操作项目内外的本机路径，并运行本机已有命令、脚本、包管理器、网络命令和系统命令。Awaken 不再用 `readable_roots`、`writable_roots`、sandbox support、open-world 分类或模型 endpoint 能力来阻断 shell。

## 唯二硬约束

shell 只保留两个产品级硬约束：

1. 删除必须进入系统回收站。
2. 工具群发消息必须触发人工审核。

除此以外，shell 不做项目边界、敏感路径、网络、依赖安装、sudo、chmod、脚本执行、HOME 读取、全盘搜索等产品层硬拦截。真正是否能执行，由操作系统权限、命令自身权限、账号授权、runtime 是否存在以及命令退出码决定。

## 边界拆分

### shell

shell 面向本机：

- 可以访问项目内路径。
- 可以访问项目外路径。
- 可以读取用户选择的附件原始路径。
- 可以运行本机 CLI、脚本、包管理器、构建工具、网络命令和系统命令。
- 可以写入项目外路径，只要操作系统允许。
- 不因为 sandbox 缺失而 fail-closed。

### typed file / artifact tools

`apply_patch`、artifact tools、connector file bridge 等结构化工具仍可以保留自己的 TaskScope 规则。它们不是 shell 的权限边界。

如果某个结构化工具只支持 workspace/output roots，这只是该工具自己的输入契约；Agent 仍可在需要时改用 shell 或更合适的本地工具处理项目外文件。

### 删除工具

删除需求不能用 shell 里的永久删除命令完成。Awaken 提供 `trash_file` 作为本机文件/文件夹删除入口：

```text
trash_file(path) -> TrashAdapter.TrashPath(path)
```

`trash_file` 可以处理项目内和项目外路径。平台 TrashAdapter 失败时必须返回错误，不得回退到永久删除。

## 决策链路

shell 调用按以下顺序处理：

```text
ShellTool.CheckPermission
  -> 群发消息检测
  -> ShellPolicy hard deny 检测
  -> PermissionAllow
  -> AutoToolPolicy allow
  -> ShellExecutor 执行前再次 hard deny
  -> 本机 shell 执行
```

`ShellExecutor` 执行前重复检查 hard deny，避免绕过 `CheckPermission` 直接执行。

## 删除拦截

shell hard deny 只拦截直接删除命令，包括：

- `rm`
- `rmdir`
- `unlink`
- `del`
- `erase`
- `Remove-Item`
- `ri`
- `rd`

命中后返回结构化拒绝，提示改用 `trash_file`。这条规则对项目内、项目外、文件、目录都一样。

不再尝试解析 `rm` 的参数是否安全；只要是直接删除命令，一律不走 shell。

## 群发消息审核

shell 会识别明显的协作 CLI 群发形态，例如：

- `feishu message send ...`
- `lark-cli im +messages-send ...`
- `dws ... robot send ...`
- 包含 `broadcast`、`bulk`、`mass`、`群发` 等批量发送语义的命令

命中后返回 `needs_mass_message_review`。审核通过前不得调用真实发送接口。

执行前策略也会再次拒绝群发 CLI 直跑，避免绕过审核 gate。

## 沙箱与环境

shell 不再依赖 sandbox 作为能否执行的判定：

- `SandboxFull`、`SandboxDegraded`、`SandboxNone` 只作为观测信息保留。
- `ShellExecutor` 不再因为 open-world/system 命令缺 sandbox 而返回 `sandbox_required`。
- `ShellExecutor` 不再对本机 shell 命令调用 `ApplySandbox`。

`EnvPolicy` 仍可影响子进程环境变量暴露方式：

- `EnvInherit`：继承父进程环境，并补充常见用户 CLI 路径。
- `EnvMinimal`：使用最小环境变量白名单。

这只是环境暴露策略，不是 shell 权限边界。

## transform 关系

shell 不再强制把文件写入 transform 到 `apply_patch`。简单 shell 写入、重定向、脚本、`sed -i`、`cp`、`mv` 等都可以作为本机命令直接执行。

`TransformShellToFileChain` 可以继续作为辅助能力保留，用于某些场景下生成更清晰的 patch，但它不再是 auto policy 主路径。

## 附件与本地路径

附件系统应生成本机文件引用。对于文档、代码、表格、PDF 等文件，Agent 可以通过本地工具或 shell 按需读取这些路径；不要求文件先复制进项目文件夹。

图片可以作为视觉输入进入模型；文档类附件始终按本机文件读取处理。附件是否位于当前 workspace 不影响 shell 读取。

## ToolOutcome 语义（Phase 1）

`ShellTool` 必须实现 `OutcomeTool`。输出保留 `command`、`cwd`、`exitCode`、`stdout`、`stderr`、`duration`、`truncated`；command 与 activity detail 做 secret redaction。

固定映射：

| 场景 | Status | TransportStatus | SemanticStatus | ProcessStatus | ErrorCode |
|---|---|---|---|---|---|
| exit 0 | succeeded | completed | success | exited | 空 |
| 普通非零 | failed | completed | failure | exited | process_exit_nonzero |
| timeout | failed | completed 或 failed | failure | timed_out | process_timeout |
| spawn 失败 | failed | failed | failure | spawn_failed | process_spawn_failed |
| 单一 rg/grep exit 1 | succeeded | completed | neutral | exited | 空；Diagnostic.reason=no_match |
| 单一 diff exit 1 | succeeded | completed | neutral | exited | 空；Diagnostic.reason=different |
| 单一 test/`[` exit 1 | succeeded | completed | neutral | exited | 空；Diagnostic.reason=false_condition |
| 复合命令未知语义非零 | failed | completed | failure | exited | process_exit_nonzero |

宿主活动：识别 `pip` / `python -m pip` / `npm` / `pnpm` / `yarn` / `brew` / `winget` install，生成“正在安装依赖”类标题，不依赖模型口述。

## 审计与 UI

shell 执行仍应记录：

- command
- workdir
- tier/classification
- sandbox support 观测值
- stdout/stderr 截断状态
- exit code
- ToolOutcome 状态字段
- denied reason
- mass message review metadata

UI 不展示通用审批卡。普通 shell 命令显示为自动执行；直接删除显示结构化拒绝；群发显示专用人工审核。非零退出必须显示 failed 与 exitCode，不得因 content 非空而显示成功。

## 验收用例

应允许：

- `cat ~/.ssh/id_rsa`
- `find / -type f`
- `cat README.md | sh`
- `sudo true`
- `chmod -R 755 /tmp/example`
- `curl https://example.com`
- `npm install`
- `git push`
- `echo hi > /tmp/awaken-out.txt`

应拒绝并提示改用回收站工具：

- `rm -rf build`
- `rmdir build`
- `unlink file.txt`
- `del file.txt`
- `Remove-Item file.txt`

应进入群发人工审核：

- `feishu message send --chat-id all --text hello`
- `dws message broadcast --text hello`

应通过 `trash_file` 完成：

- 删除项目内文件。
- 删除项目外文件。
- 删除项目内目录。
- 删除项目外目录。

## 关键结论

当前 Awaken shell 不是受 workspace 沙箱约束的窄工具，而是本机自由执行能力。安全产品契约只保留两条：删除必须可恢复，群发必须人工审核。
