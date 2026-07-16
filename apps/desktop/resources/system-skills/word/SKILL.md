---
name: word
version: 1.0.0
description: 创建、阅读、编辑或处理 Word 文档（.docx）。适用于报告、备忘录、函件、模板、目录/页眉页脚/页码、表格与图片排版，以及从现有 .docx 提取或重组内容。用户提到 Word、docx、报告文档时使用。不要用于 PDF、表格或演示文稿主交付物。
when_to_use: 需要生成或修改 .docx 专业文档时；需要从 Word 提取正文/修订痕迹时；需要把草稿整理成可交付 Word 文件时。
user-invocable: true
---

# Awaken Word Skill

用 **Bash + 脚本/库** 生成与编辑 `.docx`，再经 **`artifact_publish`** 发布为 Workbench 最终交付物卡片。

## Awaken 工具约定

| 步骤 | 工具 |
| --- | --- |
| 跑脚本、pandoc/markitdown、LibreOffice 转换 | `Bash` |
| 安装 LibreOffice / Poppler 等系统级依赖 | `dependency_install`（优先），或 `Bash` |
| 读文本/脚本、渲图后的预览图 | `Read` |
| 改脚本或中间 Markdown | `Edit` / `Write` |
| 定位文件 | `Glob` / `Grep` |
| 最终交付 | **`artifact_publish`**（必做） |

包内文本资源用 `skill_resource_read`；二进制资产用 `Read` / `Bash`。

**重要**：最终 `.docx` 必须生成在任务工作区（output roots）内，`artifact_publish` 不接受工作区外（如 /tmp）的路径。

## 依赖

首次使用前用 `Bash` 检查并按需安装：

```bash
# Node 库：在任务工作目录本地安装（不要 -g，全局包 require 不到）
npm install docx

# Python 库：新 macOS/Homebrew Python 受 PEP 668 保护，直接 pip3 install 会报
# externally-managed-environment。优先顺序：
#   1) uv pip install "markitdown[docx]"        （本机有 uv 时）
#   2) python3 -m venv .venv && .venv/bin/pip install "markitdown[docx]"
#   3) pip3 install --user --break-system-packages "markitdown[docx]"
python3 -c "import markitdown" 2>/dev/null || echo "need markitdown"

# 转换与修订处理：pandoc；预览（可选）：LibreOffice + Poppler。
# 缺失时优先用 dependency_install 工具安装
which pandoc
which soffice || which libreoffice
which pdftoppm
```

Windows 上把 `python3` / `pip3` 换成 `python` / `pip`（或 `py -m pip`）。

## 快速对照

| 任务 | 做法 |
| --- | --- |
| 阅读/分析 | `python3 -m markitdown 文件.docx` 或 `pandoc 文件.docx -t markdown` |
| 新建 | `docx`（Node）或 `python-docx` |
| 编辑已有 | 优先 `python-docx` 结构化改；复杂修订再考虑解包 XML |
| 旧 `.doc` → `.docx` | `soffice --headless --convert-to docx 文件.doc` |
| 接受全部修订 | `pandoc --track-changes=accept`（见下） |
| 预览页图 | LibreOffice 转 PDF → `pdftoppm` → `Read` 看图 |
| 交付 | `artifact_publish` |

## 新建文档（推荐 docx-js）

1. `Bash`：任务目录内 `npm install docx`。
2. `Write` 写出生成脚本（如 `build_doc.js`）。
3. `Bash`：`node build_doc.js` 生成 `.docx`。
4. `Bash`：`python3 -m markitdown output.docx` 抽查正文。
5. 需要版式确认时转图并用 `Read` 检查。
6. **`artifact_publish`**，路径指向工作区内的最终 `.docx`。

要点：

- **页面尺寸写死**：国内常用 A4，美式信纸需显式设置。
- **列表不要手写 `•`**：用 numbering / LevelFormat。
- 标题用内置 Heading 样式（便于目录）。
- 字体优先系统常见中西文字体（如 `PingFang SC` / `Microsoft YaHei` / `Arial`），避免缺字。

最小示例：

```javascript
const { Document, Packer, Paragraph, TextRun, HeadingLevel } = require("docx");
const fs = require("fs");

const doc = new Document({
  styles: {
    default: { document: { run: { font: "Arial", size: 22 } } },
  },
  sections: [{
    properties: {
      page: {
        size: { width: 11906, height: 16838 }, // A4
        margin: { top: 1440, right: 1440, bottom: 1440, left: 1440 },
      },
    },
    children: [
      new Paragraph({
        heading: HeadingLevel.HEADING_1,
        children: [new TextRun("报告标题")],
      }),
      new Paragraph({ children: [new TextRun("正文段落。")] }),
    ],
  }],
});

Packer.toBuffer(doc).then((buf) => fs.writeFileSync("output.docx", buf));
```

## 备选：python-docx 新建/编辑

```python
from docx import Document
from docx.shared import Pt, Cm

doc = Document()
doc.add_heading("报告标题", level=1)
p = doc.add_paragraph("正文。")
doc.save("output.docx")
```

编辑已有文件用 `Document("existing.docx")`，改段落后再 `save`。

## 阅读与修订痕迹

```bash
pandoc --track-changes=all document.docx -t markdown -o out.md
# 或
python3 -m markitdown document.docx
```

接受全部修订：

```bash
pandoc --track-changes=accept input.docx -o output.docx
```

**保真度边界**：pandoc 的 docx→docx 会经内部 AST 重建文档，页眉页脚、文本框、部分样式等版式信息会丢失。该路线适合"接受修订后继续加工内容"；若用户要求"仅接受修订、版式一字不动"，应改走解包 OOXML 处理 `w:ins` / `w:del` 节点的高级路径，或明确建议用户在 Word/LibreOffice 中人工接受。

## 视觉抽查

```bash
soffice --headless --convert-to pdf --outdir . output.docx
pdftoppm -jpeg -r 120 output.pdf page
```

然后对 `page-1.jpg` 等调用 Awaken **`Read`**（需视觉能力模型）。检查：溢出、空白过多、中文乱码、页眉页脚碰撞。

## 交付检查清单

- [ ] 正文无占位符（lorem / TODO / xxxx）
- [ ] 标题层级合理，中英文混排可读
- [ ] 需要时已做预览图抽查
- [ ] 最终文件位于任务工作区内
- [ ] 已调用 **`artifact_publish`**，Workbench 出现可点击打开的交付物卡片

## 明确不做

- 不伪造「已发布交付物」：只写文件不够，必须 `artifact_publish`。
- 不假装完成 pandoc/LibreOffice 不可用时的转换：缺依赖先安装或如实说明。
