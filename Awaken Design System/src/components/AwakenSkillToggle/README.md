# AwakenSkillToggle

Skill 启停状态的兼容语义外壳，视觉和交互统一委托给原子组件 `AwakenSwitch`。组件只展示状态并通过 `onCheckedChange` 请求变更，不调用 runtime 或持久化接口。

## Props

- `checked`：当前启用状态。
- `label`：用于生成“启用/停用 Skill 名称”的可访问标签。
- `busy`：处理中，禁用交互并展示轻量动效。
- `onCheckedChange`：请求切换时回传下一个布尔值。
- 其余按钮属性透传。

组件使用 `role="switch"`、`aria-checked` 和 `aria-busy`，并支持键盘焦点及 `prefers-reduced-motion`。
