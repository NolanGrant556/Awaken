# AwakenDeliverableCard

最终交付文件的紧凑全宽卡片。标准卡片高度约 88px、文件图形宽约 48px；整张主区域点击打开文件，更多菜单保持 44×44px 操作区域。

组件只负责文件类型视觉、打开状态和 reveal 菜单；父级负责 durable artifact、权限校验及 Electron IPC。PDF 使用独立图标、红色 `PDF` 格式标识和 `PDF` 类型文案，点击语义与 Word、Markdown、Excel、PowerPoint 一致，继续通过 `onOpen` 交给系统默认应用。

Gallery 在 `#/components/awaken-deliverables` 的“「最终交付物」组件库”中集中展示本组件与 `AwakenDeliverableList`；两者只合并观测入口，不合并生产组件职责。
