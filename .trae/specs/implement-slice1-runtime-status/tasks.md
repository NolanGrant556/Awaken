# Tasks

- [x] Task 1：删除外部 runtime 产品分支
  - [x] 从 Electron main、preload、shared types、renderer client 中删除 Desktop override URL
  - [x] renderer 不再接收或展示 runtime URL/source
  - [x] 删除相关旧测试并改为验证 main-managed runtime

- [x] Task 2：收口 RuntimeSupervisor
  - [x] `RuntimeSupervisor.start()` 始终启动本机 `awaken-core`
  - [x] 自动分配 localhost 端口并等待 `/health`
  - [x] `before-quit` 停止子进程，确保退出后无残留

- [x] Task 3：统一开发入口
  - [x] 新增 ensure 脚本：当前平台 binary 缺失时自动 build
  - [x] 根 `npm run dev:desktop` 串联 UI build、binary ensure、Desktop 启动
  - [x] build/dist 路径在产物生成前构建 `awaken-core` binary

- [x] Task 4：简化 Desktop UI
  - [x] 启动中禁用 run/workspace 操作
  - [x] 启动失败或崩溃时只显示 `Awaken needs restart`
  - [x] 删除 runtime 来源、端口、日志、exit code、restart 按钮等工程细节

- [x] Task 5：文档同步
  - [x] README 主流程只保留 `npm run dev:desktop`
  - [x] Desktop README 删除手动启动 runtime 说明
  - [x] 单独 runtime 调试说明放入 runtime 开发文档

- [x] Task 6：验证
  - [x] `npm run typecheck:desktop`
  - [x] `npm --workspace @awaken/desktop run test`
  - [x] `go test ./src/llm/... ./cmd/awaken-core/... ./src/server/... ./src/runtime/...`
  - [x] `npm run acceptance:desktop-hardening`
  - [x] 不开额外终端，`npm run dev:desktop` 完成一次真实 run
  - [x] 不开额外终端，`Awaken.app` 完成一次真实 run
  - [x] 退出 Desktop 后无残留 `awaken-core` 进程
