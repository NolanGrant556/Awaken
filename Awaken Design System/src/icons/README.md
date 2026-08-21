# Awaken 图标库

本文是界面图标目录、光学切法和概念对照的唯一权威。Design System Guide 只保留视觉方向；Desktop 和 Gallery 只消费 `@awaken/design-system` 出口，不另建第二份 lucide 包装层。

方法来自 [The making of Cursor's icons](https://www.minoradventures.co/blog/the-making-of-cursors-icons) 的工作方式：两套光学尺寸、outline / filled、光学形、水平 / 垂直 / 45° 构造、闭合形状、圆角线帽、一个概念一个图标。不复制 Cursor 的 600 个图形，也不做图标字体或 Figma `ship it` 管线。

品牌标记、字标和供应商 Logo 仍由 [Logo 库](../logos/README.md) 拥有。

## 当前支持

- 当前 Desktop / Design System chrome 用到的界面图标，按唯一 catalog id 登记。
- 16 / 24 光学尺寸：16 覆盖 12–20px 显示，描边 1.25px；24 从 22px 起，描边 1.5px。
- outline 与 filled。filled 只填充图纸里标记为 `fillable` 的闭合形；纯线段图标（`plus`、chevron、箭头）外观与 outline 相同。16 会丢掉只属于 24 的细节。填充色写在图形的 inline `style` 上，避免父级 SVG `fill="none"` 把实心切法盖掉。
- lucide 兼容出口：旧的 `Plus`、`FileType2`、`Loader2` 等名称指向同一个概念，并保留现有测试依赖的 `lucide-*` class。
- Gallery 伴生页：网格、搜索、尺寸/样式、概念表。
- Desktop 侧栏、设置导航、Composer 发送、文件类型、连接器和预览工具栏都走这套图标。

## 当前不支持

- 完整 600+ 图标字体，或把旧引用编译进 font 而不改调用方。
- 复制 Cursor 图标 path。
- 自动从 Figma 发布或 `ship it` 字体编译管线。
- 用这套目录替换品牌 Logo；产品标记仍在 Logo 库。
- 为纯线段图标另做一套加粗或三角填充切法；当前 filled 与 outline 共用同一组 stroke。

## 用户入口

- Gallery：`http://127.0.0.1:5173/#/components/awaken-icon-library`
- Desktop：侧栏一级导航、设置分区、发送/停止、文件树与交付物类型、预览工具栏。

## 光学与构造

| 尺寸 | 适用显示 | 线框描边 | 作用 |
| --- | --- | --- | --- |
| 16 | 12–20px | 1.25px | 侧栏、按钮、文件行、紧凑 chrome |
| 24 | 22px 及以上 | 1.5px | 较大预览与强调动作 |

- 构造只用水平、垂直和 45°，优先闭合形状，圆角线帽。
- 高的保持高，圆的保持圆；光学形是 square / circle / horizontal / vertical。
- 16 与 24 不是同一 path 的缩放：作者在 24 网格上画，投射到 16 时丢掉 `detail: "24"` 的内部细节。
- 每个概念只对应一个 catalog id。`AlertCircle` 与 `CircleAlert`、`Loader` / `Loader2` / `LoaderCircle` 都是别名。
- 新增一条图标的固定顺序：在 `catalog.ts` 登记唯一 id/概念/光学形 → 在 `drawings.ts` 补 24 网格 ops → 如需兼容旧 lucide 名则写入 `AWAKEN_LUCIDE_GLYPH_IDS` → Gallery 搜索与概念表自动出现。

## 代码 owner

| 模块 | 拥有 |
| --- | --- |
| `src/icons/` | 目录、概念、光学图纸、lucide 别名 |
| `AwakenIcon` | 按 id 渲染 |
| `src/icons/glyphs.tsx` | lucide 兼容名称 |
| Desktop 调用方 | 只消费出口，不内联第二份 path |

## 验收

- `npx tsx --test src/icons/catalog.test.ts`（在 Design System 目录）
- `npm --workspace @awaken/design-system run typecheck`
- `npm --workspace @awaken/design-system run smoke:render`
- Gallery 打开图标库，切换 16/24 与 filled/outline，搜索“发送”只命中上向箭头
- Desktop 侧栏连接器/技能/定时任务、设置导航和文件类型图标走 `data-icon-id`
