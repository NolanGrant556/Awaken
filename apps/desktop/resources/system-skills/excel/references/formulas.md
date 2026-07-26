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
