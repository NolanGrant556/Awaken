# AwakenRunPlan

纯 UI 的运行计划/任务进度组件族，对应 `docs/02_主智能体闭环/运行计划与任务进度设计契约.md` 的目标交互。组件只接收 items、计数、`open` 与回调，不读取 runtime、Electron、SSE 或 thread；窄 DTO `AwakenRunPlanItemView`（`step` / `activeForm` / `status`）在 DS 内重新声明，由 renderer 负责投影，不得 import renderer/shared。三个导出：

- `AwakenRunPlanTrigger`：ComposerDock stack 内、ComposerBar 上方的紧凑入口。受控 `open` + `onToggle`；文案 `第 N/M 步 · 当前 activeForm（缺失回退 step）`，全部完成时 `M/M · 计划已完成`；aria-label 包含完成数、总数与当前步骤。进行中为旋转圆环、全部完成为勾选、其余为空心圆，`prefers-reduced-motion` 下旋转静止。
- `AwakenRunPlanPanel`：RunStage.overlay 内的只读展开面板，`role="dialog"`；外层 wrapper `pointer-events:none`、本体 `pointer-events:auto`。宽度 `min(640px, 100%)`，高度上限 `min(360px, 100%)`，超出只滚 body（窄 thumb 滚动条）；每步同时用图标与可见状态词（已完成/进行中/待处理）表达三态；无勾选/重排/删除交互。Escape、外点关闭、焦点恢复与底部定位（composer inset + 12px）由调用方编排，组件不写死定位。
- `AwakenRunPlanHistorical`：执行过程内的只读终态快照，标题 `执行计划 · N/M` + 同款步骤列表，无动画、无交互，保留 run terminal 时的真实三态。

- `AwakenRunPlan.tsx`：三组件与共享只读步骤列表。
- `AwakenRunPlan.css`：基于语义 token 的亮暗主题、面板内滚动与 reduced-motion 回退。
- `AwakenRunPlan.states.ts`：进行中、首步、末步、全部完成、含 explanation、长步骤换行/溢出滚动六态。
- `AwakenRunPlan.demo.tsx`：`ads-workbench-scene` 真实位置场景（trigger 在 composer 上方、panel 在其上方浮层）+ States 遍历与历史变体。
