# PptxGenJS 创建细则

## 初始化

```javascript
const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();
pptx.layout = "LAYOUT_WIDE";
pptx.author = "Awaken";
pptx.subject = "用户任务对应的主题";
pptx.title = "演示文稿标题";
pptx.lang = "zh-CN";
```

先设置 layout，再添加 slide。不要凭印象把画布宽度当成 13.33 英寸；使用所选 layout 的真实宽高检查所有坐标。

## 高发错误

- 颜色写作 `"FF0000"`，不带 `#`，不把 alpha 拼进 8 位颜色；填充透明度使用 `transparency`，阴影透明度使用对应字段。
- PptxGenJS 可能原地修改 options；每次 `addShape/addText/addImage` 创建新的 options/shadow 对象。
- 阴影 offset 使用非负值；改变方向使用 angle。
- 字间距字段使用当前版本真实 API，不自行发明 `letterSpacing`。
- 列表使用 runs/paragraph options 的 bullet，列表项之间使用 paragraph spacing，不写双重项目符号。
- 文本与图形对齐时显式设置 text box `margin: 0` 或统一 padding。
- 渐变不受支持时使用由 Awaken 自己生成的背景图片，不伪造半支持参数。
- speaker notes 使用 `slide.addNotes("...")`，每页一次。
- 每个输出文件使用独立的 PptxGenJS 实例。

## 图表

优先保留原生图表：

```javascript
slide.addChart(
  pptx.ChartType.bar,
  [{ name: "销量", labels: ["Q1", "Q2", "Q3"], values: [12, 18, 15] }],
  {
    x: 5.2, y: 1.4, w: 4.3, h: 3.2,
    showTitle: true,
    title: "季度销量",
    showValue: true,
    showLegend: false,
    chartColors: ["1E2761"],
    catAxisLabelColor: "4B5563",
    valAxisLabelColor: "4B5563",
    valGridLine: { color: "D1D5DB", size: 1 },
  },
);
```

- 单系列图通常不需要 legend；标题、数据标签和坐标轴样式必须主动设置。
- 堆积条形/柱形图的数据标签位置使用图表支持的内部位置，避免 `outEnd`。
- 组合图启用次坐标轴时必须同时声明完整的主/次 value axis 和 category axis；生成后运行 `validate_pptx.py`。
- PowerPoint 有原生形式的图表不要渲成图片；Sankey、网络图等没有原生形式时才使用清晰图片，并保留数据来源。

## 图标和图片

- 图片按原始比例裁切或 contain，不直接拉伸；
- SVG/图标链路在实际运行环境不稳定时转成高分辨率 PNG；
- base64 data URI 包含正确 MIME 前缀；
- 图片、图标和背景都必须检查版权和来源；
- 不要用装饰图标代替内容层级。

## 写出

```javascript
await pptx.writeFile({ fileName: "output.pptx" });
```

写出后立即进行内容、结构、视觉三层 QA。结构错误应在生成脚本中修复并重建，不要手工修补最终包后丢失可复现性。
