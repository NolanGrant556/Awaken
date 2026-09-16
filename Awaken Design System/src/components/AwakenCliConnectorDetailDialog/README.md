# AwakenCliConnectorDetailDialog

官方 CLI 连接器详情弹窗，覆盖安装、安装进度、授权等待、重试、已连接账户与退出登录状态。弹窗直接复用 `AwakenProductLogo` 默认 mark 作为 Awaken 端的官方双柱标记；consumer 只需要提供外部连接器品牌图标。组件不启动 CLI、不打开浏览器，也不读取凭据。

`installProgress.percent` 可缺省；没有可信百分比时只展示阶段与取消入口，不生成百分比或确定进度条。
