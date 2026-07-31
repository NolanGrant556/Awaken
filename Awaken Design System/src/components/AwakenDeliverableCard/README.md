# AwakenDeliverableCard

最终交付物的统一入口。非媒体文档使用紧凑全宽卡片，标准高度约 88px、文件图形宽约 48px；图片和视频复用 `AwakenMediaItemCard`，以 128–144px 宽独立小卡展示，并在媒体元数据到达后使用真实宽高比。

组件只负责文件类型视觉、预览入口、打开状态和 reveal 菜单；父级负责 durable artifact、权限校验及 Electron IPC。媒体单击交给 `onPreview`，解码失败经 `onMediaError` 交给宿主续签 opaque 源；`previewUnavailable` 时显示“预览不可用”而非永久 ready 骨架。系统打开与定位仍经 `onOpen` / `onReveal`，组件不接触本地真实路径。PDF 使用独立图标、红色 `PDF` 格式标识和 `PDF` 类型文案，点击语义与 Word、Markdown、Excel、PowerPoint 一致，继续通过 `onOpen` 交给系统默认应用。

Gallery 在 `#/components/awaken-deliverables` 的“「最终交付物」组件库”中集中展示本组件与 `AwakenDeliverableList`；两者只合并观测入口，不合并生产组件职责。
