# AwakenLogo

`AwakenLogo` 按 Logo 库 id 渲染一条目录记录。目录、光学切法和概念对照由 [Logo 库](../../logos/README.md) 拥有。

## 用法

- 传入 catalog `id`。产品标记支持 `opticalSize` `16 | 24` 与 `appearance` `filled | outline`。
- 字标、应用图标和供应商 Logo 只有已登记的切法；不支持的组合回退到该条目的第一套切法。
- Desktop 产品入口继续用 `AwakenProductLogo`、`AwakenTaskWelcome`、`AwakenProviderLogo`，不要在 renderer 里再写第二份 path 或远程图。
