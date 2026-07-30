# AwakenProfileBalancePanel

独立用量窗口中的双栏内容面板。左侧和右侧详情头只展示 consumer 提供的厂商名称与 Logo，不展示模型 ID；右侧继续展示当前选中项的余额、配额、暂不支持、凭据恢复、加载、错误或空状态。

组件只接收 `balances`、`loading`、`error` 和选择回调，不读取 Desktop runtime。consumer 可通过每条 balance 的 `providerBrand` 复用 `AwakenProviderLogo`；没有品牌映射时显示名称首字母兜底。`AwakenProfileUsageDialog` 负责窗口外壳；真实 Awaken 只在用户点击“查询用量”后调用现有余额接口。
