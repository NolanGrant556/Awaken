# AwakenMediaGenerationCard

对话流内图片、视频生成的独立媒体卡片组。`status`、请求上限、逐项 index/status 与媒体地址由 Desktop 投影；组件按请求上限建立最多 15 张互不嵌套的小卡片，支持部分成功时逐项 ready/error 和异步任务 unknown，媒体加载后按真实宽高比原位覆盖，不读取 runtime、文件路径或 provider。

生成中自动展开并保留旋转进度动态；`ready` 后默认折叠成一行结果摘要，摘要触发区按内容宽度收紧、文字左对齐且 Chevron 位于最右侧，点击后卡片网格从摘要原有左边缘向下恢复，不额外缩进。摘要触发器始终透明，标题、状态与 Chevron 默认使用统一浅色，悬停或键盘聚焦时只提亮前景色，不显示灰色圆角卡片底。`partial`、`unknown`、`error` 和 `cancelled` 默认保持展开，确保异常与不完整结果不会被自动隐藏。用户手动选择优先于后续自动状态。
