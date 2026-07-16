---
name: ppt
version: 1.0.0
description: 创建、阅读、编辑 PowerPoint（.pptx)。适用于演示稿、路演、汇报页、模板改稿、备注与版式。用户提到幻灯片、PPT、pptx、deck 时使用。不要用于 Word/Excel 作为主交付物的任务。
when_to_use: 需要生成或修改 .pptx 时；需要从现有演示稿提取内容或按模板改页时；需要做可交付的幻灯片并发布到 Workbench 时。
user-invocable: true
---

# Awaken PPT Skill

用 **Bash + pptxgenjs / python-pptx** 做幻灯片；用 LibreOffice + Poppler 渲图，再用 **`Read`（视觉模型）** 自检。交付必须 **`artifact_publish`**。

## Awaken 工具约定

| 步骤 | 工具 |
| --- | --- |
| npm/pip、生成、转 PDF、渲图 | `Bash` |
| 安装 LibreOffice / Poppler 等系统级依赖 | `dependency_install`（优先），或 `Bash` |
| 读脚本；读 `slide-*.jpg` 做视觉 QA | `Read` |
| 改生成脚本 | `Edit` / `Write` |
| 找素材 | `Glob` / `Grep` |
| 最终交付 | **`artifact_publish`** |

包内详细 API：`references/pptxgenjs.md`、`references/editing.md`（按需 `skill_resource_read`）。

**重要**：最终 `.pptx` 必须生成在任务工作区（output roots）内，`artifact_publish` 不接受工作区外的路径。

## 依赖

```bash
# Node 库：任务工作目录本地安装（不要 -g，全局包 require 不到）
npm install pptxgenjs

# Python 库：新 macOS/Homebrew Python 受 PEP 668 保护。优先顺序：
#   1) uv pip install "markitdown[pptx]" Pillow
#   2) python3 -m venv .venv && .venv/bin/pip install "markitdown[pptx]" Pillow
#   3) pip3 install --user --break-system-packages "markitdown[pptx]" Pillow
python3 -c "import markitdown" 2>/dev/null || echo "need markitdown"

# 渲图依赖：缺失优先用 dependency_install 安装
which soffice || which libreoffice
which pdftoppm
```

Windows 上把 `python3` / `pip3` 换成 `python` / `pip`（或 `py -m pip`）。

## 快速对照

| 任务 | 做法 |
| --- | --- |
| 读内容 | `python3 -m markitdown deck.pptx` |
| 从零创建 | pptxgenjs（见 references） |
| 改已有/套模板 | python-pptx 或解包编辑（见 references/editing.md） |
| 视觉 QA | 转图 → `Read` 看图 |
| 交付 | `artifact_publish` |

## 创建流程

1. 先定主题色与版式母题（整稿统一，不要页页换风格）。
2. `Bash` 本地 `npm install pptxgenjs` → `Write` 生成脚本 → `Bash` 跑出 `.pptx`。
3. `python3 -m markitdown output.pptx` 查文案与占位符。
4. 渲图并视觉自检（见下）。
5. 修问题 → 再渲受影响页 → 再检。
6. **`artifact_publish`**。

## 设计底线（避免「AI 味」幻灯片）

- 每页至少一个视觉锚点（图/图标/色块/大数字），禁止纯标题+子弹。
- 一种主色占 60–70%，辅色与强调色克制使用；**不要默认万能蓝**。
- 标题页/收尾可用深色，内容页可浅色，或全程深色高端风——选一种坚持到底。
- 标题约 36–44pt，正文 14–16pt；页边距 ≥ 0.5"。
- **禁止**标题下装饰细线（典型 AI 幻灯片特征）。
- 正文左对齐；勿居中大段文字。
- 中文优先系统字体（PingFang SC / Microsoft YaHei 等）。

色板灵感（十六进制，可改）：

| 主题 | 主色 | 辅色 | 强调 |
| --- | --- | --- | --- |
| 午夜商务 | `1E2761` | `CADCFC` | `FFFFFF` |
| 森林 | `2C5F2D` | `97BC62` | `F5F5F5` |
| 墨灰极简 | `36454F` | `F2F2F2` | `212121` |
| 青绿信任 | `028090` | `00A896` | `02C39A` |

## 视觉 QA

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/render_slides.py" output.pptx ./preview
# 或手动：
# soffice --headless --convert-to pdf --outdir . output.pptx
# pdftoppm -jpeg -r 150 output.pdf preview/slide
```

对预览图调用 **`Read`**，按「猎虫」心态检查：重叠、裁切、对比度不足、间距不均、残留占位符、边距不足。

占位符扫描：

```bash
python3 -m markitdown output.pptx | grep -iE "xxxx|lorem|ipsum|占位|待填|this.*(page|slide).*layout" || true
```

**至少完成一轮「发现问题 → 修复 → 再验证」后再宣告完成。**

## 最小 pptxgenjs 示例

```javascript
const PptxGenJS = require("pptxgenjs");
const pptx = new PptxGenJS();
const slide = pptx.addSlide();
slide.addText("标题", { x: 0.5, y: 0.4, w: 9, h: 0.8, fontSize: 36, bold: true, color: "1E2761" });
slide.addText("支持句", { x: 0.5, y: 1.3, w: 9, h: 0.5, fontSize: 16, color: "36454F" });
pptx.writeFile({ fileName: "output.pptx" });
```

## 交付检查清单

- [ ] 无占位符与明显错别字
- [ ] 版式统一，对比度足够
- [ ] 已渲图并用 `Read` 抽查（有视觉模型时）
- [ ] 最终文件位于任务工作区内
- [ ] 已 **`artifact_publish`**

## 明确不做

- 只写文件不发布 ≠ 交付完成。
- 缺渲图依赖时不假装做过视觉 QA：如实说明并给出安装路径。
