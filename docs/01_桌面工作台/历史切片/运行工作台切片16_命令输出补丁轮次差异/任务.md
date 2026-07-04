# Tasks

- [x] 新增 command output / patch updated / turn diff 内部事件与 payload 字段
- [x] 为 shell 增加 output delta observer 与截断策略
- [x] 为 apply_patch 审批前发出 patch updated preview
- [x] run 完成时发出 bounded turn diff
- [x] AG-UI mapper/shared types 映射三类事件
- [x] Desktop reducer/App 展示 Terminal output、patch preview、turn diff
- [x] 增加 shell/apply_patch/runtime/mapper/reducer 测试
- [x] 跑 Go tests、Desktop tests/typecheck/build、真实 mutation smoke、diff check
- [x] 独立 review 并修复 findings

## Task Dependencies

- 内部事件与 shared payload 先于 Desktop reducer。
- shell observer 先于 command output reducer 测试。
- patch updated 必须在 approval wait 之前可观察。
- turn diff 需要在 mutation/approval 路径可验证后再接入 run finish。
- 全部测试和真实 smoke 完成后再勾 checklist。
