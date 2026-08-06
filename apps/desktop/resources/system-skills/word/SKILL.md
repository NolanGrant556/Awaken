---
name: word
version: 2.2.0
description: 创建、读取、编辑和校验 Word 文档（.docx）或 Word 模板（.dotx）。只要 .docx/.dotx 是主要输入或输出，就应使用本 Skill；包括专业报告、备忘录、函件、目录、页眉页脚、表格、图片、批注、修订和高保真模板改稿。若 Word 只是输入而最终还要交付 Excel/PPT/PDF，应再加载对应的最小 Skill 集合。
when_to_use: 用户提到 Word、docx、dotx、报告文档、修订、批注、套用 Word 模板，或需要把内容整理成可交付 Word 文件时。
user-invocable: true
---

# Awaken Word Skill

本 Skill 使用 Awaken 现有的 `Bash`、文件工具和本机 Office 依赖完成 Word 工作，不注册新的基础工具。包内 Python helper 支持 Python 3.9 及以上；缺少依赖时先按 route 探测并复用 doctor 返回的绝对执行计划，再使用 `dependency_install` 返回的可用计划或 `Bash` 安装。文件验证完成后必须调用 `artifact_publish`。

## 先判断任务路线

| 任务 | 首选路线 |
| --- | --- |
| 阅读、摘录、总结 | 只使用包内 `scripts/read_docx.py`；它以 Python 标准库读取 OOXML，并同时返回可分析 Markdown、结构化 blocks 和未覆盖内容 warnings |
| 从零创建 | Node `docx`；简单场景也可用 `python-docx` |
| 修改普通文档 | 能证明目标结构简单时使用 `python-docx`，修改后仍需结构与视觉 QA |
| 高保真模板改稿 | 安全解包 → 只改目标 XML → 重新打包 → 与原件做基线校验 |
| 批注、修订 | 使用包内专用脚本或 OOXML 路线，不用 Pandoc 往返冒充高保真修改 |
| 旧 `.doc` | 先用 LibreOffice 转换为 `.docx`，再按上述路线处理 |

只要输入文件包含复杂页眉页脚、文本框、批注、修订、域、目录、嵌入对象或自定义 XML，就不要默认用 `python-docx` 整体读写；它可能丢弃未建模内容。

## Awaken 工具与路径合同

| 动作 | 工具 |
| --- | --- |
| 运行包内脚本、Node/Python、LibreOffice、Poppler | `Bash` |
| 安装缺失依赖 | 优先执行 Awaken 提供的 `dependency_install` 计划，否则用 `Bash` |
| 读取文本、脚本和渲染页图 | `Read` |
| 修改生成脚本、中间 Markdown、已解包 XML | `Edit` / `Write` |
| 查找目标文件和文本 | `Glob` / `Grep` |
| 发布最终交付物 | `artifact_publish`（必做） |

- 包内脚本一律通过 `${AWAKEN_SKILL_DIR}` 的绝对路径调用。
- 最终 `.docx` 必须位于当前任务工作区的 output root 内。
- 临时解包和预览不得覆盖用户原文件；默认写到新的输出路径。
- 对外部 DOCX 解包时必须使用 `scripts/ooxml_package.py`，不要直接信任压缩包路径或符号链接。
- 用户在初始请求中点名 Word 输入或输出时，在任何相关 Bash、文件读取或 subagent 派发前加载本 Skill；同时点名多种 Office 格式时先加载对应的最小 Skill 集合，并把各格式的读取/验证边界写入子任务。
- doctor 与所有 `--json` helper 必须作为独立命令执行，或放在 `&&` 链的末尾；禁止接 `| tail`、`| head`、`grep`、命令替换等会改变退出码或截断 JSON 的管道。
- 对 JSON helper 必须同时核对 Bash outcome 与完整 `status`；任何 `failed`、结构 issues 或非零退出都属于失败。wrapper 失败时不得改用裸 `soffice` / `pdftoppm` 绕过独立 profile、超时和结果合同。

## 阅读固定路线

阅读 `.docx` / `.dotx` 不安装 Pandoc、MarkItDown、LibreOffice 或第三方 Python 包，也不先手工解包。先用当前平台已有的 Python 启动器执行一次 read route doctor（macOS/Linux 通常是 `python3`，Windows 通常是 `py -3` 或 `python`）：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route read --dependency-root "$PWD" --json
```

Windows PowerShell 示例为 `py -3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route read --dependency-root "$PWD" --json`。这一步只用于取得 `execution_plan.python` 并确认 bundled reader 存在，不探测或安装外部文档依赖。随后用 doctor 返回的绝对解释器路径执行唯一阅读 CLI；路径始终作为一个安全参数引用，PowerShell 调用绝对路径时在前面加 `&`：

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/read_docx.py" input.docx --format json --output ./docx-read.json
```

随后用 `Read` 打开 `docx-read.json`。JSON 中：

- `markdown` 是按文档顺序线性化的正文、标题、列表、超链接、表格、脚注/尾注、批注和页眉页脚；
- `blocks` 保留段落/表格等结构，表格行列分析优先使用这里的 `rows`，不要反解析 Markdown；
- `warnings` 明确指出图片/绘图、文本框、公式、域、修订、合并单元格、嵌套表格、altChunk、嵌入对象、图表或 SmartArt 等未完整覆盖或被线性化的内容；
- `stats` 用于核对实际提取到的对象数量。

有 warning 不等于读取失败，但必须按 warning 限定结论；当任务依赖图片、空间布局、复杂公式或嵌入对象时，不能把线性提取冒充完整理解，应补视觉 QA 或明确报告当前未覆盖内容。旧 `.doc` 不属于该 CLI 支持范围，需先通过 LibreOffice 输出新的 `.docx`。

## 按任务路线探测与恢复依赖

纯 DOCX 阅读只依赖正在执行脚本的 Python 标准库；read route doctor 只负责返回跨平台解释器路径。创建、OOXML 编辑、接受修订或视觉 QA 前，也只探测准备执行的路线：

```text
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route create --dependency-root "$PWD" --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route edit_ooxml --dependency-root "$PWD" --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route accept_changes --dependency-root "$PWD" --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route visual_qa --dependency-root "$PWD" --json
```

只根据目标 `routes.<route>.missing` 或 `missing_alternatives` 恢复依赖；其他路线 unavailable 不阻塞当前路线。常用依赖：

- 创建：`docx`（Node）或 `python-docx`；
- 阅读与 OOXML 包级处理：Python 标准库和包内脚本；
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

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" unpack input.docx ./docx-unpacked
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/merge_runs.py" ./docx-unpacked
# 使用 Read/Edit 精确修改 ./docx-unpacked/word/*.xml
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/ooxml_package.py" pack ./docx-unpacked output.docx
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/validate_docx.py" output.docx --original input.docx --json
```

`merge_runs.py` 只合并格式完全一致、且只包含普通文本的相邻 run；含域、超链接、批注标记、制表符、换行、绘图或修订的 run 不会合并。更完整的判断见 `references/editing.md`。

接受全部修订时输出新文件，不覆盖原件：

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/accept_changes.py" input.docx accepted.docx --json
```

超时、LibreOffice 未写回、输出仍含修订节点都必须视为失败。

## 三层 QA（必须）

### 1. 内容 QA

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/read_docx.py" output.docx --format json --output output.check.json
```

用 `Read` 检查 `output.check.json` 的 `markdown`、表格 `blocks` 与 `warnings`，核对缺段、错序、占位符、乱码、错误链接、空白项目符号和用户要求的关键字段。

### 2. 结构 QA

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/validate_docx.py" output.docx --json
```

模板改稿必须增加 `--original 原文件.docx`。脚本检查 ZIP 路径、必需部件、XML、关系目标、批注引用和修订元数据；它不把“结构可打开”冒充“内容正确”。

### 3. 视觉 QA

每轮使用一个新的预览目录：

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/render_docx.py" output.docx ./preview-word-1 --json > ./preview-word-1.result.json
```

先用 `Read` 打开完整结果 JSON；只有 Bash 成功且 `status=success` 才能继续。对 `images` 返回的每张页图调用 `Read`，逐页检查：文字溢出、分页孤行、表格跨页、中文字体回退、页眉页脚碰撞、图片变形、目录与页码、异常空白。保留实际检查的页码清单；发现问题后修复，使用新的预览目录重新生成并复核受影响页。未读取的页不能计入“逐页视觉通过”。

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
