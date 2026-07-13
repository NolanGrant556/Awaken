# 本机 Work 依赖发现与恢复

## 1. 当前产品模型

Awaken 不下载、复制、版本化或打包 Python、Node、LibreOffice、Poppler、FFmpeg 等完整 Work runtime。Desktop/Core 先正常启动；Agent 与 typed Work provider 复用当前登录用户已经拥有的项目、Skill、用户级和系统级环境。

依赖来源按实际安装语义持久化并自然复用：项目 `.venv`/`node_modules` 属于项目，Skill 依赖属于 Skill 包，pip/npm 用户依赖属于用户，Homebrew/winget/系统应用属于当前电脑。Awaken 不增加统一 tools 目录，也不维护 active、previous、revision 或 Environment Manager。

## 2. 环境观察

`ExecutionEnvironmentSnapshot` 在 run 开始观察 OS/arch、workspace/cwd、登录 Shell、完整 transport PATH digest 与有界诊断预览、可执行文件、包管理器、workspace venv/node_modules 和 Skill roots。Desktop 由 Finder 等 GUI 入口启动时，supervisor 优先读取当前用户交互式 login 环境，失败后回退非交互 login 环境和 Electron 进程环境；环境值和秘密不写日志。

普通 Shell 使用同一当前登录用户环境，可访问网络、项目外用户目录、包管理器、Skill 安装脚本与系统 CLI。环境快照是观察，不是安装许可列表。

## 3. typed Work 依赖发现

`workspace_dependencies` 与 `runtime_preflight` 保留为兼容工具名，但当前返回的是 `host-environment`：Python、Node、LibreOffice/soffice、Poppler 和 FFmpeg 的实际发现路径、探测结果与缺口，不再读取 manifest/current.json。

resolver 优先使用显式进程环境提示和当前 PATH，并补充 macOS Applications、Windows Program Files/LocalAppData 与 Linux 常见应用位置。Python 必须能启动并导入工具实际需要的模块；LibreOffice、Poppler、FFmpeg、Node 使用有界版本/能力 probe。不同 Work 工具只检查自己声明的组件，PPT 不因无关 FFmpeg 缺失而不可用。

缺失结果不跨调用缓存。Agent 或用户安装依赖后，下一次调用会重新发现；当前 GUI PATH 尚未变化时，Agent 可使用安装结果的绝对路径或开启后续 run 重新读取登录环境。

## 4. 缺失与恢复

typed Work 工具保持可见，缺少依赖不是权限 gate。执行前检查失败会返回结构化 `dependency_missing` 内容、缺失组件、实际诊断和 `install_missing_dependency`/`retry_tool` recovery options，不启动产物写入。Agent随后可按 Skill 或真实 Shell 错误执行 `brew`、`winget`、`pip`、`uv`、`npm` 等正常安装命令，再重试原工具。

通用 Skill 不需要宿主安装计划；它可在 Skill root 或项目中执行自己的安装说明。现有 typed `dependency_install` 仍只负责它明确声明的 workspace Python/Node plan，不是通用 Work 环境的管理器。

## 5. 开发、打包与旧数据

`open:desktop`、`dev:desktop`、`build:desktop`、`dist:desktop` 均不准备 Work 环境。安装包只携带 Desktop、当前架构 Core 和产品自身必需资源，不携带 runtime archive。

开发版使用 `Awaken Dev`、`@awaken-dev/desktop`；安装版使用 `Awaken`、`@awaken/desktop`。两者配置、凭据、会话和单实例域隔离，但共同复用操作系统/用户/项目/Skill 环境。

旧版本拥有的 `<userData>/runtimes/primary-runtime` 是可重新获得的产品缓存。新 Desktop 首次启动执行一次性、精确路径且拒绝 symlink root 的迁移删除；迁移失败不得阻止 Desktop 启动。源码回撤、删除 App 或普通项目清理不会被描述成已清除 userData。

## 6. 平台边界

- macOS：当前代码支持 login PATH、Homebrew 与 Applications 发现；必须用真实 Desktop 验证已有依赖复用和缺失后安装重试。
- Windows：启动不再要求可重定位 Python；代码支持 PATH、Python Launcher、Program Files/LocalAppData 和普通 Shell/winget，但仍需要 Windows 实机验收。
- Linux：遵循 PATH 与常见系统位置，当前不能从 macOS 验收推导发布结论。

## 7. 代码 owner

- `apps/desktop/src/main/runtime-supervisor.ts`：登录环境解析与 Core 注入。
- `src/tool/environment_snapshot.go`、`environment_path_probe.go`：run 环境观察。
- `src/tools/shell`：当前登录用户 Shell。
- `src/tools/artifact_runtime/runtime_resolver.go`：Work 组件发现、probe 与缺失诊断。
- `src/runtime/officerender/host_runtime.go`：用户 LibreOffice 解析与执行前指纹复核。
- `apps/desktop/src/main/legacy-runtime-cleanup.ts`：旧 primary runtime 一次性删除。
