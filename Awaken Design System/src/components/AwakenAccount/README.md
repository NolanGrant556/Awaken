# AwakenAccount

“「账户」组件库”的组合组件，通过 `AwakenPopover` 将 `AwakenProfileButton` 与 `AwakenProfileMenuPanel` 连接为可展开的个人账户入口，再由“查询用量”打开 `AwakenProfileUsageDialog`。

额度数据、查询时机、主题偏好与打开设置的业务动作由 consumer 传入。Awaken Desktop 负责 runtime 请求、主题保存和失败回滚；Design System 负责触发、双栏展示和交互状态。个人弹层与 Settings > 外观共用同一偏好，不持有局部主题。

Gallery 只在这一真实账户组合中观测 `AwakenPopover` 的打开、关闭、定位和焦点行为，不再维护虚构的模型、Plan/Act 或权限浮层。`AwakenPopover` 继续保留生产源码与公开导出，原 `awaken-popover` URL 跳转到“「账户」组件库”。
