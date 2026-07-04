# Tasks

- [x] Task 1：Runtime 支持配置目录与默认 workspace
  - [x] `llm.LoadConfig` 支持 `AWAKEN_CONFIG_DIR`
  - [x] `cmd/awaken-core` 支持 `AWAKEN_DEFAULT_WORKSPACE`

- [x] Task 2：构建 awaken-core binary
  - [x] 根 `build:binary` 输出平台 binary 到 `apps/desktop/resources/bin/<platform-arch>/`
  - [x] electron-builder 将 `resources/bin` 带入 app resources
  - [x] `resources/bin` 保持 git ignored

- [x] Task 3：RuntimeSupervisor 模块
  - [x] 分配本地端口并启动 `awaken-core`
  - [x] 等待 `/health` 后进入 running
  - [x] 失败时记录内部错误状态
  - [x] app 退出时终止子进程

- [x] Task 4：Renderer 只接收净化状态
  - [x] shared/preload 只暴露 supervisor `status`
  - [x] renderer 不接收 runtime URL、日志、exit code 或 spawn error
  - [x] UI 失败提示收敛为 `Awaken needs restart`

- [x] Task 5：验证
  - [x] supervisor 单元测试覆盖启动、失败、重试、crash、stop race
  - [x] renderer/client/reducer 测试覆盖净化后的状态形态
  - [x] typecheck、desktop tests、Go tests 通过
