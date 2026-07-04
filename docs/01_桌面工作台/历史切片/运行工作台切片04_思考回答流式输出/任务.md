# Tasks

- [x] Task 1：新增 UI streaming components
  - [x] 新增 `packages/ui/src/components/narrative/ThinkingBlock.tsx`
  - [x] 新增 `packages/ui/src/components/narrative/MarkdownAnswer.tsx`
  - [x] `MarkdownAnswer` 支持 paragraph、list、inline code 基础渲染
  - [x] 组件只通过 props 接收数据，不 import runtime/Electron/AG-UI/A2UI
  - Task Dependencies：无

- [x] Task 2：导出与 playground mock 场景
  - [x] 从 `packages/ui/src/components/index.ts` 导出 `ThinkingBlock`
  - [x] 从 `packages/ui/src/components/index.ts` 导出 `MarkdownAnswer`
  - [x] playground 增加 thinking / markdown paragraph / list / inline code 场景
  - Task Dependencies：Task 1

- [x] Task 3：Desktop Run Stage 接入
  - [x] thinking item 使用 `ThinkingBlock`
  - [x] answer item 使用 `MarkdownAnswer`
  - [x] message end / run finish 后 answer 固化显示
  - [x] provider 缺失或 stream error 显示 error narrative item
  - Task Dependencies：Task 1、Task 2

- [x] Task 4：Live tail 滚动
  - [x] Run Stage 靠近底部时新内容自动滚动到底部
  - [x] 用户手动向上滚动后不强制抢回
  - [x] 组件卸载/切换 thread 时不保留错误滚动锁
  - Task Dependencies：Task 3

- [x] Task 5：测试与验收
  - [x] reducer 测试覆盖 `RUN_ERROR` 进入 error narrative item
  - [x] reducer 测试继续覆盖 replay/idempotency
  - [x] `npm --workspace @awaken/desktop run test`
  - [x] `npm run typecheck:ui`
  - [x] `npm run typecheck:desktop`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 真实 LLM pong smoke 通过
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3、Task 4
