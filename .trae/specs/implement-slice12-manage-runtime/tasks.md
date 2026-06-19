# Tasks

- [x] Task 1：Runtime 支持 AWAKEN_CONFIG_DIR 与 AWAKEN_DEFAULT_WORKSPACE
  - [x] SubTask 1.1：在 `src/llm/client.go` 的 `LoadConfig` 中，优先从 `AWAKEN_CONFIG_DIR` 环境变量指向的目录读取 `.env` 和 `config.toml`；未设置时保持现有 cwd 行为
  - [x] SubTask 1.2：`loadEnvFile` 和 `loadConfigFile` 接受目录参数（或 LoadConfig 内部拼接路径），避免硬编码相对路径
  - [x] SubTask 1.3：在 `cmd/awaken-core/main.go` 中，`default_workspace` 优先取 `AWAKEN_DEFAULT_WORKSPACE` 环境变量，未设置时回退 `os.Getwd()`
  - [x] SubTask 1.4：在 `src/llm/llm_test.go` 新增测试：设置 `AWAKEN_CONFIG_DIR` 后从该目录读取配置；未设置时行为不变
  - [x] SubTask 1.5：运行 `go test ./src/llm/... ./cmd/awaken-core/...` 通过

- [x] Task 2：构建 bundled awaken-core binary
  - [x] SubTask 2.1：在根 `package.json` 新增 `build:binary` script，调用 `go build -o apps/desktop/resources/bin/<platform-arch>/awaken-core ./cmd/awaken-core`，根据 `process.platform`/`process.arch` 决定输出目录
  - [x] SubTask 2.2：在 `apps/desktop/package.json` 新增 `build:binary` script（或复用根 script），支持 macOS arm64/x64
  - [x] SubTask 2.3：在 `apps/desktop/.gitignore`（或根 .gitignore）添加 `apps/desktop/resources/bin/` 忽略规则，避免提交 binary
  - [x] SubTask 2.4：验证 `npm run build:binary` 产出可执行 binary，`./apps/desktop/resources/bin/<platform-arch>/awaken-core --help` 或直接运行能监听端口

- [x] Task 3：RuntimeSupervisor 模块
  - [x] SubTask 3.1：新建 `apps/desktop/src/main/runtime-supervisor.ts`，实现 `RuntimeSupervisor` 类：`start()`、`stop()`、`restart()`、`getStatus()`、`getLogs()`、`onLogs(callback)`、`onStatusChange(callback)`
  - [x] SubTask 3.2：`start()`：分配随机端口（bind 临时 socket 获取空闲端口，关闭后传给子进程），spawn bundled binary（路径根据 `process.platform`/`process.arch` 解析，dev 模式跳过），传入 `AWAKEN_CORE_ADDR=127.0.0.1:<port>`、`AWAKEN_CONFIG_DIR`、`AWAKEN_DEFAULT_WORKSPACE`
  - [x] SubTask 3.3：`start()` 健康检查：轮询 `GET /health`（间隔 200ms，超时 10s），ready 后状态转 `running`；超时转 `crashed` 并收集错误
  - [x] SubTask 3.4：收集子进程 stdout/stderr，按行推入 ring buffer（上限 1000 行），通过 `onLogs` 回调通知
  - [x] SubTask 3.5：监听子进程 `exit` 事件：非主动停止时转 `crashed`，记录 exit code；主动停止时转 `stopped`
  - [x] SubTask 3.6：`stop()`：发送 SIGTERM，等待 3s，仍存活则 SIGKILL（Windows 用 `taskkill`）
  - [x] SubTask 3.7：`restart()`：先 `stop()` 再 `start()`
  - [x] SubTask 3.8：dev 模式（`AWAKEN_DESKTOP_RUNTIME_URL` 已设置）下 `start()` 为 no-op，状态直接 `running`，不 spawn

- [x] Task 4：Config bootstrap（Application Support）
  - [x] SubTask 4.1：新建 `apps/desktop/src/main/config-bootstrap.ts`，导出 `ensureConfigDir(): Promise<string>`，返回 `app.getPath('userData')/config` 路径
  - [x] SubTask 4.2：首次启动时若 `config.toml` 不存在，写入默认模板（复用项目根 `config.toml` 的非敏感默认值，base_url/model_id/protocol 等）
  - [x] SubTask 4.3：若 `.env` 不存在，写入 `LLM_API_KEY=` 占位 + 注释提示用户填写
  - [x] SubTask 4.4：已存在的配置文件不覆盖
  - [x] SubTask 4.5：返回的 config dir 路径传给 RuntimeSupervisor 作为 `AWAKEN_CONFIG_DIR`

- [x] Task 5：Electron main 集成 supervisor
  - [x] SubTask 5.1：在 `apps/desktop/src/main/index.ts` 中，`app.whenReady` 时调用 `ensureConfigDir()`，创建 `RuntimeSupervisor` 实例并 `start()`
  - [x] SubTask 5.2：`runtimeBaseUrl` 改为从 supervisor 获取（运行时分配的端口）；dev 模式仍用 `AWAKEN_DESKTOP_RUNTIME_URL`
  - [x] SubTask 5.3：`before-quit` 时调用 `supervisor.stop()` 并等待完成
  - [x] SubTask 5.4：新增 IPC：`awaken:runtime-logs`（主进程推送日志行）、`awaken:restart-runtime`（调用 supervisor.restart）、`awaken:get-supervisor-status`（返回当前状态）
  - [x] SubTask 5.5：supervisor 状态变化时主动推送给 renderer（`awaken:supervisor-status-changed`）

- [x] Task 6：Preload 与共享类型扩展
  - [x] SubTask 6.1：在 `apps/desktop/src/shared/runtime.ts` 新增 `SupervisorStatus` 类型（`"starting" | "running" | "crashed" | "stopped"`）、`RuntimeLogEntry` 类型
  - [x] SubTask 6.2：在 `apps/desktop/src/preload/index.ts` 暴露 `getRuntimeLogs()`、`restartRuntime()`、`getSupervisorStatus()`、`onRuntimeLogs(callback)`、`onSupervisorStatusChange(callback)`
  - [x] SubTask 6.3：更新 `AwakenBridge` 类型定义（README 中的契约同步，但不强制改 README）

- [x] Task 7：UI supervisor 状态与 logs 面板
  - [x] SubTask 7.1：在 `apps/desktop/src/renderer/runtime/reducer.ts` 新增 supervisor 状态、logs 缓冲（上限 1000 行）和相关 actions
  - [x] SubTask 7.2：在 `apps/desktop/src/renderer/runtime/client.ts` 新增 `getRuntimeLogs`、`restartRuntime`、`getSupervisorStatus`、`onRuntimeLogs`、`onSupervisorStatusChange` 方法
  - [x] SubTask 7.3：在 `apps/desktop/src/renderer/App.tsx` 顶部状态区显示 supervisor 状态 pill（Starting/Running/Crashed/Stopped）
  - [x] SubTask 7.4：crashed 时显示错误 notice（含 exit code）和 Restart Runtime 按钮
  - [x] SubTask 7.5：新增可折叠 runtime logs 面板，展示 logs 缓冲；Running 时实时追加
  - [x] SubTask 7.6：Restart 按钮点击调用 `restartRuntime()`，状态变化时 UI 同步

- [x] Task 8：electron-builder 打包配置
  - [x] SubTask 8.1：在 `apps/desktop` 新增 `electron-builder.yml`，配置 macOS target（dmg），`extraResources` 把 `resources/bin` 包含进 app
  - [x] SubTask 8.2：在 `apps/desktop/package.json` 新增 `electron-builder` devDependency，新增 `dist` script
  - [x] SubTask 8.3：RuntimeSupervisor 解析 binary 路径时，区分开发环境（`resources/bin/<platform-arch>/awaken-core`）和打包环境（`process.resourcesPath/bin/<platform-arch>/awaken-core`）
  - [x] SubTask 8.4：验证 `npm run build:binary && npm run build && npm run dist` 能产出包含 binary 的 .dmg（或至少 electron-builder dry-run 不报错）

- [x] Task 9：验收与 failure path 测试
  - [x] SubTask 9.1：happy path：打包或 `npm run build:binary` 后启动 Desktop（不带 `AWAKEN_DESKTOP_RUNTIME_URL`），不开终端，完成一次真实 run
  - [x] SubTask 9.2：failure path：删除 binary 后启动 Desktop，UI 显示 crashed/错误，不白屏
  - [x] SubTask 9.3：failure path：模拟端口冲突（占用 supervisor 分配的端口），supervisor 重试或报错可读
  - [x] SubTask 9.4：crash 恢复：手动 kill `awaken-core` 进程，UI 显示 crashed，点击 Restart 后恢复 running
  - [x] SubTask 9.5：退出清理：退出 app 后 `pgrep awaken-core` 无残留
  - [x] SubTask 9.6：配置持久化：首次启动在 Application Support 生成 config.toml/.env 模板；修改后重启 app 配置保留

- [x] Task 10：验证与验收
  - [x] SubTask 10.1：`go test ./src/llm/... ./cmd/awaken-core/...` 通过
  - [x] SubTask 10.2：`npm run typecheck:desktop` 通过
  - [x] SubTask 10.3：`npm --workspace @awaken/desktop run test` 通过
  - [x] SubTask 10.4：代码审查确认进程生命周期、配置迁移、UI 状态展示完整，runtime policy 仍在 Go 层

# Task Dependencies

- Task 1（Go 配置目录）独立，可先行
- Task 2（构建 binary）独立，可先行
- Task 3（RuntimeSupervisor）依赖 Task 1（需要 AWAKEN_CONFIG_DIR）和 Task 2（需要 binary 路径）
- Task 4（Config bootstrap）独立，可与 Task 3 并行
- Task 5（main 集成）依赖 Task 3 和 Task 4
- Task 6（preload/共享类型）依赖 Task 5 的 IPC 设计，可与 Task 5 并行定义类型
- Task 7（UI）依赖 Task 6
- Task 8（electron-builder）依赖 Task 2 和 Task 3（binary 路径解析）
- Task 9（验收）依赖 Task 1-8
- Task 10（最终验证）依赖 Task 1-9
