# AwakenProfileBalancePanel

左侧栏个人入口展开后的账户额度面板。展示模型余额或配额、加载/错误/空状态，在底部提供与“设置 > 外观”共用的跟随系统、浅色和黑色主题选择，并保留进入设置的动作。

组件只接收 `balances`、`loading`、`error`、当前主题偏好和回调，不读取 Desktop runtime；真实 Awaken 由 `ProfileBalancePopover` 负责请求、定位、主题保存与失败回滚，再复用相同 UI 能力渲染内容。
