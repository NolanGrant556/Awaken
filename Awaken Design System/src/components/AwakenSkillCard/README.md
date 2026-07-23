# AwakenSkillCard

所有来源共用的 Skill 卡片。卡片主体负责请求 `SKILL.md` 预览，右侧原子 `AwakenSwitch` 独立请求启停，两种操作不会互相触发。

## Props

- `skill`：UI-only Skill 数据，包括名称、描述、来源、启用状态、诊断和可预览状态。
- `onPreview`：点击可预览的卡片主体时触发。
- `onEnabledChange`：开关变化时回传下一个启用状态。

没有 `onPreview` 或 `previewable=false` 时，卡片主体退化为静态内容；没有 `onEnabledChange` 时，开关显示但不可操作。
