# PPT 模板与已有演示稿编辑

## 使用 python-pptx 的条件

适合逐 run 替换文字、换图片、读取几何和普通占位符。以下情况谨慎：

- `text_frame.text = ...` 会把段落折叠成单一无样式 run；应遍历 paragraph/run 修改 `run.text`；
- python-pptx 没有可靠的“复制已有 slide”公开 API；
- SVG/EMF、SmartArt、复杂图表、嵌入对象和部分扩展 XML 可能无法读取或在保存时变化；
- 使用模板时先复制原件到新输出路径，不覆盖用户文件。

## 高保真 OOXML 路线

1. 使用 `ooxml_package.py unpack` 安全解包；
2. 先完成增页、删页、排序等结构工作；
3. 再修改 `ppt/slides/slideN.xml` 和对应 `.rels`；
4. 使用 `ooxml_package.py pack` 重新打包；
5. `validate_pptx.py --original`；
6. 重新抽取内容和逐页渲染。

不要只复制 `slideN.xml`。一张页还涉及：

- `ppt/presentation.xml` 的 `<p:sldId>`；
- `ppt/_rels/presentation.xml.rels`；
- `[Content_Types].xml`；
- 页面自己的 `.rels`；
- 可能共享的图片、图表、主题、SmartArt 和嵌入对象。

复制页如果继续指向源页的 chart/workbook，修改一个页面的图表可能同时改变另一个页面。

## 替换模板内容

- 一个列表项对应一个 `<a:p>`，不要把所有项目拼在同一段；
- 需要保留格式时修改 `<a:t>` 或目标 run，保留 `<a:rPr>`；
- 首尾空格使用 `xml:space="preserve"`；
- 项目符号继承模板，必要时使用 `<a:buChar>`、`<a:buAutoNum>` 或 `<a:buNone>`，不在文本中写 `•`；
- 模板有 4 个人而实际只有 3 个时，删除第 4 组完整视觉对象，不只清空名字；
- 标题从一行变两行后，重新检查装饰、图片和正文位置；
- 隐藏页也要在内容检查中识别，不能无意发布残留内容。

## 修改 XML 的安全要求

- 不使用会任意改写命名空间前缀的 XML 往返方案；
- 不格式化整个包；只改目标部件；
- 不使用正则跨多个标签盲目替换；
- 外部文件先校验 ZIP 路径、relationship 和 content type；
- 输出到新文件，并保留原件作为结构基线。
