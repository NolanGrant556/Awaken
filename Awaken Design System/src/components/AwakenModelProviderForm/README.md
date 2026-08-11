# AwakenModelProviderForm

Awaken 当前模型供应商连接表单。包含协议、内置供应商模型选择、自定义 API 模型 ID 纯文本输入、按需显示的鉴权与 Base URL、API 密钥、模型目录状态、连接测试反馈与提交动作；自定义 API 不展示或请求模型目录。`modelDisplayNames` 只把 consumer 提供的精确模型 ID 映射为稳定展示名，选中回调仍返回原始 ID，手填的 Model/Endpoint ID 原样保留。不承载图片、视频、PDF、工具、缓存、上下文或推理强度等能力开关。数据加载、测试和保存由 consumer 控制。`testFeedback` 只负责在表单内展示 consumer 已判定的成功或失败结果，不拥有连接状态、能力判定或后端策略。
