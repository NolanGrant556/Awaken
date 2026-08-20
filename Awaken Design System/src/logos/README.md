# Awaken Logo 库

本文是 Logo 目录、光学切法和概念对照的唯一权威。Design System Guide 只保留视觉方向；Desktop 只消费 `@awaken/design-system` 出口，不另建第二份 Awaken 或供应商 Logo。

方法来自 [The making of Cursor's icons](https://www.minoradventures.co/blog/the-making-of-cursors-icons) 的工作方式，不复制 Cursor 图标本身。通用界面图形仍用 `lucide-react`。

## 当前支持

- 产品三件套：`product-mark`、`product-wordmark`、`product-icon`。
- 产品标记的 16 / 24 光学尺寸，以及 filled / outline 两套切法。
- 现有模型供应商 Logo 的统一登记与检索。
- Gallery 伴生页：网格、搜索、尺寸/样式、概念表。
- Desktop 通过 `AwakenProductLogo`、`AwakenTaskWelcome`、`AwakenProviderLogo` 消费，不再内联第二份产品 path。

## 当前不支持

- 完整 Brand 导出矩阵（favicon、DMG、ICO、文档封面）。
- 用自研图标字体替换 `lucide-react`。
- 为连接器第三方品牌重绘光学切法；连接器仍由 consumer 传入官方图。
- 自动从 Figma 发布或 `ship it` 字体编译管线。

## 用户入口

- Gallery：`http://127.0.0.1:5173/#/components/awaken-logo-library`
- Desktop：欢迎区字标、连接器/设置中的产品标记、模型与用量中的供应商 Logo。

## 光学与构造

| 尺寸 | 适用显示 | 线框描边 | 作用 |
| --- | --- | --- | --- |
| 16 | 12–20px | 1.25px | 侧栏、按钮、紧凑 chrome |
| 24 | 22px 及以上 | 1.5px | 欢迎区以外的常规标记 |

- 双柱 A 按竖直光学形绘制，高的保持高，不压成方块。
- 构造只用水平、垂直和 45°，闭合形状，圆角线帽。
- 16 与 24 是两套手绘切法，不是同一 path 的缩放。
- 每个概念只对应一个 catalog id。
- 新增一条 Logo 的固定顺序：在 `catalog.ts` 登记唯一 id/概念/光学形 → 补 16/24 与 filled/outline 中产品实际需要的切法 → 放入 `src/assets/` → Desktop 若是供应商则补 `model-provider-brand.ts` → Gallery 搜索与概念表自动出现。当前没有 Figma/`ship it` 字体编译管线。

## 代码 owner

| 模块 | 拥有 |
| --- | --- |
| `src/logos/` | 目录、概念、光学切法、构造常量 |
| `AwakenLogo` | 按 id 渲染 |
| `AwakenProductLogo` / `AwakenTaskWelcome` / `AwakenProviderLogo` | 产品入口，内部只调用目录 |
| Desktop `model-provider-brand.ts` | providerId → catalog id |
| `apps/desktop/build/icon.*` | 安装包/窗口图标；完整导出矩阵仍未接通 |

## 验收

- `npx tsx --test src/logos/catalog.test.ts`（在 Design System 目录）
- `npm --workspace @awaken/design-system run typecheck`
- `npm --workspace @awaken/design-system run smoke:render`
- Gallery 打开 Logo 库，切换 16/24 与 filled/outline，搜索“双柱”只命中产品标记
- Desktop 欢迎区仍是字标；设置/连接器产品标记仍走 `AwakenProductLogo`
