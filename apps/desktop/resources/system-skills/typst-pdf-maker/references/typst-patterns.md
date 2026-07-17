# Typst 常用写法

只在当前项目或已选模板没有展示所需写法时读取相应小节。

## 字体与语言

```typst
#set text(
  font: ("Libertinus Serif", "实际存在的 CJK 字体"),
  lang: "zh",
  region: "cn",
)
```

字体名称必须来自 `typst fonts`。不要根据操作系统名称猜测。

## 页面与背景

```typst
#set page(paper: "a4", margin: (x: 2.1cm, y: 2.4cm))
#set page(fill: rgb("#f7f8fa"))
```

纯色使用 `fill`；水印等内容层使用 `background`。

## 表格

```typst
#table(
  columns: (1.2fr, 2fr, 1fr),
  inset: 7pt,
  table.header([项目], [说明], [状态]),
  [A], [正文], [完成],
)
```

长表格使用 `table.header` 让表头跨页重复。宽表格先调整列比例，再考虑横向页面。

## 图片与图注

```typst
#figure(
  image("image.png", width: 72%),
  caption: [示例图],
) <example-figure>

如 @example-figure 所示……
```

正常正文流不使用 `place` 代替 `figure`。只有用户确认需要环绕时才采用规划器选择的唯一 text-wrap 包。

## 多栏

```typst
#columns(2, gutter: 1.2em)[
  第一栏内容……
  #colbreak()
  第二栏内容……
]
```

严格双栏出版模板由模板拥有，不在外部重复设置。

## 引用块

```typst
#quote(block: true, attribution: [来源])[引用正文]
```

## 数学与代码

````typst
$ integral_0^1 x^2 dif x = 1/3 $

```python
print("hello")
```
````

代码行号、高亮或算法伪代码只使用构建计划选出的一个包。

## 链接

```typst
#show link: set text(fill: rgb("#2457a6"))
#link("https://example.com")[示例链接]
```

`link` 本身不接受 `fill` 参数。

## Universe 导入

```typst
#import "@preview/package-name:1.2.3": symbol
```

只使用 `.typst-build-plan.json` 中的精确 `universe_ref`，并在调用前读取该版本的公开 API；不要猜函数名。

## 水印

```typst
#set page(background: align(center + horizon)[
  #rotate(-25deg, text(size: 36pt, fill: rgb("#00000018"))[内部资料])
])
```

水印必须保持低对比度，不得妨碍正文阅读。
