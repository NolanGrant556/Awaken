# Tasks

- [x] Task 1：新增 run-narrative 类型与 projector
  - [x] 新增 `apps/desktop/src/renderer/run-narrative/types.ts`
  - [x] 新增 `apps/desktop/src/renderer/run-narrative/projector.ts`
  - [x] 定义 committed/active/inspector selection 模型
  - [x] 投影层只 import renderer shared AG-UI type，不 import runtime client、Electron、Go runtime 或 `@awaken/ui`
  - Task Dependencies：无

- [x] Task 2：接入 runtime reducer
  - [x] `RuntimeViewState` 新增 `narrative`
  - [x] `ThreadViewState` 保存/恢复 narrative
  - [x] `run/requested` 初始化 prompt + planning
  - [x] `run/event` 通过 projector 更新 narrative
  - [x] replay 同一批事件不重复插入 narrative item
  - Task Dependencies：Task 1

- [x] Task 3：Run Stage 渲染 narrative
  - [x] `App.tsx` 用 committed items + active item 渲染中心 narrative list
  - [x] 删除中心 standalone answer/thinking cards
  - [x] 保留 Inspector 中 runtime/workspace/approval/artifact/tool 摘要
  - [x] 长文本在 narrative item 中可换行或滚动，不撑破布局
  - Task Dependencies：Task 2

- [x] Task 4：测试覆盖
  - [x] reducer 测试覆盖 `run/requested`
  - [x] reducer 测试覆盖 reasoning/text/tool/approval/artifact/run finish
  - [x] reducer 测试覆盖 replay/idempotency
  - [x] 现有 runtime reducer 测试继续通过
  - Task Dependencies：Task 2

- [x] Task 5：验证与 review
  - [x] `npm --workspace @awaken/desktop run test`
  - [x] `npm run typecheck:ui`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 视觉检查 narrative list 无明显重叠
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3、Task 4
