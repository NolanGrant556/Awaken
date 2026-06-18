# Awaken Desktop v1 垂直切片开发清单

生成时间：2026-06-18

本文档用于指导 Awaken 第一版 Desktop 的开发。开发方式采用“垂直切片”，不按后端、前端、API、测试横向拆分。每个切片都必须从用户可见行为出发，贯穿 UI、Bridge/API、Runtime、Tool、安全边界与验收。

## 总原则

第一版 Desktop 的目标不是先做一个完整聊天壳，而是做一个可信的本地 agent 工作台：

```text
Awaken Desktop
  -> typed bridge
  -> awaken-core local runtime
  -> Agent Core
  -> LLM provider + local workspace tools
```

需要借鉴 Codex 和 Claude Code 的成熟做法，但不照搬它们的大系统：

- 借鉴：显式 workspace、分层权限、结构化审批、可恢复事件流、真实 runtime smoke。
- 借鉴：把 tool_use/tool_result pairing 做成 runtime 不变量，而不是只在请求前校验。
- 不照搬：云端任务、多用户、完整 MCP marketplace、重型 subagent/worktree、独立 exec-server 全量拆分。

每个切片完成后，都要能被真实运行验证。不能只交付“接口写好了”或“界面画好了”。

## Slice 1：打开 Desktop，确认 Runtime 可用

### 用户目标

用户打开 Awaken Desktop 后，能立即知道本地 runtime 是否可用，而不是点发送后才发现失败。

### 用户流程

1. 用户打开 Desktop。
2. 顶部状态区显示 `Connected` 或 `Disconnected`。
3. runtime 未启动时，界面显示明确错误。
4. runtime 启动后，Desktop 能恢复为 connected。

### 开发范围

- UI：顶部 runtime 状态、runtime 地址、基础错误提示。
- Bridge/API：新增或收口 `getRuntimeInfo()` / runtime status 调用。
- Runtime：返回 runtime version、API version、configured model、has_api_key、default workspace。
- Desktop main：保持代理 runtime HTTP/SSE，不在本切片启动 runtime 进程。

### 完成定义

- 不启动 `awaken-core`，启动 Desktop，不白屏、不崩溃，显示 disconnected。
- 启动 `go run ./cmd/awaken-core` 后，Desktop 能显示 connected。
- runtime status 不返回 secret，只能返回 `has_api_key` 和 base URL host 等摘要信息。
- 有用户可见状态变化，不能只在 console 里报错。
- renderer 只消费 runtime 状态，不承载 runtime policy。

### 暂不做

- 不做 bundled runtime 启动。
- 不做模型配置编辑器。
- 不做复杂 onboarding。

## Slice 2：选择 Workspace，并启动受限 Run

### 用户目标

用户选择一个 workspace 后，Awaken 的所有文件读写都被限制在该 workspace 内。

### 用户流程

1. 用户点击 workspace picker。
2. 选择 `/path/to/project`。
3. 输入任务并启动 run。
4. 模型只能读取或修改该 workspace 内的文件。

### 开发范围

- UI：workspace picker、当前 workspace 展示、workspace 缺失时的提示。
- Bridge/API：把 `createRun(input)` 升级为 `startRun(request)`。
- Runtime：`StartRunRequest` 支持 `workspace_root` 和 `writable_roots`。
- Agent Core：`AgentConfig.Workspace` 由 request 注入，不再只依赖 process cwd。
- Tools：`read_file`、`list_files`、`grep_search` 全部接入 `PermissionContext`。
- Safety：`writable_roots` v1 默认等于 `[workspace_root]`，且必须在 workspace 内。

### 建议请求形态

```json
{
  "input": "Summarize this project",
  "workspace": {
    "workspace_root": "/Users/nolangrant/Awaken",
    "writable_roots": ["/Users/nolangrant/Awaken"]
  },
  "client": {
    "session_id": "desktop-window-1",
    "request_id": "req_123",
    "entrypoint": "desktop"
  }
}
```

### 完成定义

- 真实 LLM run 中，请求读取 `/etc/hosts` 必须失败或进入显式风险审批，不能静默成功。
- 请求列出 `~/Desktop` 必须失败或进入显式风险审批。
- 请求读取当前 workspace 内 `README.md` 可以成功。
- 所有工具返回的相对路径都相对于 workspace。
- 有确定性测试覆盖 path normalize、workspace 内读取、workspace 外拒绝。
- 有真实 LLM smoke 覆盖至少一次 workspace 内只读任务。
- 不破坏 `packages/ui` 的 UI-only 边界。
- 不把 workspace policy 移入 renderer。

### 暂不做

- 不做多 workspace roots。
- 不做远程 workspace。
- 不做文件 watcher。

## Slice 3：普通问答 Run

### 用户目标

用户能发起一次不涉及工具的普通 LLM 对话，并看到稳定的 streaming 回复。

### 用户流程

1. 用户输入普通问题。
2. Desktop 显示 run running。
3. 模型返回 streaming 文本。
4. run 结束为 succeeded。

### 开发范围

- UI：输入框、发送按钮、streaming 回复区域、run 状态。
- API：`POST /v1/runs` 返回 Run。
- Runtime：创建 run、调用 LLM、发事件、结束 run。
- Event：`RUN_STARTED -> TEXT_MESSAGE_* -> RUN_FINISHED`。
- Error：空输入返回结构化错误。

### 完成定义

- 真实 LLM 输入“请只回复 pong”，最终只显示 `pong`。
- 该 run 不应触发工具调用、approval 或 artifact。
- 空输入返回 `INVALID_INPUT`，Desktop 显示可读错误。
- UI、Bridge/API、Runtime、LLM 形成一条真实链路。
- 有 failure path：空输入或 provider 缺失时不能崩溃。
- 不破坏 `packages/ui` 的 UI-only 边界。

### 暂不做

- 不做多轮 thread。
- 不做 markdown 高级渲染。

## Slice 4：只读文件任务

### 用户目标

用户让 Awaken 理解项目，模型能自主选择只读工具读取 workspace 内容。

### 用户流程

1. 用户输入“找出 README 并总结项目”。
2. 模型自主调用 `list_files` / `read_file` / `grep_search`。
3. Desktop 显示工具调用过程和结果摘要。
4. 最终回复可读。

### 开发范围

- UI：tool call inline 或 RunTimeline 中展示只读工具调用。
- Runtime：工具调用、tool result 回填、事件发射。
- Tools：只读工具全部 workspace guard。
- Agent Core：tool_use/tool_result 配对校验继续通过。

### 完成定义

- 真实 LLM 自主选择只读工具，不在 prompt 中点名具体工具。
- 出现 `TOOL_CALL_START` 和 `TOOL_CALL_RESULT`。
- 不出现 approval。
- 不允许 workspace 外读取。
- 有确定性测试覆盖只读工具的 workspace guard。
- 有 failure path：workspace 外 read/list/grep 必须失败或进入显式风险审批。
- Tool policy 仍在 Go runtime/tool 层，不移入 renderer。

### 暂不做

- 不做只读工具并发优化。
- 不做全量项目索引。

## Slice 5：文件修改审批

### 用户目标

用户让 Awaken 修改文件时，修改前必须看到审批，并能理解将要改什么。

### 用户流程

1. 用户输入“把 note.md 中 hello 改成 world”。
2. 模型选择 `apply_patch`。
3. Desktop 显示 ApprovalCard。
4. 用户点击 Approve。
5. 文件被修改，run 继续。

### 开发范围

- UI：ApprovalCard 显示 affected paths、risk tone、patch 摘要。
- Runtime：`apply_patch` 触发 approval wait。
- API：`POST /v1/runs/{run_id}/approvals/{approval_id}/resolve`。
- Event：显式状态 `running -> awaiting_approval -> running`。
- Tools：Approve 后执行 mutation。

### 完成定义

- 审批前文件不变。
- Approve 后文件内容符合 patch。
- 只产生预期最小 diff。
- `resolveApproval` 后 run 状态可恢复，不依赖 UI 猜测。
- 有 happy path：Approve 后 mutation 成功。
- 有 failure path：非法 patch、越界 writable root 或审批 API 错误能被清晰处理。
- 不把 approval policy 移入 renderer。

### 暂不做

- 不新增 `write_file` / `delete_file` / `move_file`。
- 不做复杂 diff editor。

## Slice 6：拒绝审批

### 用户目标

用户拒绝修改后，文件绝不能被改，agent 仍能得到一个合法 tool result。

### 用户流程

1. 用户触发一个文件修改任务。
2. Desktop 显示 ApprovalCard。
3. 用户点击 Reject。
4. 文件保持不变。
5. run 继续或结束，并给出可读解释。

### 开发范围

- UI：Reject 后禁用按钮，显示 rejected 状态。
- Runtime：拒绝后不执行 mutation。
- Agent Core：拒绝也必须生成合法 tool result，保持 tool_use/tool_result pairing。
- Event：`AWAKEN_APPROVAL_RESOLVED` 带 rejected decision。

### 完成定义

- Reject 后文件内容完全不变。
- 后续 LLM 请求不出现 orphan tool_use/tool_result。
- Desktop 不把 rejected 误显示成 runtime crash。
- 有真实或确定性 smoke 覆盖 Reject。
- 有 failure path：重复 resolve、未知 approval_id 返回结构化错误。
- mutation 是否执行由 runtime 决定，renderer 只提交 decision。

### 暂不做

- 不做“记住以后都拒绝/都允许”的持久策略。

## Slice 7：文件变更 Artifact

### 用户目标

文件修改完成后，用户能看到“改了哪些文件”。

### 用户流程

1. 用户 Approve 一个 `apply_patch`。
2. run 执行文件修改。
3. Desktop 的 changed files / artifact panel 显示 Added、Modified、Deleted 或 Moved。
4. 用户能点击查看简短摘要或 preview。

### 开发范围

- Runtime：`apply_patch` 实现 changed-file artifact 输出。
- Event：`AWAKEN_ARTIFACT_CREATED` / `AWAKEN_ARTIFACT_UPDATED`。
- UI：Artifact panel 按文件变更类型展示。
- Safety：artifact 是 runtime 可见记录，不等于可执行内容。

### 完成定义

- 创建文件后显示 Added。
- 修改文件后显示 Modified。
- 删除文件后显示 Deleted。
- artifact path 与实际 workspace 文件一致。
- 有 happy path：Approve 后产生 changed-file artifact。
- 有 failure path：Reject 后不产生文件变更 artifact。
- Artifact 只是 runtime 可见记录，不把 artifact 当作可执行动作。

### 暂不做

- 不做大文件内容内联。
- 不把 artifact 当作可信可执行动作。

## Slice 8：取消 Run

### 用户目标

用户能安全中断正在运行的任务。

### 用户流程

1. 用户启动一个长任务。
2. 点击 Cancel。
3. UI 进入 cancelling。
4. runtime 停止后续执行。
5. run 终态为 cancelled。

### 开发范围

- UI：Cancel 按钮、cancelling 状态、cancelled 终态。
- API：`POST /v1/runs/{run_id}/cancel`。
- Runtime：取消 context、停止等待中的 approval 或工具执行。
- Agent Core：取消时补齐 synthetic tool result，避免配对断裂。
- Event：新增或收口 `AWAKEN_RUN_STATUS`，避免把 cancelled 渲染成普通 error。

### 完成定义

- 长任务中 cancel 后，不再执行新工具。
- approval 等待时 cancel，文件不变。
- Desktop 显示 cancelled，不显示 failed。
- 有确定性测试或 smoke 覆盖 running cancel。
- 有 failure path：cancel 不存在的 run 返回结构化错误。
- cancel 语义由 runtime 状态机决定，renderer 不靠文本猜测。

### 暂不做

- 不做后台任务恢复。
- 不做自动重试。

## Slice 9：断线恢复

### 用户目标

runtime stream 断开或 Desktop 刷新后，用户不丢失当前 run 状态。

### 用户流程

1. 用户启动 run。
2. SSE 断开或 renderer 刷新。
3. Desktop 重新获取 run。
4. 从上次 `seq` 后继续订阅事件。
5. timeline、approval、artifact 不重复、不乱序。

### 开发范围

- API：`GET /v1/runs/{run_id}`。
- API：`GET /v1/runs/{run_id}/events?after_seq=n`。
- Event：每个事件有 `seq` 和 `event_id`。
- Runtime：history replay + live tail。
- UI：重连状态和恢复逻辑。

### 完成定义

- 断线重连后，事件不重复。
- pending approval 能恢复显示。
- artifact 不丢失。
- stream close 前无 terminal event 时，Desktop 显示可读恢复提示。
- 有 happy path：`after_seq` replay 后继续 live tail。
- 有 failure path：runtime 断连时 Desktop 不白屏、不崩溃。
- UI 只根据 runtime event/run state 恢复，不伪造状态。

### 暂不做

- 不做跨 runtime 重启持久化。
- 不做云同步。

## Slice 10：同一 Thread 多轮对话

### 用户目标

用户能在同一 thread 内连续追问，agent 保留上下文。

### 用户流程

1. 第一轮让 Awaken 创建或读取文件。
2. 第二轮输入“继续修改刚才那个文件”。
3. agent 能基于上一轮上下文继续工作。

### 开发范围

- UI：Conversation list、当前 thread 展示、新建 thread。
- API：`startRun({ thread_id })`。
- Runtime：按 `thread_id` 复用 Agent/Conversation。
- Agent Core：多轮消息与工具结果继续配对。
- Storage：v1 可先内存级，不跨 runtime 重启。

### 完成定义

- 第一轮创建 `a.txt`。
- 第二轮“追加 world”能识别上一轮文件。
- 未知 `thread_id` 返回 `THREAD_NOT_FOUND`，不静默新建。
- 有真实 LLM smoke 覆盖同一 thread 多轮。
- 有 failure path：未知 thread、跨 workspace thread 复用必须清晰失败。
- thread/context 归 runtime 管，不由 renderer 拼消息历史。

### 暂不做

- 不做长期历史数据库。
- 不做跨设备同步。

## Slice 11：模型状态与只读模型选择

### 用户目标

用户知道当前使用的模型和 provider 状态，不需要先做复杂配置系统。

### 用户流程

1. 用户打开 Desktop。
2. 顶部显示当前 model/provider。
3. 缺 key 时显示 not ready。
4. 有 key 时可以发起真实 run。

### 开发范围

- UI：model/provider 状态展示。
- Runtime：status 返回 configured model、protocol、has_api_key。
- Error：缺 key 返回 `PROVIDER_AUTH_MISSING`。
- Config：继续读取 `config.toml` 和 `.env`。

### 完成定义

- 缺 key 不能显示 ready。
- 缺 key 发 run 时有结构化错误。
- 有 key 时真实 LLM smoke 不 skip。
- 有 happy path：status 显示当前 configured model。
- 有 failure path：provider auth 缺失、provider 请求失败可读。
- 不把 secret 写入 renderer state、日志或文档。

### 暂不做

- 不做多 provider profile 管理。
- 不做 UI 内编辑 API key。
- 不做运行时热切换模型。

## Slice 12：Desktop 管理 awaken-core

### 用户目标

打包版用户双击 Awaken 后，不需要手动在终端启动 runtime。

### 用户流程

1. 用户启动 Awaken.app。
2. Electron main 启动 bundled `awaken-core`。
3. renderer 连接 runtime。
4. 用户直接发起 run。
5. 退出 app 后 runtime 子进程被清理。

### 开发范围

- Build：构建 bundled `awaken-core` binary。
- Electron main：启动/停止子进程。
- Runtime：支持固定或随机 localhost 端口。
- UI：runtime logs、restart runtime。
- Config：正式配置迁移到 Application Support。

### 完成定义

- 不开终端，启动 Desktop 后可完成一次真实 run。
- runtime crash 后 UI 显示错误并允许重启。
- 退出 app 后无残留 `awaken-core` 进程。
- 有 happy path：bundled runtime 完成一次真实 run。
- 有 failure path：runtime 启动失败、端口冲突、crash 能恢复或提示。
- runtime policy 仍在 Go runtime/tool 层，Electron main 只管理进程生命周期。

### 暂不做

- 不做 auto update。
- 不做 notarization。
- 不做云端登录。

## 推荐执行顺序

```text
1. Runtime 状态
2. Workspace 受限 Run
3. 普通问答 Run
4. 只读文件任务
5. 文件修改审批
6. 拒绝审批
7. 文件变更 Artifact
8. 取消 Run
9. 断线恢复
10. 同一 Thread 多轮对话
11. 模型状态与只读模型选择
12. Desktop 管理 awaken-core
```

最重要的是第 2 个切片。只要 workspace 没有贯通，文件 CRUD、审批、artifact 和 Desktop 工作台 UI 都没有可信基础。
