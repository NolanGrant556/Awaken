# AwakenSettingsNav

设置页左侧一级导航。当前正式分区为模型、搜索、记忆、外观、宠物和 ACP，并使用共享图标与选中样式；组件只投影选中状态和点击回调，不拥有路由或设置数据。

`AwakenSettingsNavItems` 是这六个分区的唯一定义，Desktop 直接消费它而不自行追加条目，分区集合和图标才不会两边各改一次。ACP 的 runner 列表与编辑表单由 Desktop 拥有，设计系统只拥有它的导航入口。
