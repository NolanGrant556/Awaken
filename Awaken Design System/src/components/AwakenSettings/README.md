# AwakenSettings

设置页的 UI-only 全窗口组合外壳，负责固定左侧一级导航、“返回应用”动作，以及右侧标准滚动或 full-bleed 内容视口。标准页面由 `.aw-settings-scroll` 承担纵向滚动；full-bleed 页面由直属内容 surface 承担唯一纵向滚动，当前模型页为 `.aw-model-settings-page`、记忆页为 `.aw-memory-light-flow-layout`。展开模型参数等动态增高内容时只滚动右侧页面，左侧一级导航保持固定，不在模型卡片内部创建第二个滚动区。模型页与记忆页都隐藏系统滚动条并投影窄滚动指示条；指示条的活动范围从各自 168px 顶部视觉下方开始，不覆盖顶部 UI。桌面密度使用较窄侧栏、紧凑导航行和受控内容宽度，避免在宽窗口中把设置项无意义拉伸；`--aw-settings-focused-content-width` 为宠物、计算机操作和 ACP 等卡片型页面提供统一的 820px 居中内容列，一级标题仍由标准页面占据当前阅读宽度。`screen` 用于 Desktop 覆盖当前内容窗口，`embedded` 用于 Gallery；组件不创建 backdrop、外层圆角卡片、右上角关闭按钮或底部关闭栏。模型配置、搜索连接、记忆数据、主题持久化与宠物导入等业务逻辑仍由 Desktop consumer 负责。

设置框架默认禁止选择标题、导航、标签、说明和其他界面文案，避免双击空白区域时出现网页式文字高亮；`input`、`textarea`、可编辑区域及显式标记为 `.aw-text-selectable` 的正文仍可选择复制。该规则由共享外壳统一提供，Gallery 与 Desktop 行为一致。
