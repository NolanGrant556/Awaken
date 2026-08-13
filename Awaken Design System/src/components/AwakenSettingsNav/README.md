# AwakenSettingsNav

设置页左侧一级导航。当前正式分区为模型、搜索、记忆、外观、宠物、计算机操作、Hook 和 ACP，其中 Hook 的用户可见文案显示为“钩子”；组件只投影选中状态和点击回调，不拥有路由或设置数据。

`AwakenSettingsNavItems` 是这八个分区的唯一定义，Desktop 直接消费它而不自行追加条目，分区集合和图标才不会两边各改一次。Hook 状态/信任/详情面板和 ACP runner 列表/编辑表单由 Desktop 拥有，设计系统只拥有它们的导航入口。
