---
name: excel
version: 2.2.0
description: 创建、读取、编辑、清洗和校验电子表格（.xlsx/.xlsm/.xltx/.csv/.tsv）。只要表格文件是主要输入或输出，就应使用本 Skill；包括公式、格式、图表、财务模型、脏表整理、模板填充和格式转换。若表格只是输入而最终还要交付 Word/PPT/PDF，应再加载对应的最小 Skill 集合。
when_to_use: 用户提到 Excel、xlsx、xlsm、xltx、csv、tsv、工作簿、公式、财务模型、表格清洗或需要产出可继续编辑的表格文件时。
user-invocable: true
---

# Awaken Excel Skill

本 Skill 使用 `Bash + openpyxl/pandas + LibreOffice` 完成电子表格工作。包内 Python helper 支持 Python 3.9 及以上；缺少依赖时先按 route 探测并复用 doctor 返回的绝对执行计划，再通过 Awaken 现有安装能力或 `Bash` 恢复。公式工作簿必须重算、检查错误并进行语义抽查，最终通过 `artifact_publish` 发布。

## 先判断任务路线

| 任务 | 首选路线 |
| --- | --- |
| 快速阅读/摘要 | `markitdown` 或 pandas；需要单元格坐标时用 openpyxl |
| 批量清洗、聚合、格式转换 | pandas |
| 公式、样式、多 sheet、图表、批注 | openpyxl |
| 阅读公式模型 | 同一文件分别以公式模式和 `data_only=True` 加载 |
| 编辑现有模板 | 先检查原格式、输入单元格、宏和外链，再只修改目标位置 |
| 含公式的最终文件 | `inspect_workbook.py` → `recalc.py` → 再次 inspect |

## Awaken 工具与路径合同

| 动作 | 工具 |
| --- | --- |
| Python、LibreOffice、转换和校验 | `Bash` |
| 安装依赖 | 优先执行 Awaken 提供的 `dependency_install` 计划，否则用 `Bash` |
| 读取脚本、CSV/TSV 和校验 JSON | `Read` |
| 修改数据处理脚本 | `Edit` / `Write` |
| 查找数据文件 | `Glob` / `Grep` |
| 发布最终交付物 | `artifact_publish`（必做） |

- 包内脚本一律通过 `${AWAKEN_SKILL_DIR}` 的绝对路径调用。
- Windows 注入到正文的 Skill 路径使用 `C:/...` 形式，可同时作为 Git Bash、PowerShell、cmd 和 Python 参数；不要改回反斜杠路径再交给 POSIX Shell。
- 最终表格必须位于当前任务工作区的 output root。
- 编辑用户原文件时默认输出新文件，不覆盖原件。
- Skill 已加载不代表 Python 包或 LibreOffice 已存在；以真实探测和执行结果为准。
- 用户在初始请求中点名表格输入、Excel 输出或跨表分析时，在任何相关 Bash、文件读取或 subagent 派发前加载本 Skill；子任务必须继承表头、类型、空值和分母检查边界。
- doctor 与所有 `--json` helper 必须作为独立命令执行，或放在 `&&` 链的末尾；禁止接 `| tail`、`| head`、`grep`、命令替换等会改变退出码或截断 JSON 的管道。
- 必须同时核对 Bash outcome 与完整 JSON `status`。wrapper 失败时不得改用裸 `soffice` 绕过独立 profile、超时、外链拒绝和原子替换合同。

## 第一步：依赖探测与恢复

按当前动作只探测对应 route。macOS、Linux 或 Windows Git Bash 使用当前快照已观察到的 `python3`/`python`：

```bash
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route read_precise --dependency-root "." --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route clean --dependency-root "." --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route create_edit --dependency-root "." --json
python3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route recalc --dependency-root "." --json
```

Windows 优先根据环境快照使用已观察到的 Python Launcher；`py` 可用时命令为：

```text
py -3 -B "${AWAKEN_SKILL_DIR}/scripts/doctor.py" --route read_precise --dependency-root "." --json
```

若快照只有 `python`，把 `py -3` 换成 `python`。两者都未观察到时，不要反复猜测命令名；报告 Python 3.9+ 缺失并按正常安装路线恢复。Bash 结果会报告实际 Shell；后续在 PowerShell 中调用 doctor 返回的绝对 Python 路径时使用 `& "<execution_plan.python>" ...`，Git Bash/cmd 使用 `"<execution_plan.python>" ...`。

后续命令固定使用 `execution_plan.python` 和 `execution_plan.libreoffice` 的绝对路径；不要重新调用裸 `python3` 猜测同一 run 的解释器。

根据 JSON 只安装缺失项：

- `openpyxl`：公式、样式、多表、图表和精确单元格操作；
- `pandas`：批量数据清洗和分析；
- `markitdown[xlsx]`：快速文本浏览；
- LibreOffice：公式重算。

macOS PEP 668 环境优先使用工作区 `.venv` 或 `uv`；Windows 可使用 `py -m pip`。安装完成后重新运行 doctor，不根据安装命令退出码之外的猜测继续。

## 编辑前检查（必须）

```bash
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/inspect_workbook.py" input.xlsx --json > ./workbook-inspection.json
```

先确认：

- sheet 名称、隐藏状态、使用范围；
- 公式数量、缓存缺失和已有错误；
- 是否含 VBA 宏；
- 是否含外部工作簿链接；
- 是否使用本运行环境无法可靠验证的动态数组/新函数；
- 原模板如何标识输入单元格、公式、跨表引用和外链。

多工作簿或脏数据分析还必须先抽样建立输入剖面：候选表头、每列实际类型、空值、状态文本混入数值列、日期格式和零分母。数值转换使用显式安全函数并保留无法转换的样本；不得直接 `float(value)`，不得假定固定表头行，也不得在未检查分母时计算比例。完成剖面后再派发并行分析。

`.xlsm` 使用 openpyxl 编辑时必须 `keep_vba=True`；即便如此也要认识到 openpyxl 不能完整维护所有 Excel 扩展。含宏和复杂外链的文件不能在没有备份与明确风险说明时保存或重算。

## 每个输出都必须满足

- 用户指定的 sheet 名、列名、顺序、公式和格式优先于本 Skill 的默认建议；
- 需要随输入变化的结果使用 Excel 公式，不写死 Python 计算值；
- 关键假设与硬编码数字在工作簿可见位置写明来源或说明；
- 新建“供人填写”的模板必须有简短图例和一行真实格式示例；编辑已有文件时不得擅自插入示例行；
- 编辑已有文件时匹配其样式和输入约定，不把自己的财务配色强加给用户模板；
- 公式错误为零；但“零错误”仍不能证明引用范围和业务逻辑正确。

## openpyxl 铁律

- `data_only=True` 只读缓存值，公式字符串已经消失；这个 workbook 绝不能保存；
- 同时读取公式和值需要加载两次；
- 刚由 openpyxl 写出的公式没有缓存值，必须重算后再用 `data_only=True` 检查；
- merged range 只能写左上角 anchor；其他 `MergedCell` 是只读的；
- sheet 名含空格时，跨表引用必须加单引号，例如 `='Assumptions Inputs'!$B$5`；
- `.xlsm` 加载时使用 `keep_vba=True`，并在输出后确认宏部件仍存在；
- 外链、图表、条件格式、数据验证、命名区域和 Excel 扩展需要单独复核。

按需读取 `references/formulas.md`、`references/editing.md` 和 `references/financial-model.md`。

## 重算（含公式时必须）

对 Agent 新生成的 `output.xlsx`：

```bash
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/recalc.py" output.xlsx --json > ./recalc-result.json
```

对用户原文件的编辑结果，显式输出新路径：

```bash
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/recalc.py" edited-before-recalc.xlsx --output final.xlsx --json > ./recalc-result.json
```

脚本使用独立 LibreOffice profile，在临时副本上重算；只有 LibreOffice 真正写回后才原子替换目标。以下情况必须失败而不是假成功：

- LibreOffice 未找到、非零退出或超时；
- 文件包含外部工作簿链接；
- 有公式但文件没有被真正写回；
- 输出无法再由 openpyxl 读取。

随后用 `Read` 打开完整 `recalc-result.json`。若 JSON `status=errors_found`，根据 `error_summary` 修复后再次重算；若 `status=failed` 或 Bash 非零则路线失败。不能只看进程是否启动、Bash 是否打印了文件路径或管道末端退出码。

## 公式语义抽查

重算成功后：

1. 抽查 2–3 个关键公式的输入、范围、绝对/相对引用和结果；
2. 检查横向/纵向复制的公式是否连续一致；
3. 检查除数为零、空值、日期边界和单位换算；
4. 检查总计是否遗漏最后一行或包含表头；
5. 再运行 `inspect_workbook.py`，确认缓存和错误状态。

## 财务模型默认习惯

仅在用户或现有模板没有其他规则时使用：

- 蓝色字体：硬编码输入和情景参数；黑色：公式；绿色：同工作簿跨表引用；红色：外部文件引用；黄色填充：需要用户填写的关键假设；
- 金额单位写在表头；负数用括号；零显示为 `-`；百分比以小数存储；估值倍数显示 `0.0x`；年份存为文本；
- 每个假设放在独立、有标签的单元格，公式引用假设，不在公式中散落魔法数字；
- 各预测期公式保持一致，异常的单个手工覆盖必须明确标注。

## 发布前检查

- [ ] sheet、列、公式和格式符合用户要求
- [ ] 没有公式错误，缓存已刷新
- [ ] 已抽查关键公式语义，不只检查 `#REF!`
- [ ] 宏、外链和高级特性的风险已处理或明确说明
- [ ] 表头、列宽、冻结窗格、数字格式和中文显示可读
- [ ] 最终文件位于工作区 output root
- [ ] 已调用 `artifact_publish`

## 明确边界

- 当前 `recalc.py` 只接受 `.xlsx`，避免把宏或模板转换成普通工作簿时静默丢失能力。
- `.xlsm/.xltx` 可以作为输入进行受控分析；需要正式发布时根据用户要求和 `artifact_publish` 当前支持类型输出 `.xlsx/.xls/.csv/.tsv`。
- Google Sheets API、数据库管道和在线协作权限不是本 Skill 的默认执行路线。
