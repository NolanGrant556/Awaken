# AwakenPluginCard

插件页列表卡。展示插件名、版本、描述或诊断、组件计数和 compact 启停开关。卡片主体只负责打开详情；开关是独立兄弟操作。

## Props

- `plugin`：UI-only 插件摘要，包括名称、版本、描述、skill/server 计数、启用状态、忙碌和诊断。
- `onOpen`：点击卡片主体时触发。
- `onEnabledChange`：开关变化时回传下一个启用状态。

没有 `onOpen` 时主体退化为静态内容；没有 `onEnabledChange` 时开关显示但不可操作。组件不读取 runtime 或插件目录。
