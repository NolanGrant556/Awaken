# Awaken Desktop Workbench Review Checklist

本清单用于 PR review，判断改动是否触碰 [Workbench Contract](workbench-contract.md) 中定义的中间区域边界。只有当清单中所有相关项都通过时，才允许合并。

## 使用方式

- 每个 PR 的 reviewer 根据改动范围，勾选或跳过相应条目；
- 如果某条目被触发但无法勾选，必须在 PR 中说明原因；
- 本清单与 `workbench-contract.md` 一一对应，改动契约时必须同步更新本清单。

---

## 结构改动

- [ ] 是否改动 `AppShell`、`WorkbenchShell`、`RunStage`、`ExecutionFeed`、`ComposerDock` 的 props、结构或样式？
  - 如果“是”：是否仍遵守 `RunStage` 是唯一滚动/定位/overlay/composer 容器的契约？
  - 如果“是”：是否附真实运行截图 / 录屏？

- [ ] 是否在 `RunStage` 外部新增 `position: fixed` 或 `position: absolute` 的中间区域元素？
  - 如果“是”：必须说明为何无法通过 `RunStage` 现有插槽实现，并更新 Workbench Contract。

- [ ] 是否修改 `desktop-root`、`desktop-main` 或 Workbench 三栏布局相关样式？
  - 如果“是”：是否验证 sidebar / inspector 折叠/展开后中间区域仍正常？

---

## 样式边界

- [ ] 是否修改 `styles.css` 中的 Workbench 中间区域样式边界？
  - 触发前缀：`.aw-run-stage-*`、`.run-stage-*`、`.execution-feed`、`.execution-turn-*`、`.execution-prompt-*`、`.composer-*`、`.turn-marker-*`、`.narrative-*`。
  - 如果“是”：是否确认改动没有破坏空态 center composer、运行态 dock composer、turn 分组、prompt header、marker rail 或 narrative item 的布局？

- [ ] 是否通过 Desktop renderer 覆写了本应由 `@awaken/design-system` 拥有的结构类或 token？
  - 如果“是”：是否说明为什么这是场景级覆写，而不是设计系统层的通用改动？

---

## Composer 形态

- [ ] 是否新增或修改 `center` / `dock` 之外的 composer 形态？
  - 如果“是”：必须更新 Workbench Contract，并附空态与运行态截图。

- [ ] 是否修改 `shouldUseCenterComposer` 的决策逻辑或入参？
  - 如果“是”：是否同步更新 `App.test.ts` 中的对应单测？

- [ ] 是否修改 `ComposerBar` 的 `variant="center"` 或 `variant="dock"` 使用方式？
  - 如果“是”：是否确认两种形态下的视觉与交互仍符合产品契约？

---

## Narrative / Turn 区域

- [ ] 是否在 `ExecutionFeed` 内添加非 narrative 的 UI 控件（按钮、输入框、通知横幅、业务控制面板等）？
  - 如果“是”：必须说明理由，并考虑将其移到 `RunStage` 的 overlay / composer 插槽或 inspector 区域。

- [ ] 是否修改 `ExecutionTurnSection`、`UserPromptBlock` 或 narrative item 的渲染结构？
  - 如果“是”：是否附运行态截图，确认 turn 分组、prompt header、item 间距仍正常？

- [ ] 是否修改 narrative 数据分组或 `groupNarrativeByTurns` 相关逻辑？
  - 如果“是”：是否同步更新对应单测？

---

## 滚动与锚定

- [ ] 是否修改 `App.tsx` 中的滚动、wheel / touch、anchor 同步、`scrollPromptToAnchor()` 等编排逻辑？
  - 如果“是”：是否附录屏或关键帧截图，展示最新 turn 自动 follow、用户滚动暂停、恢复等行为？

- [ ] 是否修改 `workbenchPresentation.ts` 中的纯决策函数（`scrollTopForPromptAnchor`、`latestTurnMinHeightBottomReserve`、`shouldPauseLatestOutputFollowForWheel`、`shouldResumeLatestOutputFollow` 等）？
  - 如果“是”：是否同步补充或更新 `App.test.ts` 中的对应单测？

- [ ] 是否修改 `turnMarkerRailDisplay` 或 `shouldShowTurnMarkerRail` 的决策逻辑？
  - 如果“是”：是否同步更新 `App.test.ts` 中的对应单测？

- [ ] 是否改动 turn marker rail 的 DOM 结构或样式？
  - 如果“是”：是否附运行态截图，确认 marker 点击跳转、active 状态、compact/summary 模式仍正常？

---

## 测试与验收

- [ ] 是否附真实运行截图 / 录屏？
  - 至少包含：空态 center composer、运行态 dock composer + `ExecutionFeed`、涉及改动场景的特写。

- [ ] 是否修改或新增 `App.test.ts` 中的测试？
  - 如果改动了 `workbenchPresentation.ts`：必须更新。
  - 如果改动了中间区域结构但未涉及纯决策函数：建议新增渲染或行为测试。

- [ ] 是否运行过本地 smoke 测试，确认 Awaken Desktop 能正常启动并进入中间区域？
  - 参考入口：`README.md` 中的“本地运行”说明。

---

## 契约同步

- [ ] 是否修改 [Workbench Contract](workbench-contract.md) 本身？
  - 如果“是”：契约变更是否与代码变更在同一 PR 中？
  - 如果“是”：是否在 PR 描述中明确说明修改理由和影响范围？

- [ ] 是否修改本 review checklist？
  - 如果 Workbench Contract 被修改，本清单通常也需要同步更新。

---

## 快速判断流程

如果 PR 完全不涉及 `App.tsx`、`workbenchPresentation.ts`、中间区域组件、或 `styles.css` 中 `.aw-run-stage-*` / `.run-stage-*` / `.execution-feed` / `.execution-turn-*` / `.execution-prompt-*` / `.composer-*` / `.turn-marker-*` / `.narrative-*` 等样式，可以只勾选：

- [ ] 本 PR 不涉及中间区域改动，跳过以上检查。

否则必须完整填写相关条目。
