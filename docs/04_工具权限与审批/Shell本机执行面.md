# Shell 本机执行面

## 1. 当前默认是当前登录用户的真实 Shell

Awaken Desktop 启动的 `awaken-core` 与当前登录用户同一操作系统身份。普通 run 的 `shell` 默认使用 `LocalUserExecutionBroker` 直接启动子进程，不再由 Awaken 额外施加 workspace、用户目录、网络、包管理器、安装脚本或 CLI 白名单。

默认执行链是：

```text
Shell input
  -> 参数与 cwd 校验
  -> 命令分类（只用于观测）
  -> LocalUserExecutionBroker allow
  -> 继承 Desktop 已解析的登录用户环境
  -> 用同一快照固定的用户 Shell 启动子进程
  -> 返回真实 exit code / stdout / stderr / timeout
  -> canonical ExecutionOutcome
```

因此 Agent 可以直接：

- 读写 workspace 之外的当前用户可访问路径，包括 `~/.awaken/skills` 和用户缓存；
- 执行 `npm install`、`pip install`、`brew` 及 Skill 自带安装脚本；
- 访问 npm registry、下载浏览器或模型等运行资源；
- 调用 Homebrew、系统程序、LibreOffice、FFmpeg、ImageMagick、Pandoc 或其他当前用户已安装的 CLI；
- 指定任意当前用户可进入的 `workdir`，并将产物写入用户指定的目录。

Shell 的 command 分类、environment snapshot 和 broker receipt 仍保留作为运行诊断；它们不是默认权限门禁。

## 2. 环境继承

普通 Desktop run 的 Shell 使用 `EnvInherit`。Electron 即使由 Finder 启动，也会在启动 Core 前优先通过当前用户的交互式 login shell 一次性解析 NUL 分隔环境；失败时回退非交互 login shell，再失败才回退 Electron 进程环境。每种 shell 模式均有 3 秒超时、256 KiB 输出上限、marker 隔离且不记录环境值；shell 探测成功后采用其最终环境，不会把启动前进程中已被 shell 删除的变量重新补回。Core 再基于这份环境继承 `HOME`、完整 `PATH`、用户缓存、语言环境、代理和证书，并在不覆盖原值的前提下补入：

- macOS/Linux 的 `~/.local/bin`、`/opt/homebrew/bin`、`/usr/local/bin`；
- Windows 的 npm、pnpm、WindowsApps、Scoop 和用户 `.local/bin` 常见路径。

run-start 环境快照与 Shell transport 共用同一个 host resolver。快照只向模型/日志投影最多 128 个 PATH entry，但另以 `PATHDigest`、`PATHEntryCount`、`PATHTruncated` 和不序列化的 pinned transport PATH 绑定完整真值；真实子进程不会因为诊断预览有界而丢失后续 PATH entry。Shell 使用快照固定的用户 shell，并以 no-profile/no-rc 参数执行当前命令；登录环境已经由 Desktop 注入，Skill 若确实需要额外 startup 文件仍可在命令中显式 source。

Awaken 不为通用 Skill或 typed Work provider 准备完整 runtime，也不把用户的 Node、Python、LibreOffice、Poppler、FFmpeg 或系统程序替换成平台私有副本。typed Work provider 使用同一当前用户环境做组件级 probe；缺失时 Agent可按真实错误使用 Shell 安装并重试。

`EnvMinimal` 仍可由 guardian 或显式受限 run 使用；它不是普通 Desktop Agent 的默认环境。

## 3. Skill 初始化与脚本

Skill 激活后，宿主将当前选中 Skill 的 identity、绝对包根、`SKILL.md` 路径和正文提供给 Agent。Skill 正文若要求：

```bash
cd <skill-root>
npm install
```

Agent 可直接使用 Shell 执行，依赖驻留在该 Skill 包中，用户缓存由实际包管理器或脚本按自身规则使用。Awaken 不解析或代理每一种 `package.json`、requirements 或系统安装方式，也不需要为单个 Skill 建立专用 runtime。

Agent 必须把项目 workspace 与 Skill root 分开：初始化依赖可在 Skill root 执行；使用相对输入/输出的 Skill 流水线应以用户工作目录为 cwd，用绝对路径调用 Skill 脚本。

## 4. 依赖探测

Node 依赖的只读探测不使用 `require.resolve("<package>/package.json")` 作为统一是否安装判据。宿主优先解析声明的模块入口；若请求子路径返回 `ERR_PACKAGE_PATH_NOT_EXPORTED`，则与 `MODULE_NOT_FOUND` 分开处理，并可回退解析 distribution 主入口。版本可从已解析入口向上定位同名 package metadata，探测过程不执行包代码。

对通用 Skill，最终仍以 Skill 声明命令的真实 exit code、stderr 和产物校验为准，不让宿主的启发式 probe 提前封死执行路径。

## 5. 安全与可观测边界

默认 direct Shell 的真实边界是当前操作系统用户的权限，而不是 Awaken 额外的 workspace policy。因此必须如实区分：

- typed file/Artifact/Connector provider 仍可提供 schema、TaskScope、Trash recovery、收件人审核或精确变更证据；
- raw Shell 只能可靠证明进程是否启动、退出、超时及它返回的 stdout/stderr；
- raw Shell 或其子脚本可以执行永久删除、项目外写入、网络请求或 CLI 消息发送；Awaken 不声称这些未建模副作用可恢复、可完整观测或必然经过审核；
- `LocalUserExecutionBroker` receipt 会明确标注 `current_login_user_direct` 与未观测副作用，不冒充 staging/sandbox 或事务提交证据。

`RestrictedExecutionBroker`、`WorkspaceExecutionBroker` 和平台 sandbox 实现仍可被测试、guardian 或未来显式受限模式注入；它们不是普通 Desktop run 的默认 provider，也不能用来说明普通 Agent 仍被限制。

## 6. 平台与验收

| 平台 | 默认语义 | 当前验收边界 |
| --- | --- | --- |
| macOS | Desktop 解析用户 login env；transport 使用快照固定的 `$SHELL`，继承完整 PATH 并补 Homebrew/用户路径 | 当前开发机可做真实 Desktop、npm、Puppeteer 与用户目录验收 |
| Windows | 继承登录用户进程环境；优先快照固定的 shell，必要时回退 `cmd.exe`，并补常见用户 CLI PATH | 需 Windows Desktop 实机验收，cross-build 不代表产品通过 |
| Linux/其他 | Desktop 解析用户 login env；transport 使用快照固定的用户 shell | 需对应 Desktop/发布环境实机验收 |

自动化至少覆盖：默认 broker 跨 workspace 读写、包管理器/网络命令不被政策层拒绝、`HOME`/PATH/代理环境 canary 继承、用户缓存写入、长时间 install/build 超时上限、取消与进程树终止，以及显式注入 restricted/staged broker 时仍按该合同执行。

## 7. 代码 owner

- `apps/desktop/src/main/runtime-supervisor.ts`：Finder/GUI 启动时解析登录用户环境并注入 Core，失败安全回退且不记录环境值。
- `src/tools/shell/environment.go`、`shell.go`、`policy.go`、`tier.go`、`executor.go`：共享 host 环境、Shell 定义、观测分类、默认执行和进程管理。
- `src/tool/execution_broker.go`：`LocalUserExecutionBroker` 与可选 restricted broker 合同。
- `src/tool/execution_broker_workspace.go`、`src/tool/sandbox/*`：显式 staged/restricted 执行路径，不是默认 Desktop Shell。
- `src/agentcore/failure_supervisor.go`：Shell 失败分类、恢复和重试预算。
