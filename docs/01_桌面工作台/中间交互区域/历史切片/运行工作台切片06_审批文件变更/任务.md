# Tasks

- [x] Task 1：UI components
  - [x] `ApprovalCard` 支持 inline narrative 模式，默认 modal 不变
  - [x] 新增 `packages/ui/src/components/narrative/FileChangeSummary.tsx`
  - [x] 导出 `FileChangeSummary`
  - [x] 新 UI 仍保持 props-only，不 import runtime/Electron/AG-UI/A2UI
  - Task Dependencies：无

- [x] Task 2：Desktop approval narrative
  - [x] approval waiting item 使用 inline `ApprovalCard`
  - [x] approve/reject 调用现有 `handleResolveApproval`
  - [x] approved/rejected item 不显示可点击按钮
  - [x] 移除重复的全局 approval modal 渲染
  - Task Dependencies：Task 1

- [x] Task 3：Desktop file change narrative
  - [x] fileChange item 使用 `FileChangeSummary`
  - [x] artifact path/title/status 可见
  - [x] Inspector artifact list 继续可见
  - Task Dependencies：Task 1

- [x] Task 4：测试与验收
  - [x] reducer 测试覆盖 approval resolved approved/rejected
  - [x] reducer 测试覆盖 cancel/stream error 后 approvals 清空且 narrative 不再可交互
  - [x] reducer 测试覆盖 artifact path 进入 fileChange
  - [x] `npm --workspace @awaken/desktop run test`
  - [x] `npm run typecheck:ui`
  - [x] `npm run typecheck:desktop`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 真实 LLM mutation approval 或 artifact smoke 通过
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3
