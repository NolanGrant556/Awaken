---
name: ppt
version: 2.0.0
description: 创建、读取、编辑和校验 PowerPoint 演示文稿（.pptx）或模板（.potx）。只要 PPTX/POTX 是主要输入或输出，就应使用本 Skill；包括路演、汇报、培训材料、模板改稿、图表、备注、合并拆分页和视觉检查。若演示文稿只是输入而最终还要交付 Word/Excel/PDF，应再加载对应的最小 Skill 集合。
when_to_use: 用户提到 PPT、PowerPoint、pptx、potx、deck、slides、演示稿、路演、汇报页或套用演示模板时。
user-invocable: true
---

# Awaken PPT Skill

本 Skill 使用 `Bash + PptxGenJS/python-pptx + LibreOffice/Poppler` 完成演示文稿工作。缺少依赖时先探测，再通过 Awaken 现有安装能力或 `Bash` 恢复；完成内容、结构、视觉三层 QA 后，必须调用 `artifact_publish`。

## 先判断任务路线

| 任务 | 首选路线 |
| --- | --- |
| 阅读内容 | `markitdown` 提取逐页文本；同时生成缩略图理解版式 |
| 从零创建 | PptxGenJS，保持原生文本、形状和图表 |
| 普通模板填充 | 先做缩略图地图，再用 `python-pptx` 逐 run 修改 |
| 高保真模板改稿 | 安全解包 OOXML，只修改目标 slide/relationship，重新打包并与原件做结构基线 |
| 内容合并/拆分 | 优先在生成脚本中重建；直接复制 slide XML 前必须同步 relationship、content type 与 presentation 注册 |
| 视觉复核 | 严格渲染到全新目录，逐页 `Read`，修复后重新渲染 |

## Awaken 工具与路径合同

| 动作 | 工具 |
| --- | --- |
| Node/Python、生成、转换、渲染 | `Bash` |
| 安装依赖 | 优先执行 Awaken 提供的 `dependency_install` 计划，否则用 `Bash` |
| 读取文本、脚本和逐页预览图 | `Read` |
| 修改生成脚本或已解包 XML | `Edit` / `Write` |
| 查找素材和占位内容 | `Glob` / `Grep` |
| 发布最终交付物 | `artifact_publish`（必做） |

- 包内脚本一律通过 `${AWAKEN_SKILL_DIR}` 的绝对路径调用。
- 最终 `.pptx` 必须位于当前任务工作区的 output root。
- 不复用旧 PDF、旧页图或旧缩略图证明本轮 QA；每轮使用新目录。
- 不直接 `rm` 用户目录中的文件；生成脚本使用唯一临时目录和新输出路径。

## 第一步：依赖探测与恢复

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --json
```

根据 JSON 只安装缺失项：

- 创建：`pptxgenjs`；图标路线可选 `react`、`react-dom`、`react-icons`、`sharp`；
- 编辑/读取：`python-pptx`、`markitdown[pptx]`；
- 缩略图：`Pillow`；
- 转换与视觉 QA：LibreOffice、Poppler (`pdftoppm`)。

依赖可以由 Awaken 恢复，但必须以真实安装结果为准，不能因为 Skill 已加载就假定环境已具备。

## 从零创建

创建前读取 `references/pptxgenjs.md`。关键规则：

- 添加第一页前设置明确 layout；所有坐标必须落在真实画布内；
- 十六进制颜色不带 `#`，透明度使用专用字段；
- 不复用会被 PptxGenJS 原地修改的 options/shadow 对象；
- 文本框需要对齐时显式设置 margin；
- 列表使用 bullet/paragraph 结构，不手写项目符号；
- PowerPoint 有原生能力的图表使用 `addChart`，不要先渲成图片；
- 堆积图、组合图和次坐标轴必须通过结构校验；
- speaker notes 使用 `slide.addNotes()`，不要画在页面上。

## 模板编辑

先建立版式地图：

```bash
python3 -m markitdown template.pptx > template-content.md
python3 -B "${AWAKEN_SKILL_DIR}/scripts/thumbnail.py" template.pptx template-map.jpg --json
```

再按 `references/editing.md` 修改。不要把所有内容都塞进同一个“标题 + 项目符号”模板页；布局数量、内容数量不一致时应删除整组多余视觉元素，而不是只清空文字。

高保真解包路线：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" unpack template.pptx ./pptx-unpacked
# Read/Edit 精确修改 ppt/slides、notesSlides、charts 或 relationships
python3 -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" pack ./pptx-unpacked output.pptx
python3 -B "${AWAKEN_SKILL_DIR}/scripts/validate_pptx.py" output.pptx --original template.pptx --json
```

结构性增删页必须先完成，再修改具体页面内容；否则复制页可能把已替换的内容或共享图表关系一起复制。

## 设计要求

- 每页有一个清晰视觉锚点：图片、图表、图标、流程、比较或关键数字；
- 一种主色承担约 60–70% 视觉重量，辅色和强调色克制使用；
- 标题页、章节页、内容页形成节奏，不重复同一个布局；
- 标题通常 36–44pt，正文 14–18pt，边距至少约 0.5 英寸；
- 正文左对齐，不居中大段文字；
- 不默认万能蓝、米黄色背景、标题下细线、贯穿整页的装饰色条或卡片单侧色边；
- 图表、图标和文字都要满足对比度；
- 字体优先用户品牌字体；没有指定时选择目标平台和 LibreOffice 都能可靠替代的字体，并为中文预留宽度余量。

## 三层 QA（必须）

### 1. 内容 QA

```bash
python3 -m markitdown output.pptx > output.check.md
grep -iE "xxxx|lorem|ipsum|TODO|占位|待填|insert.*(slide|page)" output.check.md || true
```

检查缺页、错序、错别字、数字与来源、演讲备注、残留模板说明。

### 2. 结构 QA

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/validate_pptx.py" output.pptx --json
```

模板改稿增加 `--original template.pptx`。校验包括 ZIP/XML、relationship、content type、页面注册、重复 shape ID、图表坐标轴引用和堆积图标签位置。

### 3. 视觉 QA

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/render_slides.py" output.pptx ./preview-ppt-1 --json
```

对 JSON 返回的每一张页图调用 `Read`，优先检查：

1. 文字溢出、裁切和过度换行；
2. 元素重叠、线条穿字；
3. 边距、列和卡片间距不一致；
4. 字体替代导致的布局变化；
5. 低对比度、图片变形、图表标签缺失；
6. 页脚/来源与正文碰撞；
7. 残留占位内容或模板装饰错位。

发现问题后修改源脚本或模板变换，再用新目录完整重渲染。不能只修改 PPTX 而复用旧 PDF/图片。

## 发布前检查

- [ ] 内容、顺序、数字、来源和备注符合要求
- [ ] 结构校验通过
- [ ] 每一张可见幻灯片已经视觉检查
- [ ] 至少完成一轮“发现问题 → 修复 → 重新验证”；若首轮确无问题，记录实际检查结果
- [ ] 最终文件位于工作区 output root
- [ ] 已调用 `artifact_publish`

## 明确边界

- `.potx` 可以作为输入模板；当前正式发布时输出 `.pptx`。
- LibreOffice 能渲染不等于 PowerPoint 一定接受，必须同时执行结构校验。
- 结构和视觉 QA 不证明业务数字正确，关键数据仍要回查来源。
