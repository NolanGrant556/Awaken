# AwakenSettings

设置页的 UI-only 全窗口组合外壳，负责固定左侧一级导航、“返回应用”动作，以及右侧标准滚动或 full-bleed 内容视口。桌面密度使用较窄侧栏、紧凑导航行和受控内容宽度，避免在宽窗口中把设置项无意义拉伸。`screen` 用于 Desktop 覆盖当前内容窗口，`embedded` 用于 Gallery；组件不创建 backdrop、外层圆角卡片、右上角关闭按钮或底部关闭栏。模型配置、搜索连接、记忆数据、主题持久化与宠物导入等业务逻辑仍由 Desktop consumer 负责。

设置框架默认禁止选择标题、导航、标签、说明和其他界面文案，避免双击空白区域时出现网页式文字高亮；`input`、`textarea`、可编辑区域及显式标记为 `.aw-text-selectable` 的正文仍可选择复制。该规则由共享外壳统一提供，Gallery 与 Desktop 行为一致。
