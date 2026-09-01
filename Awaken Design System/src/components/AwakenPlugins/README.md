# AwakenPlugins

插件 Catalog 页的 UI-only 组合组件。统一拥有标题、刷新与安装操作、搜索、双列插件卡片，以及加载、无结果、未安装和错误状态。「内置插件 / 已安装插件」复用连接器页的轻量分段按钮视觉：分段组没有外围底板，只有当前项使用浅色填充。

## 受控状态

- `plugins` / `searchValue` / `loading` / `errorMessage`：由 consumer 提供。
- `onSearchChange` / `onRefresh` / `onInstall`：页面级交互回调。
- `onPluginOpen` / `onPluginEnabledChange`：卡片详情和启停回调。

组件只做名称、版本、描述与诊断的本地搜索投影，不读取 runtime、文件系统或插件状态文件。Desktop 继续拥有安装、刷新、详情、启停、升级与卸载编排。
