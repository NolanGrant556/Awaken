# AwakenRunPlan

纯 UI 的运行计划/任务进度组件族，对应 `docs/02_主智能体闭环/运行计划/实现方案.md` 的目标交互。组件只接收 items、计数、`open`、`paused` 与回调，不读取 runtime、Electron、SSE 或 thread；窄 DTO `AwakenRunPlanItemView`（`step` / `activeForm` / `status`）在 DS 内重新声明，由 renderer 负责投影，不得 import renderer/shared。两个导出：

- `AwakenRunPlanTrigger`：ComposerDock stack 内、ComposerBar 上方的紧凑入口。受控 `open` + `onToggle`；文案 `第 N/M 步 · 当前 activeForm（缺失回退 step）`，全部完成时 `M/M · 计划已完成`，暂停时 `已暂停 · activeForm`；aria-label 包含完成数、总数与当前步骤。进行中为旋转圆环、暂停时保留静态圆环、全部完成为勾选，`prefers-reduced-motion` 下旋转静止。调用方为短暂完成态传入 `onDismiss` 时，组件在主入口旁渲染独立的“隐藏已完成的运行计划”按钮；它不自行计时或删除快照。
- `AwakenRunPlanPanel`：RunStage.overlay 内的只读展开面板，`role="dialog"`；外层 wrapper `pointer-events:none`、本体 `pointer-events:auto`。宽度 `min(640px, 100%)`，高度上限 `min(360px, 100%)`，超出只滚 body（窄 thumb 滚动条）；每步同时用图标与可见状态词（已完成/进行中/已暂停/待处理）表达；无勾选/重排/删除交互。Escape、外点关闭、焦点恢复与底部定位（composer inset + 12px）由调用方编排，组件不写死定位。

- `AwakenRunPlan.tsx`：两组件与共享只读步骤列表。
- `AwakenRunPlan.css`：基于语义 token 的亮暗主题、面板内滚动与 reduced-motion 回退。
- `AwakenRunPlan.states.ts`：进行中、暂停、首步、末步、全部完成、含 explanation、长步骤换行/溢出滚动七态。
- `AwakenRunPlan.demo.tsx`：`ads-workbench-scene` 真实位置场景（trigger 在 composer 上方、panel 在其上方浮层）+ States 遍历。
