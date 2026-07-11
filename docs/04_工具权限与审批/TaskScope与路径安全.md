# TaskScope 与路径安全

## 权威范围

本文唯一定义 TaskScope、ExecutionProfile、路径解析、敏感/受保护路径与 typed mutation 边界。Shell 的本机执行面另见 [Shell 本机执行面](Shell本机执行面.md)；具体文件工具行为见 [Typed 文件工具与可恢复删除](Typed文件工具与可恢复删除.md)。

## TaskScope

HTTP/runtime authority 是 [`src/runtime/run_service.go`](../../src/runtime/run_service.go)；传入 Agent 后由 [`src/agentcore/types.go`](../../src/agentcore/types.go) 保存已规范化的路径部分。当前请求字段：

```text
scope_id
workspace_root
readable_roots
writable_roots
output_roots
attached_files
protected_roots
temporary_work_root
execution_profile
```

runtime 的 `TaskScope` 使用 `workspace_root` 并携带 `execution_profile`；Agent 内部的 `TaskScope` 将根目录保存为 `Root`，profile 则进入 `AgentConfig.ExecutionProfile`。这是同一次规范化的两层形态，不是两份授权来源。

`workspace_root` 必须是绝对路径。root 列表未提供时，默认以 workspace 建立可读/可写/输出边界；显式绝对 roots 当前可以位于 workspace 外，normalize 不会自动把它们夹回 workspace。调用方因此必须把 scope 看成授权输入，不能把任意 renderer 字符串当可信扩权。

## ExecutionProfile 来源

[`src/tool/profile/registry.go`](../../src/tool/profile/registry.go) 与 [`src/tool/profile/profile.go`](../../src/tool/profile/profile.go) 拥有 profile：

- 普通客户端只允许内置 `workspace_read_only`、`workspace_write`、`open_world_restricted`；
- `guardian` 由服务端选择；
- `danger` 仅在 `AWAKEN_ALLOW_DANGER_PROFILE=true` 的显式开发环境开放；
- 客户端篡改 profile 内部字段时，runtime 用 registry canonical profile 替换，不信任同名自定义内容。

profile 描述执行策略，但没有一种普通 profile 能把敏感路径、符号链接身份或专用安全 gate 全部关闭。

## 可读路径的当前真实边界

[`src/tool/permission.go`](../../src/tool/permission.go) 的本地只读解析支持绝对路径、workspace 相对路径和 attachment display/basename；同名 attachment 会报歧义。

当前 `ResolveReadablePathChecked` 不强制 `ReadableRoots`，本地 read/list/grep 可读取进程 OS 权限允许的绝对路径。`ReadableRoots` 仍进入 intent/诊断，但不是硬隔离。任何需求若需要真正的只读根隔离，必须先修改实现和测试，不能只改文档声称已有。

## 强校验 typed mutation 路径

`text_edit`、`text_file_create`、`apply_patch` 等采用 `ResolveMutationPathChecked` / `ResolveOutputMutationPathChecked` 的链路执行：

1. 规范化 lexical path 并要求绝对/可解析身份。
2. 分别检查 writable roots 或 output roots。
3. 找到现存最近祖先并解析 symlink，复验 real path 仍在授权 root。
4. 拒绝敏感、protected root 或路径穿越。
5. 工具自身在真正 open/rename 前再次校验关键身份。

默认受保护项包括 `.git`、`.awaken`、`.vscode`、`.idea`，`.env*`，证书/私钥/keystore 后缀，`credentials`、`secrets`、常见 SSH key basename，以及用户 shell profile/agent 配置目录。列表由代码拥有；新增敏感类别必须有正反例测试。

## 特例与当前缺口

- `trash_file` 有意支持项目内外文件，并不受 writable roots 限制。当前 intent path extractor 不提取它的 `path`，`FileChainExecutor` 因空 `Paths` 跳过通用 mutation check；工具自身又只做绝对路径解析，没有调用 sensitive/protected 或 symlink-identity resolver。因此它目前只依赖平台 trash adapter 的可恢复移动，缺少其他 typed mutation 的路径硬门禁。这是已确认安全缺口，不能写成“仍受 TaskScope 保护”。
- artifact runtime中的部分写入/导出工具同样没有走上述强resolver。`artifact_export` 的permission/execution只使用 `ResolveWritablePathChecked` / `ResolveOutputPathChecked` 做root包含检查，随后可直接 `os.Create` 覆盖目标；它不拒绝workspace内的 `.git`、`.env`、protected root或symlink identity。`ArtifactWrite` 当前默认auto-allow，现有测试只覆盖root边界。这是比“producer已受全部typed mutation防线”更宽的当前安全缺口。
- ShellExecutor 当前不应用 `tool/sandbox` 的 `ApplySandbox`。TaskScope 不能被宣传为 Shell 的 OS 文件系统隔离。
- 网络、MCP/connector resource scope 由各来源域和 profile policy 管理；TaskScope 当前没有 `connector_resources` 字段。

## 不变量

- 对 `text_edit`、`text_file_create`、`apply_patch`，lexical root 命中不足以授权，必须复验真实祖先和symlink后身份；`trash_file` 和部分artifact runtime写入当前不满足这条不变量，属于明确缺口。
- 输出发布同时满足 writable/output 合同；“可写”不自动等于“可作为最终交付物发布”。
- 附件路径是输入能力，不自动加入 writable roots。
- 客户端只能选择 allowlist profile 名称，不能自定义字段扩权。
- Shell 与本地只读工具的宽边界必须在产品中诚实呈现，不能用“workspace sandbox”概括。

## 测试与 Desktop 验收

自动化 owner：[`src/tool/permission_test.go`](../../src/tool/permission_test.go)、[`src/tool/profile/context_test.go`](../../src/tool/profile/context_test.go)、[`src/tool/profile/profile_test.go`](../../src/tool/profile/profile_test.go)、[`src/tool/sensitive_test.go`](../../src/tool/sensitive_test.go)、各 typed 工具 symlink/path 测试。

验收矩阵至少包含：workspace 内外读、workspace 内写、外部显式 writable root、output root、attachment basename 歧义、现存/不存在目标的 symlink escape、敏感文件、篡改 profile、danger 未开启。artifact runtime还必须补 `.git/.env/protected/symlink` 反例；补齐前不得把现有root测试当成完整typed安全通过。真实 Desktop 使用隔离 workspace；不得为验证边界触碰用户真实凭据或配置。
