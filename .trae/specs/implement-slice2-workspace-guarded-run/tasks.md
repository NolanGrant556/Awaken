# Tasks

- [x] Task 1：升级 Runtime run 启动协议，支持 workspace
  - [x] SubTask 1.1：在 `src/runtime` 定义 `StartRunRequest`（含 input、workspace、client）
  - [x] SubTask 1.2：修改 `RunService.StartRun` 签名接收 `StartRunRequest` 并验证 workspace_root 非空且为绝对路径
  - [x] SubTask 1.3：将 request 的 workspace 注入 `AgentConfig.Workspace`，writable_roots 默认 `[workspace_root]`
  - [x] SubTask 1.4：更新 `src/server/handlers.go` 的 `handleCreateRun` 解析新请求体并调用 `StartRun`
  - [x] SubTask 1.5：更新 `cmd/awaken-core/main.go` 中构造 executor 的代码以接收 workspace
  - [x] SubTask 1.6：为 `RunService.StartRun` 添加 workspace 验证单元测试

- [x] Task 2：让 `read_file`、`list_files`、`grep_search` 接入 PermissionContext
  - [x] SubTask 2.1：`read_file` 实现 `tool.PermissionCheckingTool`，相对路径解析为 workspace 内绝对路径，越界返回 deny
  - [x] SubTask 2.2：`list_files` 实现 `tool.PermissionCheckingTool`，root 参数越界返回 deny，结果返回相对路径
  - [x] SubTask 2.3：`grep_search` 实现 `tool.PermissionCheckingTool`，root 参数越界返回 deny，结果返回相对路径
  - [x] SubTask 2.4：为三个工具添加 workspace 内/外读取的确定性测试
  - [x] SubTask 2.5：明确 `shell` 只读命令不属于 workspace 文件工具边界，允许读取 workspace 外信息；mutation/复杂命令仍走审批或拒绝

- [x] Task 3：更新 Electron bridge 支持 `startRun(request)`
  - [x] SubTask 3.1：在 `apps/desktop/src/shared/runtime.ts` 定义 `StartRunRequest` / `StartRunRequestWire` 与 `WorkspaceConfig` 类型
  - [x] SubTask 3.2：将 `AwakenBridge` 与 `RuntimeClient` 的 `createRun` 重命名为 `startRun`，参数改为 request 对象
  - [x] SubTask 3.3：更新 `apps/desktop/src/main/index.ts` 的 `awaken:create-run` handler 为 `awaken:start-run`，代理新协议
  - [x] SubTask 3.4：更新 `apps/desktop/src/preload/index.ts` 暴露 `startRun`
  - [x] SubTask 3.5：更新 `apps/desktop/src/renderer/runtime/client.ts` 的 browser/electron client 实现

- [x] Task 4：在 Desktop UI 增加 workspace picker
  - [x] SubTask 4.1：在 App.tsx 新增 workspace path state，默认值为当前目录或 `runtimeInfo.defaultWorkspace`
  - [x] SubTask 4.2：在 TopBar 或 Sidebar 增加 workspace picker 输入框与当前 workspace 展示
  - [x] SubTask 4.3：workspace 为空或非绝对路径时显示警告并禁用发送
  - [x] SubTask 4.4：点击发送时构造 `startRun` request 并传入 workspace 与 client 信息

- [x] Task 5：验证与验收
  - [x] SubTask 5.1：运行 `go test ./src/runtime/... ./src/server/... ./src/tools/...` 通过
  - [x] SubTask 5.2：Desktop build 与 type check 通过
  - [x] SubTask 5.3：真实 LLM smoke：请求读取 workspace 内 README.md 成功
  - [x] SubTask 5.4：确定性测试覆盖：`read_file /etc/hosts` 拒绝、`~/Desktop` list 失败、路径归一化正确
  - [x] SubTask 5.5：真实 LLM smoke：`shell` 只读读取 `/etc/hosts` 可成功，符合当前产品设计

- [x] Task 6：补齐 workspace 边界确定性测试
  - [x] SubTask 6.1：为 `read_file` 添加 `/etc/hosts` 越界读取测试与相对路径归一化测试
  - [x] SubTask 6.2：为 `list_files` 添加 `~/Desktop` 越界 root 测试与相对路径归一化测试
  - [x] SubTask 6.3：为 `grep_search` 添加 workspace 外 root 测试与相对路径归一化测试
  - [x] SubTask 6.4：运行 `go test ./src/tools/...` 确认新增测试通过

# Task Dependencies

- Task 2 depends on Task 1
- Task 3 depends on Task 1
- Task 4 depends on Task 3
- Task 5 depends on Task 2, Task 3, Task 4
