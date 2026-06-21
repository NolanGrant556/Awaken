# Tasks

- [x] Task 1：新增 UI narrative components
  - [x] 新增 `packages/ui/src/components/narrative/PromptCard.tsx`
  - [x] 新增 `packages/ui/src/components/narrative/ActivityLine.tsx`
  - [x] 组件只通过 props 接收数据，不 import runtime/Electron/AG-UI/A2UI
  - [x] 长文本使用稳定换行或滚动约束
  - Task Dependencies：无

- [x] Task 2：导出与 playground mock 场景
  - [x] 从 `packages/ui/src/components/index.ts` 导出 `PromptCard`
  - [x] 从 `packages/ui/src/components/index.ts` 导出 `ActivityLine`
  - [x] playground 增加短 prompt、长 prompt、中文 prompt 场景
  - [x] playground 可切换 planning active/done
  - Task Dependencies：Task 1

- [x] Task 3：Desktop Run Stage 接入
  - [x] `App.tsx` 从 `@awaken/ui` 使用 `PromptCard`
  - [x] `App.tsx` 从 `@awaken/ui` 使用 `ActivityLine`
  - [x] prompt item 不再走 generic narrative article
  - [x] planning item 不再走 generic narrative article
  - [x] cancel/error/reconnect 后 prompt card 不因布局分支消失
  - Task Dependencies：Task 1、Task 2

- [x] Task 4：验证与 review
  - [x] `npm run typecheck:ui`
  - [x] `npm run typecheck:desktop`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 视觉检查短 prompt、长 prompt、中文 prompt 无明显重叠或溢出
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3
