# Tasks

- [x] Task 1：RuntimeInfo 暴露 protocol
  - [x] SubTask 1.1：在 `src/server/runtime_info.go` 的 `RuntimeInfo` 结构体新增 `Protocol string json:"protocol"` 字段
  - [x] SubTask 1.2：`llmRuntimeInfoProvider` 新增 `protocol` 字段，`GetRuntimeInfo()` 返回它
  - [x] SubTask 1.3：`NewLLMRuntimeInfoProvider` 签名新增 `protocol` 参数
  - [x] SubTask 1.4：在 `cmd/awaken-core/main.go` 调用处传入 `string(llm.ResolveProtocol(cfg))`（或等价解析后的 protocol）
  - [x] SubTask 1.5：更新 `src/server/server_test.go` 中 `StaticRuntimeInfoProvider` 用例和 runtime info 断言，验证 `protocol` 字段

- [x] Task 2：结构化 provider 错误码
  - [x] SubTask 2.1：在 `src/agentcore/runner.go` 的 `runEventEmitter.runErrored` 中，用 `errors.Is(err, llm.ErrMissingAPIKey)` 判断并设置 `Code: "PROVIDER_AUTH_MISSING"`；用 `errors.Is(err, llm.ErrRequestFailed)` 判断并设置 `Code: "PROVIDER_REQUEST_FAILED"`
  - [x] SubTask 2.2：保留原有 `Error` 文本，但确保文本中不包含 API key（`llm.ErrMissingAPIKey` 已是固定文案，请求失败 detail 来自 provider body，需确认不含 key）
  - [x] SubTask 2.3：在 `src/agentcore/runner_test.go` 新增测试：缺 key 时 emit 的 RunError 事件 `Code` 为 `PROVIDER_AUTH_MISSING`；请求失败时为 `PROVIDER_REQUEST_FAILED`

- [x] Task 3：UI 共享类型扩展
  - [x] SubTask 3.1：在 `apps/desktop/src/shared/runtime.ts` 的 `RuntimeInfoWire` 新增 `protocol: string`，`RuntimeInfo` 新增 `protocol: string`
  - [x] SubTask 3.2：`normalizeRuntimeInfo` 映射 `protocol` 字段
  - [x] SubTask 3.3：更新相关 client/reducer 测试中构造 `RuntimeInfoWire` 的 fixture（经确认无 fixture 需更新）

- [x] Task 4：UI 顶部 Ready / Not ready 状态
  - [x] SubTask 4.1：在 `apps/desktop/src/renderer/App.tsx` 顶部状态区，根据 `runtimeInfo?.hasApiKey` 显示 `Ready` 或 `Not ready` pill
  - [x] SubTask 4.2：`hasApiKey=false` 时在发送按钮附近提示用户配置 `LLM_API_KEY`（不阻止点击，但给出可见提示；实际错误由 run error 路径处理）
  - [x] SubTask 4.3：展示 `runtimeInfo.protocol`（与 model 名并列或紧邻），让用户知道当前协议

- [x] Task 5：UI 按 code 显示可读 provider 错误
  - [x] SubTask 5.1：在 `apps/desktop/src/renderer/runtime/reducer.ts` 的 run error 处理分支，根据 `action.event.code` 映射可读描述：`PROVIDER_AUTH_MISSING` → "API key 缺失，请在 .env 中配置 LLM_API_KEY"；`PROVIDER_REQUEST_FAILED` → "Provider 请求失败：" + message
  - [x] SubTask 5.2：在 `apps/desktop/src/renderer/runtime/reducer.test.ts` 新增用例验证两种 code 的描述映射
  - [x] SubTask 5.3：确认错误展示不回显 API key（断言 message 不含 key 片段）

- [x] Task 6：真实 LLM smoke 与 failure path 验收
  - [x] SubTask 6.1：有 key 路径：发起一次真实 run（如 "请只回复 pong"），验证 status 显示当前 configured model、protocol，run 成功
  - [x] SubTask 6.2：无 key 路径：临时清空 `LLM_API_KEY` 启动 runtime，发起 run，验证 RunError 事件 `code=PROVIDER_AUTH_MISSING`，UI 显示可读提示
  - [x] SubTask 6.3：请求失败路径：配置一个无效 base_url（如有 key 但指向不可达地址），发起 run，验证 `code=PROVIDER_REQUEST_FAILED`，UI 可读
  - [x] SubTask 6.4：验证 status 响应不含 secret（grep 响应 body 不含 `LLM_API_KEY` 值）

- [x] Task 7：验证与验收
  - [x] SubTask 7.1：运行 `go test ./src/server/... ./src/agentcore/...` 通过
  - [x] SubTask 7.2：Desktop build 与 type check 通过（`pnpm --filter desktop build` 或等价命令）
  - [x] SubTask 7.3：代码审查确认 protocol 链路、错误码映射、UI 状态展示完整

# Task Dependencies

- Task 1 与 Task 2 可并行（分别改 server/runtime_info 与 agentcore/runner）
- Task 3 依赖 Task 1（需要 protocol 字段定义）
- Task 4 依赖 Task 3（需要 UI 类型扩展）
- Task 5 依赖 Task 2（需要 error code 在事件中产生）和 Task 3
- Task 6 依赖 Task 1、Task 2、Task 4、Task 5
- Task 7 依赖 Task 1-6
