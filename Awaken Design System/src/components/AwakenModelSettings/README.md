# AwakenModelSettings

模型设置页的 UI-only 页面组件。左侧以窄轨承载 Coding Plan、API、Agent Plan 接入类型导航，右侧使用紧凑页头承载连接配置与 LLM 设置二级入口、当前类型说明、页面消息和内容容器；模型工作区限制最大阅读宽度，不随超宽窗口无限拉伸。连接配置使用固定三列、统一高度的紧凑卡片，只展示官方 Logo、名称和“配置”动作；选择后由 `AwakenSettingsProviderModal` 在屏幕中央承载配置表单与连接状态。供应商读取、凭据、连接测试、保存、重复模型 Toast、删除和运行时能力同步仍由 Desktop consumer 负责。
