# Shell 内置权限执行器设计

## 文档定位

本文档细化 Awaken 在“无通用审核”目标下的 shell 设计。它补充以下文档中的原则，但更聚焦可实现的执行器、命令分层、权限来源和验收标准：

- `docs/04_工具权限与审批/当前规范/全量无审核工具运行时落地方案.md`
- `docs/04_工具权限与审批/当前规范/工具平台规范.md`
- `docs/04_工具权限与审批/当前规范/零审批自动执行策略与落地路线.md`

目标不是把 shell 变成无边界终端，而是让用户不再被通用审批打断，同时把权限能力内置到 shell 工具、执行器、profile、sandbox、TaskScope 和 audit 中。

MCP 工具不纳入本文实施范围。本文只处理 Awaken 内置 shell 与本地工程命令链。

## 调研结论

### Claude Code

Claude Code 的 Bash 不是免审批全能 shell。它的设计是：

- prompt 明确要求搜索、读取、编辑、写入优先走窄工具，不鼓励 Bash 代替全部能力。
- Bash 自身有 command parser、allow/deny/ask rules、危险命令识别、路径校验、sandbox、headless 默认策略。
- 即使某些模式允许绕过交互提示，仍存在 safety check 与敏感路径保护。
- 后台命令有输出上限、卡住检测、任务管理和后续读取通道。

本机源码证据入口：

- `/Users/nolangrant/claudecode源代码/src/tools/BashTool/BashTool.tsx`
- `/Users/nolangrant/claudecode源代码/src/tools/BashTool/bashPermissions.ts`
- `/Users/nolangrant/claudecode源代码/src/tools/BashTool/pathValidation.ts`
- `/Users/nolangrant/claudecode源代码/src/utils/permissions/filesystem.ts`
- `/Users/nolangrant/claudecode源代码/src/utils/sandbox/sandbox-adapter.ts`

### Codex

Codex 的 `approval_policy=never` 也不是让 shell 自动升级成全权限。它的设计是：

- shell/exec surface 只是命令入口，真正边界在 permission profile、sandbox permissions、exec policy、tool orchestrator。
- `approval_policy=never` 下模型不能请求 `sandbox_permissions` 升级；危险或越界命令会被拒绝或被 sandbox 拦住。
- file edit/delete 与 shell 分开，`apply_patch` 是独立 grammar 和 runtime。
- safe command classifier 很窄，主要覆盖只读命令；危险命令单独识别。

本机源码证据入口：

- `/Users/nolangrant/Codex 源代码/codex-rs/core/src/tools/handlers/shell_spec.rs`
- `/Users/nolangrant/Codex 源代码/codex-rs/core/src/tools/handlers/unified_exec/exec_command.rs`
- `/Users/nolangrant/Codex 源代码/codex-rs/core/src/tools/orchestrator.rs`
- `/Users/nolangrant/Codex 源代码/codex-rs/core/src/exec_policy.rs`
- `/Users/nolangrant/Codex 源代码/codex-rs/shell-command/src/command_safety/is_safe_command.rs`
- `/Users/nolangrant/Codex 源代码/codex-rs/shell-command/src/command_safety/is_dangerous_command.rs`

## Awaken 当前状态

Awaken 当前已经有 shell 内置权限执行器的雏形：

- `src/tools/shell/shell.go` 已把 shell 标记为 destructive/open-world，并通过 `CheckPermission` 产出风险元数据。
- `src/tools/shell/tier.go` 已有 `inspect_read_only`、`local_cli`、`workspace_mutation`、`open_world_system` 分类。
- `src/agentcore/shell_transform.go` 已能把简单 `echo > file` 转成 `apply_patch`。
- `src/tools/shell/executor.go` 已有 `ShellExecutor`，会在 open-world 且缺少可信 sandbox 时 fail-closed。
- `src/tool/profile/profile.go` 已有 `ExecutionProfile`，覆盖 permission、sandbox、network、env、visibility。
- `src/tool/sandbox/sandbox_mac.go` 已有 macOS sandbox adapter。

当前缺口也很清楚：

- shell 分类仍需持续细化，但未知本地 CLI 不应再因为 Awaken 不认识命令而被误判为工作区写入。
- transform 只覆盖简单 `echo > file`，`mkdir`、`touch`、`cp`、`mv`、`rm`、测试、构建、格式化、代码生成没有稳定 lane。
- macOS sandbox 当前主要限制写和网络，但仍允许全盘读；因此读取 HOME、SSH key、shell rc、凭证文件必须由 shell 权限层额外保护。`local_cli` 例外：用户已安装 CLI 需要正常访问 Keychain、系统证书信任链、用户级 cache/token 目录和网络，macOS 下不套 `sandbox-exec`，由 shell policy 继续兜住删除、危险 wrapper、敏感路径和群发审核。
- Windows 当前 sandbox adapter 是 degraded：`inspect_read_only`、`local_cli`、`workspace_mutation` 不提供等价 AppContainer/Job Object 进程边界，继续依赖 shell policy、权限层、敏感路径保护和审计兜底；`open_world_system` 在缺少可信 sandbox 时 fail-closed。Windows 本地 CLI 通过子进程 `PATH` 补充 `%APPDATA%\npm`、`%LOCALAPPDATA%\pnpm`、`%LOCALAPPDATA%\Microsoft\WindowsApps`、`%USERPROFILE%\.local\bin`、`%USERPROFILE%\scoop\shims` 等常见用户安装目录。
- `workspace_mutation` 被识别后缺少受控执行器承接，容易表现为拒绝。
- network、dependency install、git clone、curl 等命令还没有项目 profile 级授权模型。

## 产品原则

### 1. 无审核不等于裸 shell

Awaken 不再让用户在任务中途审批普通工具调用。但这不意味着 shell 可以无限制执行。

shell 的最终决策只能是：

- `allow`
- `deny`
- `transform`
- `needs_runtime`
- `unsupported`

不允许 shell 触发通用人工审核。人工审核只保留给 `mass_message_send`。

### 2. 权限来自系统配置，不来自模型请求

LLM 调用 shell 时不得传入临时扩权参数，例如：

- `require_escalated`
- `dangerouslyDisableSandbox`
- `allow_unsandboxed`
- `full_access`
- `extra_writable_roots`
- `network_open`

这些能力只能来自：

- 用户或项目预配置的 `ExecutionProfile`
- 当前 run 的 `TaskScope`
- 官方 runtime 的能力包
- 明确的连接器授权
- 明确的高信任会话入口

模型只能表达命令意图，不能自行扩大边界。

### 3. shell 是命令执行器，不是文件编辑主路径

普通文件增删改移动优先走文件链。shell 中识别到可稳定表达为文件链的意图时，必须 `transform`。

典型 transform：

- `echo ... > file` -> `apply_patch`
- `cat > file <<EOF ... EOF` -> `apply_patch`
- `mkdir -p path` -> `workspace_fs`
- `touch file` -> `workspace_fs`
- `cp a b` -> `workspace_fs`
- `mv a b` -> `workspace_fs`
- `rm path` -> `workspace_fs` + `TrashAdapter`

### 4. deny 优先于 allow

规则匹配顺序必须是：

1. hard deny
2. sensitive path deny
3. parser unsafe deny
4. transform
5. profile allow
6. command family allow
7. 未识别但满足单命令本地 CLI 形态 -> `local_cli`
8. default deny

allow 不能覆盖敏感路径、危险 wrapper、系统路径写入、凭证读取、永久删除、网络越界。`local_cli` 是“本地进程执行边界”，不是业务功能白名单；Awaken 不枚举每个 CLI 的子命令功能。

## Shell Lane 设计

### Lane 1：只读检查

名称：`inspect_read_only`

用途：

- 查看当前目录、文件列表、项目结构。
- 搜索代码和文本。
- 查看 git 状态和 diff。
- 查看有限的文件片段。

允许命令族示例：

- `pwd`
- `ls`
- `find .`
- `rg`
- `grep`
- `cat`
- `head`
- `tail`
- `wc`
- `git status`
- `git diff`
- `git log`
- `git show`

约束：

- 默认只能读取 `TaskScope.readable_roots`。
- 读取 HOME、SSH key、shell rc、token、证书、浏览器配置、agent 配置目录直接 deny。
- 只读命令也不能访问网络。
- 命令输出必须有截断上限。
- 不允许通过管道连接执行器，例如 `cat file | sh`。

典型决策：

- `rg "foo" src` -> `allow`
- `git diff` -> `allow`
- `cat ~/.ssh/id_rsa` -> `deny`
- `find / -type f` -> `deny`

### Lane 2：本地 CLI

名称：`local_cli`

用途：

- 调用本机已安装的任意 CLI，例如 `dws`、`gh`、`feishu`、`obsidian`。
- 让 skill 能通过 CLI 自身的 `help`、`schema`、`list`、`search`、`get` 等能力完成真实工作。
- 避免“Awaken 不认识这个命令 -> 误判为 workspace mutation -> 拒绝”的问题。

允许命令形态：

- 单个本机可执行命令。
- 可以带普通参数、URL、查询词、JSON 字符串参数。
- 可以访问网络，由 CLI 自己和用户已有认证决定业务权限。
- 执行器会在子进程 PATH 中补充常见用户 CLI 目录，例如 `~/.local/bin`、`/opt/homebrew/bin`、`/usr/local/bin`，避免 GUI 启动的 Desktop 因 PATH 过窄找不到本机 CLI。
- Windows 下同样会补充常见用户 CLI 目录，例如 `%APPDATA%\npm`、`%LOCALAPPDATA%\pnpm`、`%LOCALAPPDATA%\Microsoft\WindowsApps`、`%USERPROFILE%\.local\bin`、`%USERPROFILE%\scoop\shims`，并兼容 `Path`/`PATH` 环境变量大小写差异。
- macOS 下不使用 `sandbox-exec` 包裹 `local_cli`，否则会破坏 Keychain/token/cache、系统证书信任链与 CLI 自身状态管理。
- Windows 下 `local_cli` 当前不宣称完整 sandbox 等价能力；它是本地 CLI 执行通道，安全兜底来自命令策略、敏感路径保护、删除拦截、群发审核和执行审计。需要开放 `open_world_system` 前必须补 AppContainer/Job Object 等可信进程隔离。

约束：

- 不允许 `sh -c`、`eval`、`xargs`、把数据管入执行器等 shell wrapper 绕过工具侧判断。
- 不允许写系统路径、读取敏感 HOME 文件或触碰 protected roots。
- 文件重定向写入工作区时提升为 `workspace_mutation`，优先 transform 到文件链。
- 本地文件删除仍然必须走 `TrashAdapter`，不能用 `rm/rmdir` 物理删除。
- 群发、批量通知、机器人广播等外部大范围消息必须进入人工审核；shell 会对明显的协作 CLI 群发形态（如 `lark-cli im +messages-send`、`feishu message send`、`dws ... robot send`、`... broadcast/bulk/mass ...`）返回 `massMessage` review，直接执行路径则 fail-closed。
- 输出、超时、审计必须保留。

典型决策：

- `dws aitable base search --query "DVF" --format json` -> `allow`
- `gh issue list --limit 10` -> `allow`
- `obsidian note search "ERP"` -> `allow`
- `dws schema` -> `allow`
- `dws schema > schema.json` -> `transform/deny`，按文件写入边界处理
- `rm -rf reports` -> `deny`，提示走回收站删除工具

### Lane 3：工作区文件操作

名称：`workspace_fs`

用途：

- 新建目录。
- 新建空文件。
- 复制、移动、删除工作区内文件。
- 处理 shell 中稳定可解析的文件重定向。

不直接执行裸 shell。命中该 lane 后必须 transform 到 typed file executor。

允许意图：

- `mkdir -p <workspace-path>`
- `touch <workspace-file>`
- `cp <workspace-file> <workspace-path>`
- `mv <workspace-file> <workspace-path>`
- `rm <workspace-path>`
- `rm -r <workspace-dir>`

约束：

- 目标必须在 `writable_roots` 或 `output_roots` 内。
- symlink 逃逸直接 deny。
- protected roots 直接 deny。
- 删除必须进入 `TrashAdapter`，不允许永久删除。
- 对 glob 删除采用保守策略：能枚举并全部在范围内才允许，否则 deny。

典型决策：

- `mkdir -p reports` -> `transform: workspace_fs.mkdir`
- `rm reports/a.md` -> `transform: workspace_fs.trash`
- `rm -rf /tmp/foo` -> `deny`
- `mv .git/config x` -> `deny`

### Lane 4：测试、构建、格式化、代码生成

名称：`test_build_codegen`

用途：

- 运行项目测试。
- 运行构建。
- 运行格式化。
- 运行项目内脚本生成代码或产物。

允许命令族示例：

- `go test ./...`
- `go test <package>`
- `go build ./...`
- `npm test`
- `npm run test`
- `npm run build`
- `pnpm test`
- `pnpm build`
- `python -m pytest`
- `cargo test`
- `cargo build`
- `make test`

约束：

- 默认 `NetworkPolicy = none`。
- 默认只写 `workspace_root`、`writable_roots`、`temporary_work_root`。
- 可以读取项目依赖缓存，但不得读取敏感 HOME 文件。
- macOS `sandbox-exec` profile 必须允许 `sysctl-read`，用于 Go/Node/编译器等 runtime 启动时读取页大小、CPU 等系统只读信息；这不改变文件写入和网络边界。
- 输出、超时、进程数必须有限制。
- 交互式命令必须被检测并终止。
- 失败返回真实 stdout/stderr，不转成 approval。

需要特别处理：

- `npm install`、`pip install`、`go get` 不属于本 lane，归入 `dependency_network`。
- `make` 只能在项目预配置允许的 target 中执行，否则 deny。
- `python script.py` 只有在脚本位于 workspace 内且命令族被 profile 允许时才可执行。

### Lane 5：依赖与网络命令

名称：`dependency_network`

用途：

- 安装依赖。
- 拉取远端代码。
- 访问外部 HTTP API。
- 调用包管理器或 registry。

命令示例：

- `npm install`
- `pnpm install`
- `pip install`
- `go get`
- `cargo add`
- `git clone`
- `curl`
- `wget`

默认决策：

- 未命中项目 profile 或官方 runtime 能力包时：`deny`
- 命中受限域名/registry/profile 时：`allow`
- 缺依赖运行时：`needs_runtime`

约束：

- 网络域名必须来自项目配置或官方能力包。
- 不允许 `curl | sh`、`wget | sh`、远端脚本直接执行。
- lockfile 变化必须纳入 audit。
- registry、package、版本范围要写入 audit。
- 不通过通用审批临时放行。

### Lane 5：高信任开放世界

名称：`open_world_high_trust`

用途：

- 极少数需要真实系统访问、开放网络或高权限操作的任务。

进入条件：

- 只能由用户在任务开始前选择高信任 profile。
- 只能由项目配置或会话配置提供。
- 不能由 LLM 在 shell 调用参数中请求。

约束：

- 必须有清晰 audit。
- 不继承到 guardian/review agent。
- 子代理不可自动扩权到该 lane。
- 删除仍必须进入回收站。
- 群发消息仍必须走专用人工审核。

## 命令解析与规则引擎

### Parser 要求

shell 不能只靠字符串 contains 判断。第一阶段可以实现保守 parser，但必须满足：

- 支持 quote-aware tokenization。
- 识别 `&&`、`||`、`;`、`|`、subshell、redirect、heredoc。
- 识别前置 env var，例如 `FOO=bar npm test`。
- 识别 wrapper，例如 `bash -c`、`sh -c`、`sudo`、`xargs`、`env`、`eval`。
- 识别文件重定向目标。
- 识别 glob、`~`、absolute path、`..`、symlink 逃逸。
- 设定最大 subcommand 数，超过直接 deny。

### 规则类型

规则至少包含：

- `exact`：完整命令匹配。
- `prefix`：命令前缀匹配。
- `family`：命令族匹配，例如 `git.read_only`、`go.test`、`npm.build`。
- `path_rule`：路径范围和敏感路径规则。
- `network_rule`：域名、registry、protocol 规则。
- `profile_rule`：当前 `ExecutionProfile` 是否允许该 lane。

### 匹配顺序

规则必须先剥离绕过层再匹配：

1. 剥离前置 env var。
2. 展开或拒绝 wrapper。
3. 分析 redirect/heredoc。
4. 分析路径。
5. 判断 hard deny。
6. 判断 transform。
7. 判断 lane allow。

示例：

- `FOO=bar rm -rf .git` 仍应命中 hard deny。
- `bash -c "cat ~/.ssh/id_rsa"` 仍应命中 sensitive read deny。
- `cat file | sh` 仍应命中 pipe-to-executor deny。

## 敏感路径规则

shell 权限层必须补齐 sandbox 无法可靠表达的读保护。

默认拒绝读取或写入：

- `~/.ssh`
- `~/.gnupg`
- shell rc/profile，如 `.zshrc`、`.bashrc`、`.profile`
- `.env`、`.env.*`
- key、pem、p12、crt、cert、kubeconfig、token 文件
- 浏览器 profile 与 cookie 存储
- `.git`
- `.awaken`
- `.codex`
- `.claude`
- `.mcp.json`
- Awaken Desktop/runtime 本地配置目录
- 系统目录，如 `/etc`、`/usr`、`/bin`、`/sbin`、`/System`

敏感路径规则不能被项目 allowlist 覆盖。只有专门的高信任 profile 可以放宽部分读路径，但仍必须记录 audit，且默认不继承给 subagent。

## Profile 编译规则

`ExecutionProfile` 不应停留在文档概念，需要编译成 shell executor 可消费的 runtime profile。

建议结构：

```text
ShellExecutionProfile
├── allowed_lanes
├── readable_roots
├── writable_roots
├── output_roots
├── temporary_roots
├── protected_roots
├── allowed_command_families
├── denied_command_families
├── allowed_network_scopes
├── allowed_registries
├── env_policy
├── sandbox_policy
├── timeout_policy
└── output_policy
```

来源优先级：

1. 系统硬规则。
2. 用户或项目 profile。
3. 当前 TaskScope。
4. 官方 runtime 能力包。
5. 工具调用输入。

工具调用输入只能收窄，不得扩大。

## 执行器拆分

### ShellPolicyEngine

职责：

- 解析命令。
- 归类 lane。
- 计算 `allow`、`deny`、`transform`、`needs_runtime`、`unsupported`。
- 产出结构化原因。

### WorkspaceFsExecutor

职责：

- 执行 `mkdir`、`touch`、`cp`、`mv`、`trash`。
- 保证路径在 TaskScope 内。
- 删除走 `TrashAdapter`。

### TestBuildExecutor

职责：

- 执行测试、构建、格式化、代码生成。
- 继承 `ShellExecutor` 的 stdout/stderr、timeout、observer。
- 强制应用 profile、sandbox、env、network。

### DependencyNetworkExecutor

职责：

- 执行包管理器和网络下载类命令。
- 按受限 registry/domain 运行。
- 拒绝管道执行远端脚本。
- 记录 lockfile 和 manifest 变化。

### ShellExecutor

职责：

- 只执行 policy 已允许且无法 transform 的真实命令。
- 应用 sandbox。
- 应用 env policy。
- 应用 timeout 和 output cap。
- 记录 audit。

## 实施拆分

### 第一阶段：文档与契约

- 新增本文档。
- 在工具平台规范中声明 shell 不接受模型临时扩权。
- 在全量无审核方案 Phase 6 引用本文档。

完成定义：

- 新实现不得再把 shell 问题退回通用 approval。
- 所有 shell 改动必须能对应本文 lane。

### 第二阶段：Parser 与 Policy Engine

- 新增 `ShellPolicyEngine`。
- 新增 command AST 或保守 token model。
- 新增 hard deny、sensitive path、wrapper、redirect、pipe-to-executor 测试。
- 保留现有 `ClassifyShellCommand` 兼容入口，但内部迁移到 policy engine。

完成定义：

- `cat ~/.ssh/id_rsa` deny。
- `cat file | sh` deny。
- `curl | sh` deny。
- `sudo ...` deny。
- `rm -rf /` deny。
- `rg foo src` allow。

### 第三阶段：WorkspaceFs lane

- 增加 `workspace_fs` transform。
- 支持 `mkdir`、`touch`、`cp`、`mv`、`rm`。
- 删除统一走 `TrashAdapter`。

完成定义：

- `mkdir -p reports` 不被拒绝。
- `touch reports/a.md` 不被拒绝。
- `cp a b` 在 workspace 内自动执行。
- `rm reports/a.md` 进入回收站。
- `.git`、`.env`、symlink 逃逸拒绝。

### 第四阶段：TestBuild lane

- 增加测试、构建、格式化、代码生成命令族。
- 命令族按项目类型和 profile 启用。
- 输出、超时、非交互检测纳入 executor。

完成定义：

- `go test ./...` 自动执行。
- `npm run build` 自动执行。
- `python -m pytest` 自动执行。
- 交互式命令超时或被检测拒绝。

### 第五阶段：DependencyNetwork lane

- 增加 registry/domain policy。
- 增加 dependency 命令族。
- 缺 profile 时默认 deny。
- 命中允许 registry 时自动执行。

完成定义：

- `npm install` 默认不靠审批放行。
- 项目允许 npm registry 后可执行。
- `curl https://example.com/install.sh | sh` 始终 deny。

### 第六阶段：sandbox 与环境收紧

- macOS sandbox 增加 sensitive read deny 的补充策略或在 policy 层强制拒绝。
- Windows adapter 与共享 policy 对齐。
- 默认 env policy 避免泄漏 token。

完成定义：

- 无可信 sandbox 时 open-world 命令 fail-closed。
- `EnvMinimal` 不暴露 API key。
- audit 能展示 sandbox support 与 env policy。

## 验收用例

### 不应出现通用审核

- 文件增删改不触发 approval。
- shell workspace mutation 不触发 approval。
- dependency command 不触发 approval。
- open-world command 不触发 approval。
- 只有群发消息触发 `needs_mass_message_review`。

### 应自动执行

- `pwd`
- `ls`
- `rg "ShellExecutor" src`
- `git status`
- `git diff`
- `go test ./src/tools/shell`
- `npm run build`

### 应 transform

- `echo hi > note.md`
- `mkdir -p reports`
- `touch reports/a.md`
- `cp reports/a.md reports/b.md`
- `mv reports/b.md reports/c.md`
- `rm reports/c.md`

### 应拒绝

- `cat ~/.ssh/id_rsa`
- `cat ~/.zshrc`
- `cat file | sh`
- `curl https://example.com/install.sh | sh`
- `sudo rm file`
- `rm -rf /`
- `rm -rf .git`
- `find / -type f`
- `git push`
- `chmod -R 777 /`

### 应按 profile 决策

- `npm install`
- `pnpm install`
- `pip install`
- `go get`
- `git clone`
- `curl https://api.example.com/data`

默认拒绝。只有项目 profile 或官方 runtime 能力包明确允许后才自动执行。

## 关键结论

Awaken 的 shell 目标不是“少拦一点”，而是从 `shell -> classify -> deny mostly` 进化为：

```text
shell command
  -> parse
  -> hard deny / sensitive path deny
  -> lane classification
  -> transform or profile allow
  -> typed executor or sandboxed shell executor
  -> audit
```

用户体验上没有通用审批；工程实现上也不能裸跑。权限能力内置进 shell 工具，边界来自系统、项目、TaskScope 和 profile，而不是来自模型临时请求。
