# Slice 9: Bottom Composer、Stop 与 Follow-up

## 目标

底部 composer 始终可见；run 中主按钮变为 stop 并取消当前 run；run 完成后再次输入会作为同一 thread 的 follow-up。

## 范围

- `packages/ui` 收紧 `Composer` / `SendButton` 的主按钮状态，增加 UI-only 的 send/stop 展示能力。
- Desktop busy 状态下 composer 主按钮调用现有 cancel flow。
- Desktop done/cancelled/error 后再次提交仍携带当前 active thread id，形成 follow-up。
- 保持空输入不能提交，run 中不排队 follow-up。
- 覆盖 cancel/follow-up 相关状态测试，并做视觉验收。

## 验收标准

- run 中 composer 主按钮显示 stop，并调用 runtime cancel。
- cancel 后 UI 状态为 cancelled，不显示 failed。
- run 完成后再次提交作为同一 thread follow-up。
- composer 固定在底部，不被中心 narrative 遮挡。
- Cmd/Ctrl+Enter 在可提交时触发 submit，空输入仍不能提交。

## 暂不做

- 不做 run 中排队 follow-up。
- 不做语音输入。
- 不做富文本 prompt 编辑器。
