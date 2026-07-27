# AwakenScheduledTaskEditorDialog

定时任务创建/编辑共用的受控编辑器，区块顺序固定为契约 §4.2：基本信息 → 运行位置 → 调度（循环 Cron / 日程 + IANA 时区）→ 任务内容（提醒/Agent）→ 模型（默认“每次运行时使用当前默认 profile”）→ 高级选项 → 取消/保存任务。表单值是 camelCase 的 `AwakenScheduledTaskDraft`，完全由父级持有（`value` + `onValueChange`），保存错误因此天然不丢表单；`fieldErrors` 就地展示，`errorMessage` 用 `role="alert"`。日程时间用 datetime-local 本地字符串 + `timezone` 表达，组件不做时区换算，也不包含 task_id/version 等 Core 概念。
