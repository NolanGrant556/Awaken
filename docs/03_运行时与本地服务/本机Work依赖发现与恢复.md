# 本机 Work 依赖发现与恢复

## 1. 当前产品模型

Awaken 不下载、复制、版本化或打包 Python、Node、LibreOffice、Poppler、FFmpeg 等完整 Work runtime。Desktop/Core 先正常启动；Agent、`Bash` 与 Skill 复用当前登录用户已经拥有的项目、Skill、用户级和系统级环境。

依赖按实际安装语义自然复用：项目 `.venv`/`node_modules` 属于项目，Skill 依赖属于 Skill 包，pip/npm 用户依赖属于用户，Homebrew/winget/系统应用属于当前电脑。Awaken 不维护独立 Work runtime revision。

## 2. 环境观察

`ExecutionEnvironmentSnapshot` 在 run 开始观察 OS/arch、workspace/cwd、登录 Shell、完整 transport PATH digest 与有界诊断预览、可执行文件、包管理器、workspace marker 和 Skill roots。Desktop 由 Finder 等 GUI 入口启动时，supervisor 优先读取当前用户交互式 login 环境，失败后回退非交互 login 环境和 Electron 进程环境；环境值和秘密不写日志。

`Bash` 使用同一固定环境，可访问网络、项目外用户目录、包管理器、Skill 安装脚本与系统 CLI。环境快照是观察，不是安装许可列表。

## 3. 当前依赖发现

当前没有 `workspace_dependencies`、`runtime_preflight` 或 typed Work runtime 工具。run-start snapshot 只观察固定 PATH 中的通用 executable、Python/Node、包管理器和 workspace marker；`Read` 的 PDF fallback 另外从固定 snapshot 查找 `pdfinfo`、`pdftoppm`。其他 Office/媒体依赖由 Skill 或 Bash 命令真实发现。

Agent 或用户安装依赖后，新的 run/显式环境 refresh 才能形成新 snapshot；当前 run 不热换已经固定的 executable identity。命令也可在用户明确知道路径时使用绝对 executable。

## 4. 缺失与恢复

生成工具缺少依赖不是权限 gate。`Read` 缺 Poppler 返回 `needs_runtime`；Skill/Bash 返回真实 spawn/exit/stderr。Agent 可按 Skill 指令执行 `brew`、`winget`、`pip`、`uv`、`npm` 等正常安装命令，再在新 run/刷新后重试。

保留的 typed `dependency_install` 只负责 provider 明确声明的 workspace Python/Node plan，继续使用 pinned executable、full sandbox、private staging、post-probe 和 recoverable commit；它不是通用 Work 环境管理器。

## 5. 开发、打包与旧数据

`open:desktop`、`dev:desktop`、`build:desktop`、`dist:desktop` 均不准备 Work 环境。安装包只携带 Desktop、当前架构 Core 和产品自身必需资源，不携带 runtime archive。

开发版与安装版配置、凭据、会话和单实例域隔离，但共同复用操作系统/用户/项目/Skill 环境。旧 `<userData>/runtimes/primary-runtime` 仅是历史可再获得缓存；一次性精确路径清理失败不得阻止 Desktop 启动。

## 6. 平台边界

- macOS：当前支持 login PATH、Homebrew 与当前用户应用；必须用真实 Desktop 验证依赖复用和安装后重试。
- Windows：代码支持登录用户环境与 Bash/pwsh/powershell/cmd 回退；cross-build 不代替实机。
- Linux：遵循 PATH 与用户环境，不能从 macOS 验收推导发布结论。

## 7. 代码 owner

- `apps/desktop/src/main/runtime-supervisor.ts`：登录环境解析与 Core 注入。
- `src/tool/environment_snapshot.go`、`environment_path_probe.go`：run 环境观察。
- `src/tools/bash`：当前登录用户 Shell。
- `src/tools/read`：PDF 的 Poppler dependency route。
- `src/tools/artifact_publish`：生成后的交付物发布；不负责依赖安装。
- `apps/desktop/src/main/legacy-runtime-cleanup.ts`：旧 primary runtime 一次性删除。
