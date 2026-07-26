---
name: word
version: 2.0.0
description: 创建、读取、编辑和校验 Word 文档（.docx）或 Word 模板（.dotx）。只要 .docx/.dotx 是主要输入或输出，就应使用本 Skill；包括专业报告、备忘录、函件、目录、页眉页脚、表格、图片、批注、修订和高保真模板改稿。若 Word 只是输入而最终还要交付 Excel/PPT/PDF，应再加载对应的最小 Skill 集合。
when_to_use: 用户提到 Word、docx、dotx、报告文档、修订、批注、套用 Word 模板，或需要把内容整理成可交付 Word 文件时。
user-invocable: true
---

# Awaken Word Skill

本 Skill 使用 Awaken 现有的 `Bash`、文件工具和本机 Office 依赖完成 Word 工作，不注册新的基础工具。缺少依赖时先探测，再使用 `dependency_install` 返回的可用计划或 `Bash` 安装；文件验证完成后必须调用 `artifact_publish`。

## 先判断任务路线

| 任务 | 首选路线 |
| --- | --- |
| 阅读、摘录、总结 | `pandoc` 或 `markitdown`；需要精确定位时再安全解包 OOXML |
| 从零创建 | Node `docx`；简单场景也可用 `python-docx` |
| 修改普通文档 | 能证明目标结构简单时使用 `python-docx`，修改后仍需结构与视觉 QA |
| 高保真模板改稿 | 安全解包 → 只改目标 XML → 重新打包 → 与原件做基线校验 |
| 批注、修订 | 使用包内专用脚本或 OOXML 路线，不用 Pandoc 往返冒充高保真修改 |
| 旧 `.doc` | 先用 LibreOffice 转换为 `.docx`，再按上述路线处理 |

只要输入文件包含复杂页眉页脚、文本框、批注、修订、域、目录、嵌入对象或自定义 XML，就不要默认用 `python-docx` 整体读写；它可能丢弃未建模内容。

## Awaken 工具与路径合同

| 动作 | 工具 |
| --- | --- |
| 运行脚本、Node/Python、Pandoc、LibreOffice、Poppler | `Bash` |
| 安装缺失依赖 | 优先执行 Awaken 提供的 `dependency_install` 计划，否则用 `Bash` |
| 读取文本、脚本和渲染页图 | `Read` |
| 修改生成脚本、中间 Markdown、已解包 XML | `Edit` / `Write` |
| 查找目标文件和文本 | `Glob` / `Grep` |
| 发布最终交付物 | `artifact_publish`（必做） |

- 包内脚本一律通过 `${AWAKEN_SKILL_DIR}` 的绝对路径调用。
- 最终 `.docx` 必须位于当前任务工作区的 output root 内。
- 临时解包和预览不得覆盖用户原文件；默认写到新的输出路径。
- 对外部 DOCX 解包时必须使用 `scripts/ooxml_package.py`，不要直接信任压缩包路径或符号链接。

## 第一步：依赖探测与恢复

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --json
```

根据 JSON 只安装缺失项。常用依赖：

- 创建：`docx`（Node）或 `python-docx`；
- 阅读：`pandoc`、`markitdown[docx]`；
- OOXML：`lxml`；
- 转换与视觉 QA：LibreOffice、Poppler (`pdftoppm`)。

macOS/Homebrew Python 遇到 PEP 668 时优先使用工作区 `.venv` 或 `uv`；不要反复执行同一条已失败的安装命令。Windows 可使用 `py -m pip`，命令名以 doctor 的真实探测结果为准。

## 创建新文档

创建前按需读取 `references/creation.md`。至少遵守：

- 明确页面尺寸和页边距；
- 标题使用内置 Heading/outline level，保证目录可用；
- 列表使用真正的 numbering，不手写 `•`；
- 表格同时设置表宽、列宽和单元格宽度，优先使用 DXA；
- `ImageRun` 明确图片类型；分页符必须放在段落内；
- 不用换行符拼接多个段落；不使用空表格冒充装饰线。

生成后先抽查文本，再执行结构和视觉 QA。

## 编辑已有文档

高保真路线：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" unpack input.docx ./docx-unpacked
python3 -B "${AWAKEN_SKILL_DIR}/scripts/merge_runs.py" ./docx-unpacked
# 使用 Read/Edit 精确修改 ./docx-unpacked/word/*.xml
python3 -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" pack ./docx-unpacked output.docx
python3 -B "${AWAKEN_SKILL_DIR}/scripts/validate_docx.py" output.docx --original input.docx --json
```

`merge_runs.py` 只合并格式完全一致、且只包含普通文本的相邻 run；含域、超链接、批注标记、制表符、换行、绘图或修订的 run 不会合并。更完整的判断见 `references/editing.md`。

接受全部修订时输出新文件，不覆盖原件：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/accept_changes.py" input.docx accepted.docx --json
```

超时、LibreOffice 未写回、输出仍含修订节点都必须视为失败。

## 三层 QA（必须）

### 1. 内容 QA

```bash
pandoc output.docx -t markdown -o output.check.md
# 或
python3 -m markitdown output.docx
```

检查缺段、错序、占位符、乱码、错误链接、空白项目符号和用户要求的关键字段。

### 2. 结构 QA

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/validate_docx.py" output.docx --json
```

模板改稿必须增加 `--original 原文件.docx`。脚本检查 ZIP 路径、必需部件、XML、关系目标、批注引用和修订元数据；它不把“结构可打开”冒充“内容正确”。

### 3. 视觉 QA

每轮使用一个新的预览目录：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/render_docx.py" output.docx ./preview-word-1 --json
```

对返回的每张页图调用 `Read`，逐页检查：文字溢出、分页孤行、表格跨页、中文字体回退、页眉页脚碰撞、图片变形、目录与页码、异常空白。发现问题后修复，使用新的预览目录重新生成并复核受影响页。

## 发布前检查

- [ ] 用户要求的正文、表格、图片、修订或批注都已完成
- [ ] 没有 lorem、TODO、xxxx、待填等占位内容
- [ ] 结构校验通过
- [ ] 已逐页视觉检查；无法视觉检查时明确说明，不能伪称通过
- [ ] 最终文件位于工作区 output root
- [ ] 已调用 `artifact_publish`，Workbench 出现可点击交付物卡片

## 明确边界

- `.dotx` 可以作为输入模板，但当前 `artifact_publish` 的正式 Word 交付类型是 `.doc/.docx`；需要发布时输出 `.docx`。
- PDF 主交付物交给 `pdf` 或用户明确选择的其他 PDF Skill；本 Skill 只负责 Word 部分。
- 结构校验、LibreOffice 可打开和视觉预览都不能替代业务事实核对。
