# pptxgenjs 速查（Awaken）

按需用 `skill_resource_read` 读取本文。完整 API 以 npm 包文档为准。

## 安装与写出

```bash
# 在任务工作目录本地安装；不要 -g（全局包 require 不到）
npm install pptxgenjs
```

```javascript
const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();
pptx.defineLayout({ name: "LAYOUT_16x9", width: 10, height: 5.625 });
pptx.layout = "LAYOUT_16x9";

const s = pptx.addSlide();
s.addText("标题", {
  x: 0.5, y: 0.4, w: 9, h: 0.8,
  fontSize: 36, bold: true, color: "1E2761", fontFace: "Arial",
});
s.addShape(pptx.ShapeType.roundRect, {
  x: 0.5, y: 1.5, w: 4.2, h: 2.2,
  fill: { color: "F2F2F2" }, rectRadius: 0.1,
});
s.addText("要点说明", { x: 0.7, y: 1.7, w: 3.8, h: 1.8, fontSize: 14, color: "36454F" });

pptx.writeFile({ fileName: "output.pptx" });
```

## 常用尺寸（英寸）

- 16:9 → width 10 / height 5.625
- 内容区建议边距 ≥ 0.5

## 中文

指定本机存在的中文字体，例如 macOS `PingFang SC`，Windows `Microsoft YaHei`。缺字体时可能方框或回退。

## 图表

```javascript
s.addChart(pptx.ShapeType.bar, {
  x: 5.2, y: 1.4, w: 4.3, h: 3.2,
  series: [{ name: "销量", labels: ["Q1", "Q2", "Q3"], values: [12, 18, 15] }],
  showTitle: false,
});
```

（具体枚举名以当前 pptxgenjs 版本为准，不确定时用 `Bash` 查 `node_modules` 示例。）

## 备注

```javascript
s.addNotes("演讲者备注");
```
