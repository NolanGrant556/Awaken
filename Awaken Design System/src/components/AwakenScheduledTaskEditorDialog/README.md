# AwakenScheduledTaskEditorDialog

定时任务创建/编辑共用的受控编辑器，区块顺序固定为契约 §4.2：基本信息 → 运行位置 → 调度（循环 Cron / 日程 + IANA 时区）→ 任务内容（提醒/Agent）→ 默认折叠的模型与高级选项 → 取消/保存任务。表单值是 camelCase 的 `AwakenScheduledTaskDraft`，完全由父级持有（`value` + `onValueChange`），保存错误因此天然不丢表单；`fieldErrors` 就地展示，`errorMessage` 用 `role="alert"`。

运行项目使用单一下拉选择器，候选项目在限高弹层中搜索和滚动，不会因项目数量增加而撑高表单。日程时间由 Awaken 中文日历与小时/分钟控件编辑，draft 仍使用 `YYYY-MM-DDTHH:mm` 本地字符串。时区选择器使用 Chromium/CLDR 的中文名称、中文区域分组与计划日期对应的中文偏移，IANA ID 只作为受控 value 和搜索别名；Renderer 仍负责将本地时间与 IANA value 换算为 RFC3339。组件不包含 task_id/version 等 Core 概念，也不改变创建、编辑或调度协议。
