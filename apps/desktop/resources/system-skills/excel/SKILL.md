---
name: excel
version: 1.0.0
description: 创建、阅读、编辑或整理电子表格（.xlsx / .xlsm / .csv / .tsv）。适用于新建工作簿、加列、公式、格式、图表、清洗脏表、财务模型与表格互转。用户提到 Excel、xlsx、表格文件时使用。主交付物必须是表格文件；不要在交付物是 Word/PPT/纯脚本时误用。
when_to_use: 需要产出或修改 Excel/CSV 时；需要用公式驱动的可更新模型时；需要清洗或重组表格数据时。
user-invocable: true
---

# Awaken Excel Skill

用 **Bash + pandas/openpyxl** 处理表格，公式用 Excel 公式而非 Python 硬编码结果；交付前用 LibreOffice 重算并经 **`artifact_publish`** 发布。

## Awaken 工具约定

| 步骤 | 工具 |
| --- | --- |
| 跑 Python / LibreOffice | `Bash` |
| 安装 LibreOffice 等系统级依赖 | `dependency_install`（优先），或 `Bash` |
| 读脚本、抽查 CSV 文本 | `Read` |
| 改生成脚本 | `Edit` / `Write` |
| 找数据文件 | `Glob` / `Grep` |
| 最终交付 | **`artifact_publish`** |

**重要**：最终表格文件必须生成在任务工作区（output roots）内，`artifact_publish` 不接受工作区外的路径。

## 依赖

```bash
# Python 库：新 macOS/Homebrew Python 受 PEP 668 保护。优先顺序：
#   1) uv pip install openpyxl pandas
#   2) python3 -m venv .venv && .venv/bin/pip install openpyxl pandas
#   3) pip3 install --user --break-system-packages openpyxl pandas
python3 -c "import openpyxl, pandas" 2>/dev/null || echo "need openpyxl pandas"

# 有公式时需要重算：缺 LibreOffice 优先用 dependency_install 安装
which soffice || which libreoffice
```

Windows 上把 `python3` / `pip3` 换成 `python` / `pip`（或 `py -m pip`）。

## 库怎么选

| 场景 | 库 |
| --- | --- |
| 分析、清洗、简单导出 | **pandas** |
| 公式、样式、多表、图表 | **openpyxl** |

## 铁律：用公式，不要硬编码计算结果

错误：在 Python 里 `sum()` 再把数字写进单元格。
正确：写入 `=SUM(B2:B9)`，再用重算脚本刷新缓存值。

## 标准流程

1. 选库 → 创建或 `load_workbook`。
2. 写入数据、公式、格式。
3. `wb.save("output.xlsx")`。
4. **有公式则必须重算**：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/recalc.py" output.xlsx
```

注意：重算脚本会就地覆写目标文件（LibreOffice 往返），图表等 openpyxl 本就不支持的对象可能丢失；要保留原件先自行备份。

5. 根据脚本 JSON 修 `#REF!` / `#DIV/0!` / `#VALUE!` / `#NAME?`。
6. 抽查：`python3 -c "import pandas as pd; print(pd.read_excel('output.xlsx').head())"`。
7. **`artifact_publish`**。

## 新建示例（openpyxl）

```python
from openpyxl import Workbook
from openpyxl.styles import Font, PatternFill, Alignment, Border, Side

wb = Workbook()
ws = wb.active
ws.title = "汇总"
ws["A1"] = "项目"
ws["B1"] = "金额"
ws["A1"].font = Font(bold=True)
ws.append(["销售收入", 100])
ws.append(["成本", 40])
ws["A4"] = "合计"
ws["B4"] = "=SUM(B2:B3)"
ws.column_dimensions["A"].width = 16
ws.column_dimensions["B"].width = 12
wb.save("output.xlsx")
```

## 编辑已有

```python
from openpyxl import load_workbook

wb = load_workbook("existing.xlsx")
ws = wb.active
ws["A1"] = "新标题"
wb.save("existing.xlsx")
```

注意：`data_only=True` 读到的是缓存值；若再保存会**毁掉公式**。重算用 `scripts/recalc.py`，不要用 data_only 回写。

## 财务模型习惯（可按用户模板覆盖）

- 输入：蓝色字体；公式：黑色；跨表引用：绿色（若项目有惯例则从其惯例）。
- 关键假设写独立单元格，公式引用假设，禁止魔法数字。
- 年份作文本；百分比 `0.0%`；负数用括号格式。
- 硬编码来源在旁注或批注写明出处。

## 阅读分析（pandas）

```python
import pandas as pd
df = pd.read_excel("file.xlsx")
df.head(); df.info(); df.describe()
```

## 交付检查清单

- [ ] 无公式错误
- [ ] 计算用公式而非写死数字（除非用户只要静态表）
- [ ] 列宽/表头可读，中文不乱码
- [ ] 最终文件位于任务工作区内
- [ ] 已 **`artifact_publish`**

## 明确不做

- 不把 Google Sheets API 或数据库管道当成默认路径。
- 缺 LibreOffice 时不假装已重算：如实报告并给出安装路径。
