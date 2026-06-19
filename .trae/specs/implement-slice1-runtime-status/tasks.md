# Tasks

- [ ] Task 1：删除外部 runtime 产品分支
  - [ ] 从 Electron main、preload、shared types、renderer client 中删除 Desktop override URL
  - [ ] renderer 不再接收或展示 runtime URL/source
  - [ ] 删除相关旧测试并改为验证 main-managed runtime

- [ ] Task 2：收口 RuntimeSupervisor
  - [ ] `RuntimeSupervisor.start()` 始终启动本机 `awaken-core`
  - [ ] 自动分配 localhost 端口并等待 `/health`
  - [ ] `before-quit` 停止子进程，确保退出后无残留

- [ ] Task 3：统一开发入口
  - [ ] 新增 ensure 脚本：当前平台 binary 缺失时自动 build
  - [ ] 根 `npm run dev:desktop` 串联 UI build、binary ensure、Desktop 启动
  - [ ] build/dist 路径在产物生成前构建 `awaken-core` binary

- [ ] Task 4：简化 Desktop UI
  - [ ] 启动中禁用 run/workspace 操作
  - [ ] 启动失败或崩溃时只显示 `Awaken needs restart`
  - [ ] 删除 runtime 来源、端口、日志、exit code、restart 按钮等工程细节

- [ ] Task 5：文档同步
  - [ ] README 主流程只保留 `npm run dev:desktop`
  - [ ] Desktop README 删除手动启动 runtime 说明
  - [ ] 单独 runtime 调试说明放入 runtime 开发文档

- [ ] Task 6：验证
  - [ ] `npm run typecheck:desktop`
  - [ ] `npm --workspace @awaken/desktop run test`
  - [ ] `go test ./src/llm/... ./cmd/awaken-core/... ./src/server/... ./src/runtime/...`
  - [ ] 不开额外终端，`npm run dev:desktop` 完成一次真实 run
  - [ ] 不开额外终端，`Awaken.app` 完成一次真实 run
  - [ ] 退出 Desktop 后无残留 `awaken-core` 进程
