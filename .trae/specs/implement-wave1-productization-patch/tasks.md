# Tasks

- [x] Task 1: 侧栏会话标题与摘要展示
  - [x] SubTask 1.1: 在 `threadPresentation.ts` 或 App.tsx 增加 `threadDisplayTitle(thread)` 辅助函数，按 `displayTitle` → `aiTitle` → `customTitle` → `label` 取值
  - [x] SubTask 1.2: 侧栏 thread item 使用 `threadDisplayTitle`，并在其下渲染 `taskSummary` 摘要行（muted、单行截断）
  - [x] SubTask 1.3: 确认 `ProjectItem` / `Sidebar` 组件支持二级摘要行；若不支持，在 `packages/ui` 扩展可选 `description` 字段（不引入 runtime/Electron）

- [x] Task 2: 侧栏状态中文文案
  - [x] SubTask 2.1: 在 `threadPresentation.ts` 的 `describeThreadStatus` 把 label 改为中文：运行中 / 待输入 / 已取消 / 已完成 / 出错 / 待复核 / 空闲
  - [x] SubTask 2.2: 检查 `TaskCenterPanel` 等其它消费 `describeThreadStatus` 的位置，确保中文文案一致
  - [x] SubTask 2.3: 更新受影响的 renderer 测试断言

- [x] Task 3: run 终态后自动刷新侧栏
  - [x] SubTask 3.1: 在 reducer 或 App.tsx 的 `RUN_FINISHED` / `RUN_ERROR` / cancelled 处理路径里，在状态更新后触发一次 `refreshThreads()`
  - [x] SubTask 3.2: 避免重复刷新：同一 run 终态只触发一次，且不打断正在进行的 `refreshThreads`
  - [x] SubTask 3.3: 补测试覆盖 run 完成后侧栏 threads 被刷新

- [x] Task 4: 接入 LLM title generator
  - [x] SubTask 4.1: 在 `src/titlegen` 或 `cmd/awaken-core` 实现 LLM-backed `titlegen.Generator`（复用 `llm.LoadConfig()` 的 APIKey/BaseURL/Model）
  - [x] SubTask 4.2: 在 `cmd/awaken-core/main.go` 构造该 generator 并通过 `runtime.RunServiceConfig.TitleGenerator` 注入
  - [x] SubTask 4.3: 验证 `promptTitleGenerator` 不再作为活跃 generator 生效（除非 LLM 调用失败时作为兜底）
  - [x] SubTask 4.4: 真实 LLM smoke：完成一个 run 后 `GET /v1/threads` 返回的 thread `metadata.aiTitle` 是 LLM 总结而非 prompt 截断

- [x] Task 5: Inspector Context 面板
  - [x] SubTask 5.1: 把 Inspector "Files" tab 改名为 "Context"
  - [x] SubTask 5.2: 在切换 thread 或当前 thread run 终态后调用 `runtimeClient.getThreadContext(threadId)`，结果存入 reducer 状态
  - [x] SubTask 5.3: Context 面板默认展示上下文文件列表（path / source / tokens / last_read_time）+ usage 摘要（model、total_tokens / max_tokens、percentage、message_count）
  - [x] SubTask 5.4: 选中 tool result / artifact 时优先展示选中项（保留原 `renderFilesPane` 子视图），清空选中后回到 Context 总览
  - [x] SubTask 5.5: 空上下文展示 empty state
  - [x] SubTask 5.6: 加载中 / 出错状态展示

- [x] Task 6: Branch 按钮
  - [x] SubTask 6.1: 在 thread header 或 Composer 工具区加 "分支" 按钮，仅当当前 thread 有内容时启用
  - [x] SubTask 6.2: 点击调用 `runtimeClient.branchThread(threadId, { customTitle? })`，loading / error 状态可观测
  - [x] SubTask 6.3: 成功后把新 thread upsert 进 `runtimeState.threads`，并切换 active thread 到新 branch
  - [x] SubTask 6.4: 当前 thread 的 `metadata.branch` 在侧栏 item 或 Inspector 展示 "从 {parent label} 分支"

- [x] Task 7: 修 branch transcript 空白会话坑
  - [x] SubTask 7.1: 在 `replayThreadTranscript` / thread 切换路径里检测 `transcript.runs` 为空且 thread 有 `metadata.branch`
  - [x] SubTask 7.2: 满足条件时在会话区渲染 "从 {parent label} 分支" 来源卡片 + forked-from message 预览
  - [x] SubTask 7.3: 卡片提供 "查看来源会话" 操作，切换回父 thread
  - [x] SubTask 7.4: 当 branch thread 后续产生 run 后（`runIDs` 非空），不再展示来源卡片，正常 replay

- [x] Task 8: 验证与收口
  - [x] SubTask 8.1: `npm --workspace @awaken/desktop run test` 通过
  - [x] SubTask 8.2: `npm run typecheck:ui` 与 `npm run typecheck:desktop` 通过
  - [x] SubTask 8.3: `npm run build:desktop` 通过
  - [x] SubTask 8.4: `go test ./src/runtime/... ./src/titlegen/... ./src/server/...` 通过
  - [x] SubTask 8.5: 真实 LLM smoke：新建 run → 完成 → 侧栏显示中文状态 + LLM 标题 + 摘要；Inspector Context 显示文件与 usage；点分支 → 切到新 thread 显示来源卡片
  - [x] SubTask 8.6: `git diff --check` 通过

# Task Dependencies

- Task 4（LLM title generator）独立于前端任务，可与 Task 1–3 并行
- Task 1 依赖 Task 4 的产物才能在 smoke 里看到真实 LLM 标题，但渲染逻辑本身不阻塞
- Task 5 与 Task 6 / Task 7 互相独立，可并行
- Task 7 依赖 Task 6 已确认 branch metadata 结构
- Task 8 在所有功能任务完成后执行
