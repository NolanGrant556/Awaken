# AwakenMediaItemCard

图片或视频的一项独立紧凑卡片。组件根据媒体加载后的真实宽高比调整画幅；ready 媒体直接贴合透明画布，不增加黑色衬底或内框。组件拥有加载、成功、状态待确认、失败、预览入口和受控本地操作菜单；unknown 是不再旋转的终态展示，不等同于生成失败。挂载或 `src` 变化后会检查 `img.complete` / 视频 metadata，避免 remount 错过 `load` 事件后永久停在骨架；解码失败时通过可选 `onMediaError` 通知宿主续签源。组件不读取 runtime、Electron 或真实文件路径。
