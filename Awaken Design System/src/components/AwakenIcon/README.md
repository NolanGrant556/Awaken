# AwakenIcon

`AwakenIcon` 按图标库 id 渲染一条目录记录。目录、光学切法、概念对照和 Desktop 消费方式由 [图标库](../../icons/README.md) 拥有。

## 用法

- 传入 catalog `id`。支持 `opticalSize` `16 | 24` 与 `filled`。`filled` 只改变 `fillable` 闭合形；线段图标看起来与 outline 相同。
- 显示尺寸 12–20px 使用 16 切法，22px 及以上使用 24 切法。
- 现有 lucide 名称通过 `src/icons/glyphs.tsx` 的兼容出口指向同一个 id，不要再从 `lucide-react` 画界面图标。
