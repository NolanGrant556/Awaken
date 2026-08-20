# AwakenProductLogo

`AwakenProductLogo` 是 Desktop 使用官方产品标志的 React 入口。资产、光学切法和概念对照由 [Logo 库](../../logos/README.md) 拥有；本组件只选择 `product-mark` 或 `product-icon`。

## 三件套

1. **Mark**（默认 `variant="mark"`）— Logo 库 `product-mark`。16 / 24 光学切法，filled 或 outline。颜色跟随 `--aw-text-primary`。
2. **App icon**（`variant="icon"`）— Logo 库 `product-icon`，已创作的 256 PNG。只用于 Dock、开始菜单和安装包。
3. **Wordmark** — Logo 库 `product-wordmark`，由 `AwakenTaskWelcome` 消费，不经过本组件。

三件套不得互相裁切或互相替代。安全区为标记高度的四分之一。

## 用法

- 默认 `variant` 是 `mark`。需要完整 App Icon 构图时才传 `icon`。
- `size`：`16` / `24` 使用对应光学切法；`sm` / `md` / `lg` 显示为 32 / 40 / 64，并选用 24 光学切法。
- `appearance`：`filled`（默认）或 `outline`。
- 默认提供 `Awaken` 替代文本；已有等价文字上下文时使用 `decorative`。
- 不要把标记改成 `--aw-accent` 紫色，不要另加描边、投影、拉伸或旋转。
- consumer 不得再复制、覆盖或注入另一份 Awaken 方形图标。
- 完整导出矩阵（favicon、DMG、ICO、文档封面）当前不支持。
