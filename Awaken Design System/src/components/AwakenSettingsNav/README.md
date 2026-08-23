# AwakenSettingsNav

设置页左侧一级导航。当前九个正式分区为模型、搜索、记忆、全局规则、外观、宠物、计算机操作、Hook 和 ACP，其中 Hook 的用户可见文案显示为“钩子”；组件只投影选中状态和点击回调，不拥有路由或设置数据。

`AwakenSettingsNavItems` 是这九个分区及其 concept 图标的唯一定义，Desktop 直接消费它而不自行追加条目；`AwakenSettings` 的右侧一级标题也从当前 item 继承同一 concept，不维护第二张图标表。Hook 状态/信任/详情面板和 ACP runner 列表/编辑表单由 Desktop 拥有，设计系统只拥有它们的导航入口。
