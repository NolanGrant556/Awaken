# Bash 本机执行面

## 1. 当前支持

模型基础工具 `Bash` 以启动 Awaken Desktop 的当前登录用户身份执行命令。它不经过 `WorkspaceExecutionBroker`，不建立 restricted、supervised、staging 或 `broker_receipt` 分支，也不把 TaskScope 冒充为操作系统沙箱。

`ComputerEnabled` 只表示当前 run 的 Computer Runtime 与 Host 已就绪，不改变 `Bash` 的暴露。Computer-ready run 中，`Bash`、`computer_observe`、`computer_act`、`computer_verify` 可以同时存在并交替调用；显式 `ToolExposure.ExtraHidden=["Bash"]` 仍按通用工具暴露策略生效。

输入合同：

```text
command             必填命令文本
timeout             毫秒；前台默认 120000，最大 600000
description         可选活动标题
run_in_background   可选；true 时立即返回后台任务信息
```

前台结果包含实际 shell、cwd、stdout、stderr、exit code、timeout、cancel、truncation 和 duration。合并输出上限为 50,000 bytes；超出时显式标记 `truncated`。进程退出非零、超时和取消都保留真实 process/transport 事实。

## 2. 环境、Shell 与 cwd

Desktop 在启动 Core 前解析当前用户 login environment；run-start 环境快照固定完整 transport PATH、shell 和 executable observations。`Bash` 使用同一快照，启动时禁用 profile/rc，避免每次命令重新运行用户启动脚本。

- macOS/Linux 使用快照固定的用户 shell；Bash 传 `--noprofile --norc`，zsh 传 `-f`。
- Windows 按已验证的 Git Bash -> `pwsh` -> `powershell` -> `cmd` 回退，并在结果中报告实际 shell。Git Bash 只从显式 `AWAKEN_GIT_BASH_PATH`、Git for Windows 的 `git.exe` 相邻安装根、标准安装根或可识别的 Git PATH 取得；`Windows\System32\bash.exe` / `Sysnative\bash.exe` 属于 WSL 启动入口，不能作为 Git Bash。每个候选在首次用于当前环境快照前都执行最长 3 秒的无 profile、无交互 marker 健康检查，失败后才选择下一候选。
- cwd 属于 thread 的 `ToolSessionState`，不能调用进程级 `chdir`；命令成功改变目录后只刷新该 thread 的 cwd。
- thread 删除、workspace/profile 重建或 Core 退出时，内存 cwd 安全失效；它不写入 SemanticJournal。

`Bash` 可以在当前用户可访问范围内运行包管理器、Skill 脚本、网络 CLI、LibreOffice、Pandoc、Typst、Python/Node 等程序。它不自动获得 `Read`/`Edit`/`Write` 的 TaskScope、预读快照、原子替换、Trash 或 Artifact 事实。

## 3. 后台任务

`run_in_background=true` 启动 thread-owned 进程并立即返回：

```text
task_id
log_path       stdout/stderr 合并日志
status_path    原子更新的 JSON 状态
shell
cwd
```

后台任务在正常 run 结束后继续运行；显式取消该 run 时终止其进程组。删除 thread 或退出 Core 时终止归属任务。Unix 使用进程组，Windows 使用新进程组并优先 `taskkill /T /F` 清理子树。

日志和状态文件位于 Awaken 管理的用户缓存，不进入 workspace，也不成为 Artifact。它们可能包含命令本身输出的敏感内容；工具不宣称对任意 CLI 自由文本完成通用秘密扫描。

## 4. 安全边界

- `Bash` 的真实边界是当前用户的 OS 权限、环境和网络权限。
- command 分类只服务活动摘要、失败监督和恢复判断，不授权或拒绝命令。
- 直接观察和操作 GUI 应使用 `computer_*`。Computer 不可用时不得静默用 AppleScript、`xdotool`、坐标脚本等冒充 Computer Use；用户明确要求终端 GUI 自动化时可以使用 `Bash`，但结果只属于终端行为。
- `Bash` 与 Computer 在 Journal/API 中继续形成独立 tool call/outcome。Bash 调用不会获得 Computer 的 observation、action digest、review、lease/fence 或 verify 事实，也不能被展示为 Computer 动作。
- 永久删除、workspace 外写入、网络请求和子脚本副作用不能被 Awaken 完整观察或恢复。
- Shell 回退只发生在用户命令启动前的健康检查阶段；用户命令已经开始后，无论非零退出还是 transport 失败都不得换 Shell 自动重放，避免重复副作用和语法误判。
- 需要明确可恢复删除时使用 `trash_file`；需要文件并发保护时使用 `Read` 后的 `Edit`/`Write`。
- `WorkspaceExecutionBroker` 只保留给 host-owned `dependency_install`、`git_remote` 和 MCP Artifact commit；这些窄合同不得被解释为 `Bash` 保证。

## 5. 当前限制

- 后台任务当前只有启动、日志、状态和生命周期清理，没有独立模型工具用于列举或主动终止单个 task。
- Windows 只有 cross-build/测试编译证据时不能宣称实机进程树与 shell 回退已经通过。
- raw 命令结果证明 process 事实，不证明输出文件格式、业务正确性或可恢复性。

## 6. 代码 owner 与验收

- `src/tools/bash`：定义、环境、cwd、前后台执行和进程树。
- `src/tool/host_environment.go`、`environment_snapshot.go`：当前用户环境快照。
- `src/tool/session_state.go`：thread cwd。
- `src/runtime/run_service.go`、`project_service.go`、`cmd/awaken-core/main.go`：取消、thread 删除和 Core 退出清理。

自动化至少覆盖 cwd 隔离、真实 exit/stdout/stderr、120/600 秒边界、timeout、前后台取消、Windows test binary 编译、WSL 启动入口拒绝、Git Bash 派生、健康检查失败后的 PowerShell/cmd 回退、旧 restricted/broker 路径残留扫描，以及 Computer-ready 快照中的 Bash 共存和显式隐藏。真实 Desktop 还需运行一个前台 cwd 变更和一个后台任务，并在 Computer-ready run 中交替执行 Computer observation、Bash 只读命令和 Computer observation/verify，核对同一 Computer session、固定快照与 thread cwd。
