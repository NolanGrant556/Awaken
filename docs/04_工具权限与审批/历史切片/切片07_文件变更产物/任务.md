# Tasks

- [x] Task 1：apply_patch 实现 ArtifactProducingTool
  - [x] SubTask 1.1：在 `src/tools/apply_patch/apply_patch.go` 为 `ApplyPatchTool` 新增 `Artifacts(input, output string) []tool.ArtifactDescriptor` 方法
  - [x] SubTask 1.2：解析 input patch（复用 `parsePatch`），按 operation kind + movePath 产出 descriptor，kind 为 `added/modified/deleted/moved`
  - [x] SubTask 1.3：artifact path 使用相对 workspace 根的路径，与 approval affectedPaths 口径一致
  - [x] SubTask 1.4：Action 选择：added → `ArtifactActionCreated`，modified/deleted/moved → `ArtifactActionUpdated`
  - [x] SubTask 1.5：为 Artifacts 方法添加单测（Add/Update/Delete/Move 各一 + 多 operation 组合）

- [x] Task 2：扩展 UI ArtifactItem 类型与 ArtifactCard 组件
  - [x] SubTask 2.1：在 `packages/ui/src/types.ts` 的 `ArtifactItem` 新增 `changeType?: "added"|"modified"|"deleted"|"moved"`、`path?: string`、`kind?: string`、`status?: string`
  - [x] SubTask 2.2：在 `packages/ui/src/components/feedback/ArtifactCard.tsx` 按 changeType 切换图标与色调（Added 绿 FilePlus、Modified 蓝 FilePen、Deleted 红 FileMinus、Moved 紫 FileOutput）
  - [x] SubTask 2.3：ArtifactCard 展示 path 与简短描述
  - [x] SubTask 2.4：构建 `@awaken/ui` 确认类型与构建通过

- [x] Task 3：Desktop 映射 ArtifactRecord 到扩展后的 ArtifactItem
  - [x] SubTask 3.1：在 `apps/desktop/src/renderer/App.tsx` 将 reducer 的 `ArtifactRecord` 映射到扩展后的 `ArtifactItem`，传递 changeType/path/kind/status
  - [x] SubTask 3.2：从 kind 字段推断 changeType（`added/modified/deleted/moved`），不再把 kind/status/path 塞进 description
  - [x] SubTask 3.3：Desktop typecheck 通过

- [x] Task 4：Reject 不产 artifact 断言
  - [x] SubTask 4.1：在 `src/agentcore/tool_runtime_test.go` 补充 reject 不产 artifact 的断言

- [x] Task 5：真实 LLM smoke 测试
  - [x] SubTask 5.1：在临时 workspace 创建 `note.md`（内容为 `hello`）
  - [x] SubTask 5.2：使用真实 LLM 启动 run：输入"把 note.md 中 hello 改成 world"
  - [x] SubTask 5.3：Approve 后验证产生 `AWAKEN_ARTIFACT_CREATED` 或 `AWAKEN_ARTIFACT_UPDATED` 事件，kind 为 `modified`，path 含 `note.md`
  - [x] SubTask 5.4：验证 `note.md` 内容已变为 `world`

- [x] Task 6：验证与验收
  - [x] SubTask 6.1：运行 `go test ./src/tools/... ./src/agentcore/... ./src/server/...` 通过
  - [x] SubTask 6.2：Desktop build 与 type check 通过
  - [x] SubTask 6.3：代码审查确认 artifact 链路完整

# Task Dependencies

- Task 1 与 Task 2 可并行
- Task 3 依赖 Task 2（UI 类型扩展后才能映射）
- Task 4 依赖 Task 1
- Task 5 依赖 Task 1、Task 2、Task 3
- Task 6 依赖 Task 1、Task 2、Task 3、Task 4、Task 5
