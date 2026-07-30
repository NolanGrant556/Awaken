# AwakenMediaGenerationCard

对话流内图片、视频生成的独立媒体卡片组。`status`、请求上限、逐项 index/status 与媒体地址由 Desktop 投影；组件按请求上限建立最多 15 张互不嵌套的小卡片，支持部分成功时逐项 ready/error 和异步任务 unknown，媒体加载后按真实宽高比原位覆盖，不读取 runtime、文件路径或 provider。
