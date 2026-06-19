# Slice 12 Review Fixes Plan

## Summary

修复 slice 12（Desktop 管理 awaken-core）review 中发现的问题，按 P1/P2/P3 优先级处理。P1 聚焦 supervisor 竞态、竞态测试覆盖、`.env` 空值覆盖真实环境变量；P2 修复 preload `runtimeBaseUrl` 过期、IPC channel 命名对齐 spec、Go 测试环境污染；P3 处理 logs 面板内联样式、React key、crashed 视觉区分、workspace 持久化、bootstrap 模板未生效字段。

## Current State Analysis

已通过 `go test`、`vitest`、`typecheck:desktop` 全绿。核心功能（binary 构建、supervisor 启停、端口分配、健康检查、日志收集、配置迁移、UI 状态展示）按 spec 实现。主要缺陷集中在：

- `RuntimeSupervisor.start()` 重试循环无取消机制，`stop()/restart()` 期间可能继续 spawn 新 child，违反"退出后无残留进程"验收。
- `loadEnvFile()` 对 `KEY=` 空值无条件 `os.Setenv`，会覆盖父进程真实 `LLM_API_KEY`。
- Go 测试用 `os.Setenv`/`os.Unsetenv` 手动恢复，污染同进程后续 smoke 测试。
- preload `bridge.runtimeBaseUrl` 在打包模式下停留在 `127.0.0.1:8787`，supervisor 分配随机端口后不更新。
- IPC 查询 channel 命名 `awaken:get-supervisor-status` 与 spec 的 `awaken:runtime-supervisor-status` 不一致。
- Logs 面板用内联样式 + 裸 hex 颜色 + `key={index}`，违反项目约定。
- `crashed` 与 `starting` 共用 `runtime-pill-warning`，视觉不可区分。
- workspace picker 结果不持久化，supervisor 始终用 home。
- bootstrap 默认模板含 `context_window`/`auto_compact_token_limit`，但 `LoadConfig` 未使用。

## Assumptions & Decisions

- **P1 #3 `.env` 空值**：采用"不覆盖已有非空环境变量"方案（保留模板 `LLM_API_KEY=` 占位）。模板不动，只改 `loadEnvFile` 逻辑。
- **P2 #5 IPC 命名**：将查询 channel 重命名为 spec 的 `awaken:runtime-supervisor-status`；保留 push channel `awaken:supervisor-status-changed`（spec 未命名 push，属合理扩展）。
- **P3 #10 workspace 持久化**：现在实现轻量版——userData 下 `last-workspace.json`，main 启动 supervisor 前读取，picker 成功后写入。
- **P3 #11 未生效字段**：仅从 `config-bootstrap.ts` 的 `DEFAULT_CONFIG_TOML` 移除 `context_window`/`auto_compact_token_limit`；parser 与根 `config.toml` 不动。
- **P3 #8 React key**：supervisor 生成单调递增 `id` 加入 `RuntimeLogEntry`，用 `entry.id` 作 key。
- 现有 CSS 约定：`styles.css` 已用裸 hex（如 `#166534`、`#b42318`），新增 class 沿用同色值保持一致；重点是消除 App.tsx 内联样式。

## Proposed Changes

### P1 #1 — 修复 supervisor start/stop/restart 竞态

**文件**：[apps/desktop/src/main/runtime-supervisor.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/runtime-supervisor.ts)

引入 `startGeneration` 单调递增 token，保证"generation 变更后不再 spawn 或 setStatus"：

- 新增字段 `private startGeneration = 0;`。
- `start()`（行 113-183）：
  - 在 dev 模式判断之后、循环之前，`const myGen = ++this.startGeneration;`。
  - for 循环体顶部（`this.setStatus("starting")` 之前）：`if (this.startGeneration !== myGen) break;`。
  - `await this.deps.allocatePort()` 之后、`spawnProcess` 之前：`if (this.startGeneration !== myGen) break;`（已分配端口但未 spawn，无需清理）。
  - catch 块内 `terminateChild` 之后、`if (attempt < maxAttempts)` 之前：`if (this.startGeneration !== myGen) break;`（被 stop/restart 取消，不再重试）。
  - 循环结束后的 `setStatus("crashed", ...)` 之前：`if (this.startGeneration === myGen)` 才设 crashed（避免被取消的旧循环覆盖 stopped）。
- `stop()`（行 185-198）：在 `this.setStatus("stopping")` 之前 `this.startGeneration++;`，使任何在途 `start()` 的 generation 检查失效。
- `restart()`（行 200-204）：`stop()` 已 bump generation，`start()` 再 bump 一次并运行新循环；旧循环在下一个 await 边界 break。
- 不变量：stale child 的 `exit/error` 已由 `attachChildHandlers` 的 `if (this.child !== child) return;` 守卫；stale `waitForHealth` resolve 不会触发 `setStatus("running")`，因为循环在 await 后先检查 generation。

### P1 #2 — 补竞态测试

**文件**：[apps/desktop/src/main/runtime-supervisor.test.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/runtime-supervisor.test.ts)

新增用例：

1. `stop() during starting (health pending)`：`waitForHealth` 返回永不 resolve 的 pending promise；`start()` 不 await（用 `void`），`await tick()` 让其进入 starting；调用 `supervisor.stop()`；断言 `spawnProcess` 只被调用 1 次，状态为 `stopped`，无第二个 child。
2. `restart() during starting`：同上 pending health；`void supervisor.start()`；`await tick()`；`await supervisor.restart()`（restart 内 stop 用立即 resolve 的 health）；断言只产生 2 个 child（旧的被 kill），最终 `running`，旧 child `killedWith === "SIGTERM"`。
3. `stale waitForHealth resolve does not overwrite stopped`：`waitForHealth` 返回可控 deferred；`void supervisor.start()`；`await tick()`；`await supervisor.stop()`；再 resolve 旧 deferred；`await tick()`；断言状态仍 `stopped`，`runtimeBaseUrl` undefined。
4. `stale waitForHealth reject does not trigger retry after stop`：`waitForHealth` 第一次 reject（可控 deferred）；`void supervisor.start()`；`await tick()`；`await supervisor.stop()`；reject 旧 deferred；`await tick()`；断言 `spawnProcess` 只调用 1 次（未重试），状态 `stopped`。

### P1 #3 — 修复 `.env` 空值覆盖真实环境变量

**文件**：[src/llm/client.go](file:///Users/nolangrant/Awaken/src/llm/client.go) `loadEnvFile()`（行 91-114）

在 `os.Setenv(key, value)` 之前加守卫：

```go
if strings.TrimSpace(value) == "" && os.Getenv(key) != "" {
    continue
}
```

即：`.env` 中空值不覆盖进程里已有的非空环境变量；空值且原变量也未设置时跳过（无副作用）。

**文件**：[src/llm/llm_test.go](file:///Users/nolangrant/Awaken/src/llm/llm_test.go)

新增 `TestLoadConfigPreservesParentEnvWhenDotEnvEmpty`：`t.Setenv("LLM_API_KEY", "real-key")`；在 tmp config dir 写 `.env` 内容 `LLM_API_KEY=\n`；`t.Setenv("AWAKEN_CONFIG_DIR", tmpDir)`；`LoadConfig()` 后断言 `cfg.APIKey == "real-key"`。

### P2 #4 — 修复 preload/client `runtimeBaseUrl` 过期

**文件**：[apps/desktop/src/renderer/runtime/client.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/client.ts) `createElectronClient`（行 114-234）

- 将 `runtimeBaseUrl` 从静态字段改为受 supervisor 状态驱动的值：
  - `let currentBaseUrl = bridge.runtimeBaseUrl;`
  - 调用 `bridge.onSupervisorStatusChange((info) => { if (info.runtimeBaseUrl) currentBaseUrl = info.runtimeBaseUrl; });`（返回的 unsubscribe 不持有，client 生命周期即 app 生命周期）。
  - 返回对象中 `get runtimeBaseUrl() { return currentBaseUrl; }`。
- `RuntimeClient` 类型 `runtimeBaseUrl: string` 不变（getter 兼容）。
- browser client 保持原样。

**文件**：[apps/desktop/src/renderer/App.tsx](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx) 行 311-312

`displayRuntimeBaseUrl` 逻辑保持 `supervisorRuntimeBaseUrl || runtimeClient.runtimeBaseUrl`；因 client 现在会随 supervisor 更新，fallback 也正确。

### P2 #5 — IPC channel 命名对齐 spec

**文件**：[apps/desktop/src/main/index.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/index.ts) 行 394

- `ipcMain.handle("awaken:get-supervisor-status", ...)` → `ipcMain.handle("awaken:runtime-supervisor-status", ...)`。

**文件**：[apps/desktop/src/preload/index.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/preload/index.ts) 行 102-103

- `getSupervisorStatus: () => ipcRenderer.invoke("awaken:get-supervisor-status")` → `ipcRenderer.invoke("awaken:runtime-supervisor-status")`。

**文件**：[apps/desktop/src/shared/runtime.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/shared/runtime.ts)

- `AwakenBridge.getSupervisorStatus` 方法名保留（语义清晰），只改底层 channel 字符串。

**验证**：grep `awaken:get-supervisor-status` 确认无残留引用（测试用 browser fallback，预期不命中 IPC 字符串）。

### P2 #6 — 修复 Go 测试环境污染

**文件**：[src/llm/llm_test.go](file:///Users/nolangrant/Awaken/src/llm/llm_test.go)

- `TestLoadConfigWithEnv`（行 31-63）：删除 `os.Setenv` + defer `os.Unsetenv` 块，全部改为 `t.Setenv("LLM_API_KEY", "test-key")` 等（`t.Setenv` 自动恢复原值，原未设置则恢复为 unset）。
- `TestLoadConfigFromConfigDir`（行 126-167）：删除手动 `old*` 保存与 defer 恢复块；对需要"未设置"的变量用 `t.Setenv("LLM_API_KEY", "")` 等（空字符串等效于未设置，因 `LoadConfig`/`firstNonEmpty` 视空为 absent；且 `t.Setenv` 恢复时若原值非空会还原真实 key，避免污染 smoke）。
- `TestLoadConfig`（行 10-29）：将 `os.Unsetenv(...)` 改为 `t.Setenv(..., "")`。
- 不变量：smoke 测试 `TestCallWithSmoke`/`TestStreamCallWithSmoke` 在同进程内运行时，前置测试不再清空真实 `LLM_API_KEY`。

### P3 #7 — Logs 面板去内联样式

**文件**：[apps/desktop/src/renderer/styles.css](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/styles.css)

新增 class（沿用现有 hex 色值）：

```css
.runtime-logs { max-height: 200px; overflow: auto; }
.runtime-logs-empty { margin: 0; color: var(--aw-text-tertiary); font-size: 13px; }
.runtime-log-entry {
  font-family: var(--aw-font-mono);
  font-size: 12px;
  white-space: pre-wrap;
  line-height: 1.5;
}
.runtime-log-time { color: var(--aw-text-tertiary); }
.runtime-log-stream-stdout { color: #166534; }
.runtime-log-stream-stderr { color: #b42318; }
```

**文件**：[apps/desktop/src/renderer/App.tsx](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx) 行 906-936

替换内联 `style={{...}}` 为 class：容器 `className="runtime-logs"`，空态 `className="runtime-logs-empty"`，每行 `className="runtime-log-entry"`，时间 `className="runtime-log-time"`，stream `className={\`runtime-log-stream-${entry.stream}\`}`。

### P3 #8 — Logs React key 改为稳定 id

**文件**：[apps/desktop/src/shared/runtime.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/shared/runtime.ts) 行 131-135

`RuntimeLogEntry` 新增 `id: number`。

**文件**：[apps/desktop/src/main/runtime-supervisor.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/runtime-supervisor.ts)

- 新增 `private logSeq = 0;`。
- `appendLog` 中 `const id = this.logSeq++;`，构造 entry 时带上 `id`。
- `clearLogs` 不重置 `logSeq`（保持全局单调，避免重启后 id 重复——可接受；若需重置则在 restart 的 `clearLogs` 后重置，但 supervisor 测试断言 logs 为空数组，重置与否都通过）。

**文件**：[apps/desktop/src/renderer/App.tsx](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx) 行 912

`key={index}` → `key={entry.id}`。

**文件**：[apps/desktop/src/main/runtime-supervisor.test.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/runtime-supervisor.test.ts)

- 行 129-135 的 `getLogs()` 断言：每个 entry 增加 `id` 字段（`id: 0` 等）。
- 行 188 `toHaveLength(1)` 不受影响。

**文件**：[apps/desktop/src/renderer/runtime/reducer.test.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.test.ts)

- 所有构造 `RuntimeLogEntry` 的 fixture（行 305-320、335、344、360、376 附近）补 `id` 字段（递增即可）。

### P3 #9 — 区分 crashed 与 starting 视觉

**文件**：[apps/desktop/src/renderer/styles.css](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/styles.css)

新增：

```css
.runtime-pill-error { background: rgba(242, 90, 90, 0.14); color: #b42318; }
```

**文件**：[apps/desktop/src/renderer/App.tsx](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx) 行 88-92

`crashed` 分支 `className` 由 `runtime-pill runtime-pill-status runtime-pill-warning` 改为 `runtime-pill runtime-pill-status runtime-pill-error`。

### P3 #10 — workspace 持久化（轻量版）

**文件**：[apps/desktop/src/main/index.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/index.ts)

- 新增内联 helper：
  - `async function readLastWorkspace(): Promise<string | undefined>`：读 `app.getPath('userData')/last-workspace.json`，解析 `{ path: string }`，校验是绝对路径目录则返回，否则 undefined。
  - `async function writeLastWorkspace(path: string): Promise<void>`：写上述文件。
- `app.whenReady`（行 411-448）：`const lastWorkspace = await readLastWorkspace();`，supervisor 构造时 `defaultWorkspace: lastWorkspace ?? app.getPath("home")`。
- `awaken:pick-workspace` handler（行 352-373）：成功返回前 `await writeLastWorkspace(selected);`。

**验收**：picker 选目录 → 退出 app → 重启 → `RuntimeInfo.default_workspace` 为上次选择（因 supervisor 用其作 `AWAKEN_DEFAULT_WORKSPACE`，runtime info 反映该值）。

### P3 #11 — 移除 bootstrap 模板未生效字段

**文件**：[apps/desktop/src/main/config-bootstrap.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/config-bootstrap.ts) 行 5-16

`DEFAULT_CONFIG_TOML` 删除 `context_window = 400000` 与 `auto_compact_token_limit = 360000` 两行；保留 `temperature`、`max_output_tokens`（`LoadConfig` 实际使用）。

**文件**：[apps/desktop/src/main/config-bootstrap.test.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/main/config-bootstrap.test.ts) 行 32-33

断言 `config.toml` 含 `[llm]`、`model_id` 仍成立；无需新增断言（移除字段不影响现有断言）。

## Verification

按顺序执行：

1. `go test -count=1 ./src/llm/... ./cmd/awaken-core/...`
2. `npm run typecheck:desktop`
3. `npm --workspace @awaken/desktop run test`
4. `npm run build:binary`
5. `npm --workspace @awaken/desktop run build`
6. `npm --workspace @awaken/desktop run dist -- --dir`
7. 手动 smoke：启动 packaged dir app，确认 runtime 自动启动、restart 可用、退出后 `pgrep awaken-core` 无残留；picker 选 workspace 后重启 app，`RuntimeInfo.default_workspace` 反映上次选择。

## 风险与回滚

- supervisor 竞态修复改动 `start/stop/restart` 核心路径，依赖新增测试保护；若测试发现行为回归，回滚 generation 字段相关改动。
- IPC 重命名若遗漏引用会导致 supervisor 状态查询失败，需 grep 全仓确认。
- `RuntimeLogEntry` 加 `id` 字段会波及多个测试 fixture，需逐一更新避免 typecheck/测试失败。
