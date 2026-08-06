# AwakenMediaInspector

`AwakenMediaInspector` 是用户从媒体卡片菜单主动固定到 Desktop 右侧栏的图片/视频查看器。组件只消费父级传入的临时媒体地址、固有尺寸和回调；不解析 Artifact 身份、不访问文件系统，也不负责续签媒体地址。

图片与视频都按真实宽高比使用 `contain` 展示，视频保留原生播放控件。Desktop 负责用同一 Artifact 身份续签失败的 `awaken-artifact://` 地址，并在切换任务时清理固定状态。
