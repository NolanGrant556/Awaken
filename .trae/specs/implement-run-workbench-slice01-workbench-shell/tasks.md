# Tasks

- [x] Task 1：重构 Desktop 主布局
  - [x] 将 `App.tsx` 主区域改为 Run Workbench 三栏加底部 Composer
  - [x] 保留 `Sidebar` 的 runs/thread/workspace 入口
  - [x] 保留 `TopBar` 中必要 runtime/provider/workspace/cancel 控件
  - [x] 删除中心主路径里的 hero、assistant/reasoning debug panels、tool result debug list、artifact debug section
  - Task Dependencies：无

- [x] Task 2：建立 Inspector Rail
  - [x] 右侧展示 runtime/model/provider 状态摘要
  - [x] 右侧展示 workspace、approval、artifact、tool result 摘要
  - [x] 右侧展示 latest artifact / A2UI surface 的只读预览入口
  - [x] 不把 AG-UI、A2UI、SSE、runtime client 或 Electron 依赖引入 `packages/ui`
  - Task Dependencies：Task 1

- [x] Task 3：保留 run 操作链路
  - [x] Composer 仍调用既有 `handleSubmit`
  - [x] workspace validation、thread reuse、subscribe、localStorage recovery 保持不变
  - [x] runtime starting/crashed 状态下禁用 run/workspace 操作
  - [x] approval approve/reject 弹窗保持可用
  - Task Dependencies：Task 1

- [x] Task 4：响应式和视觉验收
  - [x] 宽屏下左 Sidebar、中 Run Stage、右 Inspector、底 Composer 不重叠
  - [x] 窄窗口下 Inspector 不挤破中心内容
  - [x] 长 workspace、长 prompt、长 tool result 文本可换行或截断
  - [x] 第一屏不出现 Dashboard、卡片墙、开发者日志面板或 Phase 4 debug 文案
  - Task Dependencies：Task 1、Task 2

- [x] Task 5：验证与 review
  - [x] `npm run typecheck:ui`
  - [x] `npm run build:desktop`
  - [x] `git diff --check`
  - [x] 进行一次独立 review pass
  - [x] 修复 review findings 后重新验证和复审
  - Task Dependencies：Task 1、Task 2、Task 3、Task 4
