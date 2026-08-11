# AwakenProfileBalancePanel

独立用量窗口中的双栏内容面板。左侧和右侧详情头只展示 consumer 提供的厂商名称与 Logo，不展示模型 ID；右侧只展示当前选中项由供应商官方 API 返回的余额、配额或组织用量，以及需要额外管理授权、仅能前往平台查看、凭据恢复、加载、错误或空状态。组件不接收或展示 Awaken 本地累计 token、请求次数与本地成本。

组件只接收 `balances`、`loading`、`error` 和选择回调，不读取 Desktop runtime。consumer 可通过每条 balance 的 `providerBrand` 复用 `AwakenProviderLogo`；没有品牌映射时显示名称首字母兜底。`requires_setup` 条目必须同时给出准确的授权说明、官方 `setupURL` 和操作标签；普通推理 Key 不能被描述成管理凭据。额度单位为百分比时通过 `quota.unit = "percent"` 明确呈现 `%`，不把百分比伪装成请求数；无限窗口通过 `quota.unlimited = true` 显示 `∞ / 无限制`，不能伪装成有限的 100%。`AwakenProfileUsageDialog` 负责窗口外壳；真实 Awaken 只在用户点击“查询用量”后调用现有余额接口。
