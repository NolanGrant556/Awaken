# AwakenProfileBalancePanel

左侧栏个人入口展开后的账户额度面板。展示模型余额或配额、加载/错误/空状态，并提供进入设置的动作。

组件只接收 `balances`、`loading`、`error` 和回调，不读取 Desktop runtime；真实 Awaken 由 `ProfileBalancePopover` 负责请求和定位，再复用本组件渲染内容。
