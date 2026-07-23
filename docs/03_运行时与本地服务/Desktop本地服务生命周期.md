# Desktop 本地服务生命周期

## 权威范围

本文唯一定义 Awaken Desktop 如何启动、探活、监督和关闭内置 Go 服务。正式用户启动命令仍以根 README 为唯一来源；本文不定义 Gallery、模型配置或 renderer 页面布局。

## 当前产品行为

Desktop 主进程启动一个本地 core 子进程，并通过随机 `127.0.0.1` 端口与其通信。端口由操作系统使用 `listen(0)` 分配，每次重试都重新分配；`8787` 不是 Desktop 产品合同，只能出现在独立开发/测试示例中。

renderer 不直接管理 core URL 或子进程，必须经 preload/main bridge 使用当前 supervisor 提供的地址。

## 状态机与 owner

[`apps/desktop/src/main/runtime-supervisor.ts`](../../apps/desktop/src/main/runtime-supervisor.ts) 拥有状态：

```text
stopped -> starting -> running -> stopping -> stopped
                    \-> crashed
```

- `starting`：选择新端口、构造受控环境、启动 child、轮询健康检查。
- `running`：健康检查已成功，renderer 才能获得可用 runtime。
- `crashed`：未处于显式 stop 流程的 child 退出。
- `stopping`：先请求应用级 shutdown，再升级为进程信号/强制结束。

启动默认最多 3 次，健康检查总时限 10 秒、轮询间隔 200 ms。generation token 隔离旧 start/restart 回调，避免一次迟到的健康检查把新进程状态覆盖。

## 环境与数据根

Desktop 向 core 注入：

- `AWAKEN_CORE_ADDR`：本次随机 loopback 地址；
- `AWAKEN_RUNTIME_MANIFEST`：打包的主运行时 manifest；
- `AWAKEN_USER_DATA_DIR`：当前 launch mode 的 Electron userData，供桌面偏好、宠物、旧状态兼容和测试隔离使用；
- `AWAKEN_HOME`：由 Main 从用户主目录确定的业务数据根 `~/.awaken`，承载 FileStore、模型配置、user MCP server 配置和 connector 认证 metadata；其中 user MCP 配置唯一位于 `config/mcp-servers.json`；
- `AWAKEN_DEFAULT_WORKSPACE`：始终注入。值为用户明确选择过的 workspace；没有任何用户选择时注入空值，core 视为"无默认 workspace"启动，不回退 cwd 或用户主目录。用户主目录不再被隐式注册为默认项目；startRun 仍要求每次 run 携带有效 workspace。

启动前移除 `AWAKEN_CONFIG_DIR` 和历史 LLM provider/API key 环境变量，避免 shell 环境意外改变 Desktop 身份或泄露旧凭据。模型 key 由 05 域的 `~/.awaken/.env` 文件契约管理。MCP user config 不从 `AWAKEN_USER_DATA_DIR` 或任何旧位置迁移、导入或 fallback；没有 default workspace 时 project scope 缺席，但 user scope 的管理与连接不受影响。

## 停止与崩溃恢复

正常 stop 先调用 09 协议域定义的 shutdown 动作，最多等待 2 秒，再发送平台对应的 graceful signal；约 3 秒后仍未退出则强制结束，约 3.5 秒完成 stop 收口。Unix 使用 `SIGTERM`/`SIGKILL`，Windows 使用受支持的中断与 `taskkill` 路径。

supervisor 只保留最多 1,000 条当前进程日志；重启会清空内存日志缓冲，这不是审计或会话存储。

core 异常退出后状态为 `crashed`。重新启动会恢复线程历史，但不会恢复旧 child 中的 goroutine、SSE stream 或 Office 子进程。开放语义事实如何封口由 [SemanticJournal 与重放](SemanticJournal与重放.md) 定义。

## 不变量与当前不支持

- 正式 Desktop 必须使用随机 loopback 地址，不得把固定端口写入 renderer 或文档。
- renderer 不得自行 spawn core 或持有独立的 runtime lifecycle。
- userData、`AWAKEN_HOME`、manifest 和 workspace 必须显式传入；不得依赖调用者终端当前目录推断身份。
- `AWAKEN_HOME/config/mcp-servers.json` 是 user MCP server 配置的唯一 authority；无 default workspace 不能使 user scope 管理面不可用。
- stop 必须有 graceful -> force 的有界升级，不能留下孤儿 core。
- 当前没有把外部远程 runtime URL 作为正式 Desktop 产品入口。
- 当前不保证 crash 后自动续跑 active run，只保证可确认历史恢复。

## `awaken-acp` 入口

除 Desktop 外，Awaken 还提供一个小型 stdio ACP Agent 入口 `cmd/awaken-acp`。它由 `src/acp/agentbridge` 实现，行为如下：

- 通过 `--startup=desktop|core|fail` 指定没有 ready Core 时的策略；默认 `desktop`。
- 复用同一 userData identity 的现有 Core；没有时按策略拉起 Desktop 或启动一个受 lease 管理的 headless Core，禁止无约束多 Core 写同一 userData。
- Core 在 `<AWAKEN_USER_DATA_DIR>/runtime/core.json` 发布 manifest（含随机 loopback base URL、control token、runtime identity 和 PID），launcher 校验 identity、PID 存活和 authenticated `/v1/control/runtime/identity` 后 attach。
- attach 后通过 authenticated control routes 创建 bridge lease，并每 10 秒 heartbeat；stdio EOF 或 signal 触发 5 秒有界清理，cancel active run、close session、释放 lease。
- 不执行模型与工具，只转发 ACP session lifecycle 和 prompt 到 Core HTTP routes。

`awaken-acp` 是机器调用入口，但仍禁止用户直接运行和长期维护裸 `awaken-core`。

## 测试与真实 Desktop 验收

自动化 owner：[`apps/desktop/src/main/runtime-supervisor.test.ts`](../../apps/desktop/src/main/runtime-supervisor.test.ts)、[`scripts/tests/desktop-lifecycle.test.mjs`](../../scripts/tests/desktop-lifecycle.test.mjs)。测试覆盖随机端口、重试、健康超时、generation、环境清理和升级关闭。

真实验收必须通过 README 正式入口：确认只出现一个受管 Desktop/core 实例、实际地址为 loopback 随机端口、health 成功后 UI 才可用；关闭后 core 退出；重新打开后历史从持久层恢复。不要把独立运行 core 或固定 `8787` 的 curl smoke 当成 Desktop 验收。
