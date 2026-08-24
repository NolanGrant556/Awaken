# AwakenSettingsNav

设置页左侧一级导航。当前正式分区为模型、搜索、记忆、全局规则、外观、宠物、计算机操作、Hook 和 ACP，其中 Hook 的用户可见文案显示为“钩子”；固定末项是“已归档的聊天”，它通过 `groupLabel: "已归档"` 与普通分区隔开。组件只投影选中状态和点击回调，不拥有路由或设置数据。

`AwakenSettingsNavItems` 是这些分区的唯一定义，Desktop 直接消费它而不自行追加条目，分区集合和图标才不会两边各改一次。Hook 状态/信任/详情面板和 ACP runner 列表/编辑表单由 Desktop 拥有，设计系统只拥有它们的导航入口。

条目可声明可选的 `groupLabel`：当某一项的 `groupLabel` 与前一项不同，组件在该条目上方渲染分组标题。导航容器是 `role="tablist"`，分组标题以 `role="presentation"` 的纯文本渲染，不是 tab、也不进入 tab 语义；Desktop 不允许在导航旁手工插入第二份分组条目。
