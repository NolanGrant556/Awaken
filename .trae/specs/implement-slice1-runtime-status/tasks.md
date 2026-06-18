# Tasks

- [x] Task 1：在 awaken-core 实现 `GET /v1/runtime/info` 端点
  - [x] SubTask 1.1：在 `src/server` 中新增 `runtimeInfoProvider` 接口与 `RuntimeInfo` 结构体
  - [x] SubTask 1.2：在 `cmd/awaken-core/main.go` 中组装运行时信息（version、api_version、configured_model、has_api_key、default_workspace、base_url_host）
  - [x] SubTask 1.3：在 `src/server/router.go` 注册 `GET /v1/runtime/info` 路由
  - [x] SubTask 1.4：在 `src/server/handlers.go` 实现 `handleRuntimeInfo` 处理器，确保不返回 secret
  - [x] SubTask 1.5：为 `handleRuntimeInfo` 添加 Go 单元测试，验证字段与不泄露 secret

- [x] Task 2：在 Electron main 与 preload 暴露 `getRuntimeInfo` bridge
  - [x] SubTask 2.1：在 `apps/desktop/src/shared/runtime.ts` 新增 `RuntimeInfoWire` 与 `RuntimeInfo` 类型及 `normalizeRuntimeInfo` 函数
  - [x] SubTask 2.2：在 `AwakenBridge` 类型中新增 `getRuntimeInfo: () => Promise<RuntimeInfo>`
  - [x] SubTask 2.3：在 `apps/desktop/src/main/index.ts` 新增 `awaken:get-runtime-info` ipcMain.handle，代理到 runtime `/v1/runtime/info`
  - [x] SubTask 2.4：在 `apps/desktop/src/preload/index.ts` 实现 `getRuntimeInfo` 并暴露到 `window.awaken`

- [x] Task 3：在 renderer runtime client 封装 `getRuntimeInfo`
  - [x] SubTask 3.1：在 `apps/desktop/src/renderer/runtime/client.ts` 的 `RuntimeClient` 类型新增 `getRuntimeInfo`
  - [x] SubTask 3.2：在 `createElectronClient` 中调用 `bridge.getRuntimeInfo`
  - [x] SubTask 3.3：在 `createBrowserClient` 中通过 `fetch` 调用 `/v1/runtime/info`

- [x] Task 4：在 App.tsx 中展示 `Connected`/`Disconnected` 状态并轮询
  - [x] SubTask 4.1：新增 state 保存 `runtimeInfo` 与 `connectionError`
  - [x] SubTask 4.2：在 `useEffect` 中启动定时轮询（建议 3 秒间隔），调用 `runtimeClient.getRuntimeInfo()`
  - [x] SubTask 4.3：TopBar 状态 pill 根据连接结果显示 `Connected` / `Disconnected`
  - [x] SubTask 4.4：Connected 时展示 configured model；Disconnected 时展示可读错误提示
  - [x] SubTask 4.5：组件卸载时清理轮询定时器

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/server/...` 通过
  - [x] SubTask 5.2：运行 Desktop build 与 type check 通过
  - [x] SubTask 5.3：不启动 runtime 时启动 Desktop，确认显示 `Disconnected`、无白屏崩溃（已通过 renderer dev server + 浏览器验证）
  - [x] SubTask 5.4：启动 `go run ./cmd/awaken-core` 后，确认 Desktop 自动显示 `Connected` 与模型信息（已通过 renderer dev server + 浏览器验证）

# Task Dependencies

- Task 2 depends on Task 1
- Task 3 depends on Task 2
- Task 4 depends on Task 3
- Task 5 depends on Task 4
