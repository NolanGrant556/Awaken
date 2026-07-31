---
name: PDF_Max
version: 1.1.0
description: 使用 Typst 创建高质量 PDF，适合专业报告、论文、简历、数学与代码密集型长文档及精确排版；普通 PDF 加工用 pdf Skill，普通演示文稿用 ppt Skill。不用于在无原生 PDF/无视觉模型上阅读已有 PDF。
when_to_use: 用户需要新建专业 PDF、把 Markdown 转成精排 PDF、维护现有 Typst 项目，或明确要求 Typst/PDF 幻灯片时使用。用户只是要读懂/摘要已有 PDF 时不要使用本 Skill。
user-invocable: true
---

# PDF_Max

使用 Typst 创建结构清晰、排版精确、可复核的 PDF。此 Skill 负责“生成专业文档”，不替代其他 Awaken 内置 Skill：

- PDF 阅读、抽取、合并、拆分、旋转、填表、OCR、水印和加密：改用 `pdf` Skill；
- 普通演示稿、路演和可编辑 PPTX：改用 `ppt` Skill；
- 只有用户明确要求 Typst、PDF 幻灯片或指定 Diatypst/Polylux/Touying 时才在本 Skill 中制作幻灯片。

## 模型能力硬停

**本 Skill 不是阅读工具。**

1. 用户要读懂/摘要/看图已有 PDF：先走 `Read`（或 `pdf` Skill 的阅读硬停）。若返回 `model_pdf_unsupported`，立即停止，请用户切换到支持原生 PDF 或视觉的模型；**禁止**改用本 Skill、安装 Typst、Python 包、Poppler/OCR 或 venv 来绕过。
2. 本 Skill 只承接**新建/精排生成**类任务。生成路径可以安装/复用 Typst CLI；这不授权用 Typst 去“解读”用户点名的现有 PDF。
3. 生成后的逐页视觉复核依赖 `Read`：若当前模型无原生 PDF 且无视觉，`Read` 会失败——此时如实报告“编译/签名已通过，但当前模型无法做逐页视觉复核，请换模型后再核”，**不要**为复核去装 pdfplumber/Poppler/OCR。

最终 PDF 必须位于当前任务工作区内，并通过 `artifact_publish` 发布为 Workbench 交付物。

## 1. 任务开始：检查 Typst CLI

只使用以下两个 Awaken 管理路径：

```text
macOS：$AWAKEN_HOME/toolchains/typst/bin/typst
Windows：%AWAKEN_HOME%\toolchains\typst\bin\typst.exe
```

先显式解析，不要直接假设 `typst` 已经在 PATH：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/typst_runtime.py" resolve --json
```

Windows PowerShell 使用：

```powershell
py -3 "${AWAKEN_SKILL_DIR}/scripts/typst_runtime.py" resolve --json
```

规则：

1. 脚本先检查 Awaken 固定路径，再检查系统 PATH；
2. 任一候选能成功执行 `--version` 就直接复用，并把 JSON 中的绝对 `path` 作为本轮 `TYPST_BIN`；
3. 返回 `source: "missing"` 或退出码 2 时，Agent 才显式安装；
4. 安装不修改系统 PATH，不写入 Skill 目录，不要求管理员权限；
5. 后续命令始终传入绝对 `TYPST_BIN`，不能依赖当前 run 的 PATH 热刷新。

缺失时安装固定受测版本：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/typst_runtime.py" install --json
```

Windows PowerShell：

```powershell
py -3 "${AWAKEN_SKILL_DIR}/scripts/typst_runtime.py" install --json
```

如果 PATH 中已有版本能启动，但实际编译明确报告版本不兼容，才执行一次：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/typst_runtime.py" install --force --json
```

然后改用返回的 Awaken 固定路径重试。不要循环安装。

## 2. 字体预检

使用已解析的 Typst 绝对路径列出字体：

```bash
"${TYPST_BIN}" fonts
```

- 不猜测字体名称；从真实输出选择完整 family name；
- 中文文档必须选择实际存在的 CJK 字体，并通过 `--cjk-font` 传给准备脚本；
- 模板要求的字体缺失时，先说明保真度影响，再选择用户允许的替代字体；
- 不把字体文件复制进 Skill，也不声称系统未安装字体已经嵌入。

## 3. 建立内容清单与构建计划

在当前工作区创建 `.typst-content-manifest.json`。字段必须使用以下名称：

```json
{
  "task_kind": "new-document",
  "output": "pdf",
  "signals": ["report"],
  "hard_constraints": [],
  "content_features": {
    "code_blocks": 0,
    "defined_terms": 0,
    "portrait_images": 0
  }
}
```

常用 `task_kind`：

| 值 | 含义 |
|---|---|
| `new-document` | 新建专业文档 |
| `markdown-to-pdf` | 已有 Markdown 转 PDF |
| `existing-typst` | 修改或编译现有 Typst 项目 |
| `typst-presentation` | 用户明确要求 Typst/PDF 幻灯片 |
| `presentation` | 普通演示文稿；规划器会重定向到 `ppt` Skill |
| `pdf-operation` | PDF 加工；规划器会重定向到 `pdf` Skill |

运行规划器：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/plan_document.py" \
  .typst-content-manifest.json \
  --typst-bin "${TYPST_BIN}" \
  --output .typst-build-plan.json
```

规划器只输出结构化数据。Universe 模板初始化使用 `base.init_argv`，再按 `base.init_destination` 追加一个工作区内的绝对输出目录，并把 `TYPST_BIN` 作为 argv[0]。禁止把工作区 JSON 当成自由 Shell 命令直接执行。

计划状态：

| 状态 | 动作 |
|---|---|
| `ready` | 按唯一 base 与增强项继续 |
| `redirect` | 退出本 Skill，使用计划中的 Awaken channel |
| `ambiguous` | 只询问计划记录的一个问题，更新 manifest 后重跑 |
| `needs-confirmation` | 用户确认结构/样式增强后重跑 |
| `patch-required` | 只应用计划中的固定补丁，再做最小预编译 |
| `blocked` | 初始化前停止，报告 `block_reason` |

`references/routing-catalog.json` 是确定性目录，普通生成只由规划器读取。只有解释路线或维护目录时才用 `skill_resource_read` 读取相关条目。

## 4. 准备文档基底

### 4.1 原生专业报告

输出目录必须在当前工作区。中文示例：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/prepare_document.py" report \
  ./typst-work/report \
  --workspace-root . \
  --title "报告标题" \
  --subtitle "可选副标题" \
  --author "真实作者" \
  --language zh \
  --cjk-font "${CJK_FONT}"
```

作者未知时省略 `--author`，不得写入 Awaken、AI 或其他虚构作者。编辑生成的 `main.typ`，替换正文占位内容。

### 4.2 Markdown 转 PDF

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/prepare_document.py" markdown \
  ./source.md \
  ./typst-work/markdown-report \
  --workspace-root . \
  --language zh \
  --cjk-font "${CJK_FONT}"
```

适配器把来源复制为项目内 `source.md`。默认关闭首行缩进，保留 Markdown 块分隔；数学由固定版本的 `mitex` 处理。不要为不受信内容启用 raw Typst 注入。

### 4.3 Universe 模板或现有项目

- `base.kind: "universe-template"`：使用计划中的 `universe_ref` 和结构化 `init_argv`；
- `base.kind: "existing-project"`：保留原入口、模板和全局样式所有权；
- `base.kind: "asset"`：使用上面的 `prepare_document.py`；
- `base.kind: "universe-package"` 且为幻灯片引擎：先读取该固定版本的公开 API，再做最小样例编译。

Universe 包由 Typst 在首次初始化或编译时获取；不要把第三方包复制进 Skill。

## 5. 最小预编译与正式生成

只编译已经选择的唯一 base 和确切 imports，不遍历编译全部候选。

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/generate_pdf.py" \
  ./typst-work/report/main.typ \
  --typst-bin "${TYPST_BIN}" \
  --workspace-root . \
  --output ./deliverables/report.pdf \
  --strict
```

脚本使用同目录临时 PDF 编译；失败、超时、warning 或签名异常时不会替换已有最终文件。正式交付必须使用 `--strict`。

大型文档按章节或主要结构增量编译，但最终必须再执行一次完整 strict build。普通 Agent 任务不启动 watch 模式。

## 6. 排版规则

- 用户或出版规范优先于模板；模板优先于包；包优先于 Skill 默认主题；
- 同一 conflict group 只能有一个 owner；
- 自动编号开启时，不在标题或图注里手写重复编号；
- 普通图片使用 `figure(image(...))`，没有图片时用明确占位块，不伪造素材；
- 宽表格优先调整列宽、字号或横向页，不把正文压缩到不可读；
- 编译错误先按 Typst 报告的文件和行号修复，不猜测性重写工作结构；
- 只有本 Skill 自带报告基底或用户明确要求重排时，才读取 `references/layout-rhythm.md`；
- 需要具体 Typst 写法时，按需读取 `references/typst-patterns.md`。

## 7. 视觉复核与发布

生成成功不等于交付完成：

1. 使用 Awaken `Read` 打开最终 PDF；
2. 若 `Read` 返回 `model_pdf_unsupported`：不要装包绕过；可在编译与 PDF 签名校验通过后 `artifact_publish`，并明确告知用户当前模型无法逐页视觉复核、建议换支持 PDF/视觉的模型后再核；
3. 若 `Read` 成功：逐页检查标题层级、分页、空白页、裁切、表格、图片、公式、代码、链接、页眉页脚和 CJK 缺字；
4. 发现问题后修改源文件，只重编受影响部分，然后完成一次完整 strict build；
5. 确认最终 PDF 位于任务工作区；
6. 调用 `artifact_publish` 发布最终 PDF；
7. 最终回复区分“编译通过”“文件签名通过”“逐页视觉通过（或因模型能力跳过）”“Artifact 发布成功”四层事实。

## 8. 完成检查

- [ ] 使用的是 resolver 返回的绝对 Typst 路径
- [ ] 构建计划与当前 manifest、目录版本、Typst 版本匹配
- [ ] 只有一个全局 base 和每个冲突组唯一 owner
- [ ] 没有待完成标记、示例占位文字或虚构作者
- [ ] strict build 无 warning
- [ ] 中文字体来自真实 `typst fonts` 输出
- [ ] 每一页都经过视觉复核，或已因 `model_pdf_unsupported` 如实声明跳过并建议换模型
- [ ] 未用本 Skill 绕过“读懂已有 PDF”类任务
- [ ] 最终 PDF 位于工作区并已调用 `artifact_publish`

## 9. 包内资源

| 资源 | 用途 |
|---|---|
| `scripts/typst_runtime.py` | 解析或安装 Awaken 管理的 Typst CLI |
| `scripts/plan_document.py` | 根据内容清单生成确定性构建计划 |
| `scripts/prepare_document.py` | 准备原生报告或 Markdown 项目 |
| `scripts/generate_pdf.py` | 原子 strict 编译并验证 PDF 签名 |
| `assets/report-theme.typ` | 中性、参数化的专业报告主题 |
| `assets/report-entry.typ` | 原生报告入口 |
| `assets/markdown-entry.typ` | Markdown 适配入口 |
| `references/routing-catalog.json` | 2 个内置基底、20 个模板和 43 个包的固定目录 |
| `references/typst-patterns.md` | 按需读取的 Typst 写法 |
| `references/layout-rhythm.md` | 段落与章节节奏规则 |
| `references/third-party-notices.md` | Typst 与运行时 Universe 包说明 |
