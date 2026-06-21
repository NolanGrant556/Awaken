# Tasks

- [x] Task 1：UI inspector components
  - [x] 新增 `packages/ui/src/components/inspector/InspectorTabs.tsx`
  - [x] 新增 `packages/ui/src/components/inspector/InspectorRail.tsx`
  - [x] 导出 inspector 组件与必要类型
  - [x] 新 UI 保持 props-only，不 import runtime/Electron/AG-UI/A2UI
  - Task Dependencies：无

- [x] Task 2：Desktop inspector state
  - [x] Desktop 增加本地 active inspector tab state
  - [x] center tool/action/fileChange/approval item 点击后设置 inspector selection
  - [x] 右侧 Inspector 用 selection 展示 selected detail，而不是只显示 latest
  - [x] Browser tab 显示明确 disabled/empty state
  - Task Dependencies：Task 1

- [x] Task 3：Inspector detail panes
  - [x] Changes pane 显示 selected artifact/file change path、status/change type、summary
  - [x] Terminal pane 显示 selected tool args/result 或完成后的 result 摘要
  - [x] Files pane 显示 file-oriented tool output 或空状态
  - [x] Approval selection 显示 title、affected paths、patch summary
  - Task Dependencies：Task 1、Task 2

- [x] Task 4：测试与验收
  - [x] reducer/projector 测试覆盖 tool/fileChange/approval inspector selection
  - [x] desktop/UI typecheck 通过
  - [x] desktop test 通过
  - [x] desktop build 通过
  - [x] `git diff --check` 通过
  - [x] UI 视觉检查覆盖 inspector tabs 与 empty Browser
  - [x] 独立 review pass，修复后复审
  - Task Dependencies：Task 1、Task 2、Task 3
