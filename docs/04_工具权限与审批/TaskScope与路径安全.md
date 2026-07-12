# TaskScope 与路径安全

## 1. 当前支持

每个 run 固定 `WorkspaceRoot`、`WorkingDirectory`、`WritableRoots`、`OutputRoots`、显式附件/可读文件和敏感/`ProtectedRoots` 策略。相对路径从 workspace root 解析；typed 文件写入受 `WritableRoots` 约束，产生交付物的 typed provider 另外拥有 `OutputRoots` 契约，当前未完全统一路径 resolver 的 Artifact 例外由 [Artifact 与演示文稿工具契约](Artifact与演示文稿工具契约.md) 明确列为缺口。读取可覆盖 workspace 和用户显式附加的文件。

当 run 显式注入 `WorkspaceExecutionBroker` 时，Shell 从 normalized `PermissionContext` 产生 `workspace_scope_declared=true`、相对真实 workspace 的 canonical `writable_roots`/`protected_roots`。Writable roots 先解析 symlink，workspace 外的 root 不进入 workspace-local 可发布集；protected roots 同时保留 workspace 内 lexical 位置和 resolved target，防止删掉 protected symlink 本身或经别名改写 target。默认 `LocalUserExecutionBroker` 不启用该 workspace 发布门禁。

路径判断使用 lexical path 与 realpath/祖先检查：

- 拒绝通过现有叶子或祖先 symlink 逃逸写根；
- 允许 writable root 内尚不存在的叶子；
- 显示名映射到附件时必须唯一，歧义即失败；
- home、根目录、credential、SSH、Git 配置等敏感位置有独立 hard gate；
- recoverable delete 额外拒绝 symlink、特殊文件、根/受保护路径。

WorkspaceRoot 是 typed 文件/Artifact/provider 的默认 run 范围，不是当前登录用户 raw Shell 的操作系统边界。Shell 可指定当前用户可进入的 workspace 外 cwd，也可读写 Skill root/用户 cache；其 receipt 不声称这些副作用经过 TaskScope 观测或提交。外部 provider 的远程资源不伪装成本地路径；其 scope 由 provider manifest 与 typed input/output 契约表达。

## 2. 读、写、删除

| 操作 | 当前边界 |
| --- | --- |
| 读 | workspace + 显式附件/可读文件；敏感路径仍可拒绝 |
| 新建/覆盖/patch | 仅 writable roots，父路径和 symlink 链校验 |
| 删除 | 仅明确普通文件/目录，且必须由 recoverable-delete 路径处理 |
| 默认 Shell | cwd 可在 workspace 外，以当前登录用户的操作系统权限直接执行；TaskScope 只作运行上下文，不冒充全部副作用门禁 |
| 显式 Workspace broker Shell | cwd/workspace 和每个 staged delta 依据 canonical writable/protected roots 验证，Trash/发布前 fail closed |
| typed Git clone | destination 必须是 writable TaskScope 内非敏感、非 workspace root 的相对路径，父目录必须已存在；broker 的可发布集合固定为该 destination，覆盖前版本进入 Trash |

## 3. 不变量

- Renderer 传入的 path、Skill root 或 MCP metadata 不是授权事实。
- lexical `HasPrefix` 不能作为路径边界；必须使用平台路径规范化和祖先/realpath 校验。
- Workspace broker 顺序必须是 seal -> scan staged -> build plan -> validate protected/writable -> recheck live concurrent state -> Trash recovery -> publish。任一 delta 越界时整批 deny，在 Trash 和 live publish 前返回 `denied_writable_scope` 或 `denied_protected_root`、`side_effect_state=none`、`live_workspace_unchanged` 和 `trash_not_invoked`。
- `workspace_scope_declared=false` 且 roots 缺省的 legacy/restricted broker caller 保留“workspace root”兼容默认；显式 Workspace broker Shell 始终 declared。declared=true 但 canonical writable set 为空时表示 workspace 内零可发布路径，不得回落扩大到 workspace。默认 LocalUser broker 不进入该 publish scope。
- Workspace Shell broker 将 `WritableRoots` 与 `ProtectedRoots` 用于提交门禁，不单独使用 `OutputRoots`；要求交付物位于更窄 output root 时应使用 typed Artifact/file provider 的输出契约。
- `git_remote ls_remote` 显式声明零 writable roots；`git_remote clone` 不继承整个 workspace 的可写范围，而是把 broker request 再收窄为唯一 canonical destination。remote checkout 产生 destination 外 delta 或 symlink 时整批拒绝。
- 写失败、删除失败或副作用未知时不得把目标状态写成已完成。
- Windows 盘符、UNC、反斜杠、大小写与非 ASCII 路径需要平台测试，不能从 Unix fixture 推断。

## 4. 代码 owner

- `src/tool/permission.go`、`sensitive.go`：TaskScope 与敏感路径。
- 各 typed tool：输入 schema 与具体路径类型。
- `src/tool/execution_broker.go`、`execution_broker_workspace.go`：Shell workspace scope request、canonical roots 与提交前全量 delta 门禁。
- `src/tool/trash`：平台 recoverable-delete adapter。
