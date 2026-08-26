# AwakenSkillCard

所有来源共用的紧凑 Skill 卡片。卡片固定为 `78px` 高，描述限制为单行，图标为 `36px`，开关使用 compact 规格；Awaken、项目、全局与 Agents 来源不得各自覆盖另一套卡片高度。来源以小字紧跟 Skill 名称，卡片不显示“可预览”提示和删除入口；卡片主体负责进入统一 Skill 包预览，右侧原子 `AwakenSwitch` 独立请求启停。

## Props

- `skill`：UI-only Skill 数据，包括名称、描述、来源、启用状态、诊断和可预览状态。
- `onPreview`：点击可预览的卡片主体时触发。
- `onEnabledChange`：开关变化时回传下一个启用状态。
没有 `onPreview` 或 `previewable=false` 时，卡片主体退化为静态内容；没有 `onEnabledChange` 时，开关显示但不可操作。删除属于预览窗口操作，不由小卡片承载。
