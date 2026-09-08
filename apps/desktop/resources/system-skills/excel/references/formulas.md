# Excel 公式兼容与验证

## 优先选择

在需要由 LibreOffice 参与重算和验证时，优先使用兼容性稳定的函数，例如：

- `SUM`、`SUMIF/SUMIFS`；
- `COUNTIF/COUNTIFS`；
- `INDEX` + `MATCH`；
- `IF`、`IFERROR`；
- `SUMPRODUCT`；
- `VLOOKUP/HLOOKUP`（仅在结构确实适合时）。

## 高风险函数

`XLOOKUP`、`XMATCH`、`FILTER`、`SORT`、`UNIQUE`、`SEQUENCE` 等动态数组或新函数的可用性取决于 Excel、LibreOffice 和写入库版本。即使没有显示错误，也可能只写出左上角结果或缺少 spill 元数据。

处理方式：

1. 先用 `inspect_workbook.py` 识别；
2. 用户没有强制要求时，改用 `INDEX/MATCH` 或在 Python 中先完成排序、筛选、去重，再把数据和稳定公式写入；
3. 用户明确要求现代 Excel 函数时，保留公式并说明 LibreOffice 不能作为充分验收，需要真实 Excel 复核。

## 公式审查

- 检查每个投影区间首列、中间列和末列；
- 检查绝对引用 `$A$1`、混合引用 `$A1/A$1`；
- 检查跨表引用中的引号；
- 检查 total range 是否漏行或多含表头；
- 对除法使用明确的零值策略；
- 对日期边界、闰年、月末和文本日期做样本验证；
- 对百分比确认底层值为 `0.15` 而不是 `15`；
- 对外部链接公式保留原始缓存与原件，禁止无备份重算。

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
