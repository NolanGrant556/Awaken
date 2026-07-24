# AwakenSkillCard

所有来源共用的 Skill 卡片。卡片主体负责请求 `SKILL.md` 预览，右侧原子 `AwakenSwitch` 独立请求启停；由安装生命周期拥有的来源还可注入独立删除操作，三种操作不会互相触发。

## Props

- `skill`：UI-only Skill 数据，包括名称、描述、来源、启用状态、诊断和可预览状态。
- `onPreview`：点击可预览的卡片主体时触发。
- `onEnabledChange`：开关变化时回传下一个启用状态。
- `onDelete`：存在时显示删除按钮并回传删除请求；组件不判断来源权限。
- `deleting`：删除进行中时禁用卡片操作并显示忙碌文案。

没有 `onPreview` 或 `previewable=false` 时，卡片主体退化为静态内容；没有 `onEnabledChange` 时，开关显示但不可操作；没有 `onDelete` 时完全不渲染删除入口。
