# AwakenPluginDetailDialog

插件详情弹窗。展示 manifest 摘要、来源、digest、诊断、skills 预览入口、MCP server 短名/readiness/单 server 启停，以及升级与卸载。卸载风险说明固定写在弹窗内；确认与副作用由 Desktop 拥有。

## Props

- `name` / `version` / `description` / `sourceLabel` / `digest`：只读展示。
- `skills` / `servers` / `diagnostics`：UI-only 列表。
- `onPreviewSkill`、`onServerEnabledChange`、`onUninstall`：回调；缺省时对应操作不可用或仅展示。

组件不读取 runtime、不执行安装或卸载。
