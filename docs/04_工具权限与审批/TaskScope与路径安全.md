# TaskScope 与路径安全

## 1. 当前支持

**roots 决定默认在哪儿干活，不决定能不能干。** 每个 run 固定 `WorkspaceRoot`、`WorkingDirectory`、`ReadableRoots`、`WritableRoots`、`OutputRoots`、显式附件/可读文件和敏感/`ProtectedRoots`/`ProtectedRootExceptions` 策略，但这些 root 不再作为路径围栏：

- 相对路径按 `WorkspaceRoot`（主文件夹）解析，这是 roots 仍然承担的职责；
- 绝对路径指哪读哪、指哪写哪，落在项目文件夹之外同样执行，**不触发审批**；
- 不给 `path` 的搜索只覆盖项目文件夹，这是「默认优先」的体现，也避免一次无范围搜索遍历整块磁盘。

之所以不用 root 围栏：`Bash` 的 `CheckPermission` 从来不做路径检查，围栏只约束 `Read`/`Write`/`Edit`/`Glob`/`Grep`，因此它不构成边界，只会把越界工作赶进 shell 调用——那样反而丢掉结构化工具的 diff 记录与结果规范。

写入侧保留的是**按形状**判定的固定拒绝名单，与所在 root 无关：`.git` 内部、`.env`、私钥与凭据文件（`id_rsa`、`*.pem`、`credentials`、`secrets` 等）、home 下的 shell 配置、以及 `~/.awaken` 与运行时 config 目录。理由是写坏这些从来不是指令的本意，而 `~/.awaken` 承载 Awaken 自身的 store generations、config 与 credentials。**读取不套用这份名单**，与围栏时期项目内可读 `.env` 的行为保持一致。

三个 roots 数组本来就是多根的。一个项目有多个文件夹时，Desktop 用 `[主文件夹, ...次文件夹]` 填这三个数组，`WorkspaceRoot` 是主文件夹。线程锚定在次文件夹时，那个文件夹排在数组首位并成为该 run 的 `WorkspaceRoot`。

roots 要生效，模型必须知道它们存在。scope 通过 `workspace.project_id/project_name` 拿到稳定项目身份和展示名，system prompt 以结构化 `<environment_context>` 分开渲染项目 ID/名称、cwd、逐项保留的 `workspace_roots` 和独立的 unrestricted `permission_profile`。Host 不计算或注入多个 root 的共同父目录。默认系统提示的「项目文件夹与作业范围」再声明四条约定：默认在主文件夹作业、文件夹组是一个整体、“本项目”严格等于这组 roots、跨文件夹检索省略 `path`；项目外路径只有在用户明确点名时才进入显式路径模式。模型侧渲染由 [上下文窗口与压缩](../02_主智能体闭环/上下文窗口与压缩.md) 拥有；此处只声明它与 roots 语义必须一致。

项目可以没有任何文件夹。此时 Desktop 传 `workspace.project_id` 而 `workspace_root` 为空，Core 把它解析到该项目自己的 session 工作目录：`<AWAKEN_HOME>/sessions/<project_id>`，产出默认落在其下的 `output/`；同一零文件夹项目的 thread 共享这一区域。

独立对话同时省略 `workspace_root` 与 `project_id`。Core 必须先确定稳定 thread ID，再解析到 `<AWAKEN_HOME>/sessions/<thread_id>`，产出同样落在 `output/`。新独立对话由 Core 生成 thread ID，继续对话使用请求中的已有 `thread_id`；不回退到 Core 进程 cwd、用户主目录或 Renderer 记住的项目路径。managed workspace owner ID 只能包含安全的路径组件字符，不能用 `..` 或分隔符逃出 sessions 子树。

`ProtectedRoots` 按 **每个** root 生成 `.git` / `.awaken` / `.vscode` / `.idea` 保护项，不只为主文件夹生成；否则次文件夹仓库的 `.git` 会变成可写。

`ProtectedRootExceptions` 是受保护子树内部的窄口子，只为 session 工作目录存在：`~/.awaken` 整体受保护，而 session 目录在它下面。只有当本次 scope 的某个 root 落在 `<AWAKEN_HOME>/sessions` 里时才放行这一条子树，`config`、`credentials`、`store-generations`、`memory` 依旧不可写。例外内部仍然适用上述形状规则。

`Glob` 和 `Grep` 默认使用 `scope_mode=project`。调用方没有显式给 `path` 时覆盖全部 readable roots：cwd 在前，其余按 scope 顺序跟随，嵌套与重复 root 会被去掉，同一个文件不会出现两次。合并多个 root 的结果时输出绝对路径（`path_style=absolute`），只有一个 root 时保持原来的 root 相对路径（`path_style=root_relative`）。project 模式显式收到多个 roots 的共同父目录或项目外路径时返回 `PROJECT_SCOPE_PATH_MISMATCH`，提示省略 `path`；用户明确要求项目外路径时，模型必须使用 `scope_mode=explicit_path`，该模式仍按开放权限合同直接搜索，不触发审批。回执同时记录 `scope_mode/scope_relation/scope_source`，区分 `inside_project/common_ancestor/outside_project` 与默认/显式来源。

`WorkspaceExecutionBroker` 只由 `dependency_install`、`git_remote` 和 MCP Artifact bridge 使用。调用方从 normalized `PermissionContext` 产生 canonical writable/protected scope；workspace 外 root 不进入可发布集，protected roots 同时保留 lexical 位置和 resolved target。`Bash` 不调用该 broker，也没有 `workspace_scope_declared`/staging 分支。

路径判断使用 lexical path 与 realpath/祖先检查：

- 拒绝通过现有叶子或祖先 symlink 逃逸写根；
- 允许 writable root 内尚不存在的叶子；
- 显示名映射到附件时必须唯一，歧义即失败；
- home、根目录、credential、SSH、Git 配置等敏感位置有独立 hard gate；
- recoverable delete 额外拒绝 symlink、特殊文件、根/受保护路径。

Desktop 右侧项目文件树使用另一条更窄的只读边界，不继承“工具可读任意绝对路径”的 Agent 语义。Renderer 只能请求当前 active `projectId` 已绑定文件夹中的一个相对目录；Electron Main 每次从 Core 刷新项目事实，核对精确 folder membership，并同时执行 lexical containment 与 realpath containment。目录 symlink 可以作为叶子显示，但不能继续遍历；未绑定文件夹、`..`、绝对路径和 symlink escape 全部 fail closed。该 bridge 只返回目录项元数据，不提供任意文件读取。

WorkspaceRoot 是 typed 文件/Artifact/provider 的默认 run 范围，不是当前登录用户 `Bash` 的操作系统边界。Bash 可通过 thread cwd 进入 workspace 外目录，也可读写 Skill root/用户 cache；Outcome 只证明进程事实，不声称这些副作用经过 TaskScope 观测或提交。外部 provider 的远程资源不伪装成本地路径；其 scope 由 provider manifest 与 typed input/output 契约表达。

## 2. 读、写、删除

| 操作 | 当前边界 |
| --- | --- |
| 读 | 任意路径。相对路径按主文件夹解析，绝对路径指哪读哪；形状名单不套用于读 |
| `Edit`/`Write` | 任意路径，但形状名单（`.git` 内部、`.env`、密钥/凭据、shell 配置、`~/.awaken`）在任何位置都拒绝；父路径和 symlink 链校验；覆盖还要求当前 thread 的未过期 Read 快照 |
| 删除 | 仅明确普通文件/目录，且必须由 recoverable-delete 路径处理 |
| `Bash` | cwd 可在 workspace 外，以当前登录用户的操作系统权限直接执行；TaskScope 只作运行上下文，不冒充全部副作用门禁 |
| 保留的 Workspace broker | 只服务 dependency/git/MCP Artifact；每个 staged delta 依据 canonical writable/protected roots 验证，Trash/发布前 fail closed |
| typed Git clone | destination 必须是 writable TaskScope 内非敏感、非 workspace root 的相对路径，父目录必须已存在；broker 的可发布集合固定为该 destination，覆盖前版本进入 Trash |

## 3. 不变量

- Renderer 传入的 path、Skill root 或 MCP metadata 不是授权事实。
- lexical `HasPrefix` 不能作为路径边界；必须使用平台路径规范化和祖先/realpath 校验。
- Workspace broker 顺序必须是 seal -> scan staged -> build plan -> validate protected/writable -> recheck live concurrent state -> Trash recovery -> publish。任一 delta 越界时整批 deny，在 Trash 和 live publish 前返回 `denied_writable_scope` 或 `denied_protected_root`、`side_effect_state=none`、`live_workspace_unchanged` 和 `trash_not_invoked`。
- broker 的 `workspace_scope_declared=true` 且 canonical writable set 为空时表示 workspace 内零可发布路径，不得回落扩大到 workspace；这是 `git_remote ls_remote` 的正常合同。
- Workspace broker 将 `WritableRoots` 与 `ProtectedRoots` 用于提交门禁；MCP Artifact bridge 在进入 broker 前还必须满足 `OutputRoots`。
- `git_remote ls_remote` 显式声明零 writable roots；`git_remote clone` 不继承整个 workspace 的可写范围，而是把 broker request 再收窄为唯一 canonical destination。remote checkout 产生 destination 外 delta 或 symlink 时整批拒绝。
- `ProtectedRoots` 必须覆盖 scope 里的每一个 root；只为 `WorkspaceRoot` 生成保护项是不安全的。
- `ProtectedRootExceptions` 只能是 `<AWAKEN_HOME>/sessions` 这一条子树，且只在本次 scope 真有 root 落在其中时出现。没有例外时 `~/.awaken` 全域保护不得被削弱，例外内部的形状规则也不得跳过。
- roots 不得被重新当成路径围栏。越界访问由用户指令驱动，不引入审批；要收紧只能通过形状名单，而形状名单只作用于写，不作用于读。
- `Glob`/`Grep` 的 `project` 模式纠正共同父目录和项目外路径只约束默认意图，不是权限拒绝；`explicit_path` 必须继续允许用户点名的外部绝对路径。
- 第三方提议的路径与用户指令不同源，但当前**不**因此额外收紧：MCP Artifact bridge 已放开 `writable ∩ output` 约束，仅保留 `relative_path` 格式校验（相对、canonical、禁止 `..`）、symlink 组件拒绝和 `ProtectedRoots`。
- 不带 `path` 的搜索必须只覆盖 readable roots；把默认范围放大到全盘会让一次无范围搜索不可用。
- 项目 ID、项目名、cwd、精确文件夹组和 permission profile 必须出现在 provider 可见的结构化 environment context 里。只写进宿主侧 `runtime_snapshot` 不算下发：模型看不到的 roots 会退化成「只知道 cwd」，进而靠 `..` 向上级目录试探。
- `project_id` 是 scope 身份，`project_name` 只做模型可读上下文。任何路径解析或权限判定都不得从展示名派生。
- 写失败、删除失败或副作用未知时不得把目标状态写成已完成。
- Windows 盘符、UNC、反斜杠、大小写与非 ASCII 路径需要平台测试，不能从 Unix fixture 推断。

## 4. 代码 owner

- `src/tool/permission.go`、`sensitive.go`：TaskScope、敏感路径与 `ProtectedRootExceptions`。
- `src/runtime/run_service.go`：`normalizeWorkspace` / `normalizeWorkspaceForThread`、`scopeProjectID`、`buildProtectedRoots`、`buildProtectedRootExceptions`。
- `src/desktopdata/paths.go`：`SessionsRoot` / `SessionWorkspacePath` / `SessionOutputPath`，session 工作目录路径的唯一构造点。
- `src/tools/internal/searchroots`：只读搜索工具的多根解析与展示路径规则。
- `src/prompt/system_prompt.go`、`runtime.go`、`templates/default_system.md`：项目名、文件夹组与作业范围约定的模型可见渲染。
- `src/agentcore/request_builder.go`：`projectFolderObservations`，把 scope 的 readable roots 投影成模型侧文件夹组。
- `apps/desktop/src/renderer/App.tsx`：`resolveRunProjectForRun` 决定每次 run 下发的 `project_id` 与 `project_name`，`resolveRunRootsForRun` 决定下发的 roots。两者都按「cwd 所属项目的任意文件夹」匹配，新建项目与改动文件夹组后由 `refreshProjects` 刷新的项目列表直接生效。同一个文件夹可以属于多个项目，所以路径不足以确定身份：优先采信侧栏当前选中的项目，只有它并不持有这个根时才回退到按路径查找，roots 必须取自被判定的那个项目，否则模型会看到另一个项目的文件夹组。
- `apps/desktop/src/main/project-files.ts`：右侧项目文件树的 project/folder membership、relative path、lexical containment 与 realpath containment 只读门禁。
- 各 typed tool：输入 schema 与具体路径类型。
- `src/tool/execution_broker.go`、`execution_broker_workspace.go`：dependency/git/MCP Artifact 的 canonical roots 与提交前全量 delta 门禁。
- `src/tool/trash`：平台 recoverable-delete adapter。
