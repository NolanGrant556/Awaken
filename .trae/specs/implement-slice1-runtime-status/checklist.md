# Checklist

- [x] `GET /v1/runtime/info` 端点已实现并返回指定字段
- [x] Runtime info 响应中不包含 API key、完整 base URL 等 secret
- [x] `handleRuntimeInfo` 有 Go 单元测试覆盖
- [x] `apps/desktop/src/shared/runtime.ts` 中新增 `RuntimeInfo` / `RuntimeInfoWire` 与 `normalizeRuntimeInfo`
- [x] `AwakenBridge` 类型包含 `getRuntimeInfo` 方法
- [x] Electron main 进程新增 `awaken:get-runtime-info` IPC handler 代理到 runtime
- [x] Preload 脚本将 `getRuntimeInfo` 暴露到 `window.awaken`
- [x] `RuntimeClient` 类型与 electron/browser 实现均包含 `getRuntimeInfo`
- [x] App.tsx 启动后自动轮询 runtime 状态
- [x] TopBar 正确显示 `Connected` / `Disconnected` 状态 pill（已通过浏览器访问 renderer dev server 验证）
- [x] Connected 时显示 configured model（已通过浏览器访问 renderer dev server 验证）
- [x] Disconnected 时显示用户可见错误提示（已通过浏览器访问 renderer dev server 验证）
- [x] Runtime 从关闭到启动后，Desktop 自动恢复为 Connected（轮询逻辑与端点验证通过）
- [x] `go test ./src/server/...` 通过
- [x] Desktop build 与 type check 通过
- [x] 不启动 runtime 时 Desktop 不白屏、不崩溃（Disconnected 状态正常展示，无崩溃）
