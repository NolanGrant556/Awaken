# PDF 填表（Awaken）

## 流程

1. 列出字段：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/extract_form_field_info.py" form.pdf > fields.json
```

2. 编辑 JSON，为每个要填的字段加上 `value`（保留 `field_id` / `page`）。

3. 填充：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/fill_fillable_fields.py" form.pdf values.json filled.pdf
```

4. 用 `pdftoppm` 渲第一页并用 Awaken `Read` 目视确认；必要时再 `artifact_publish`。

## 注意

- 复选框取值因表单而异（常见 `/Yes`、`/On`）；以 `extract_form_field_info` 输出为准。
- 部分 PDF 需要 `NeedAppearances` 才能在阅读器中显示；脚本会尽量设置。
- 扫描件无 AcroForm 字段时，本路径无效，应改 OCR 或注释层方案（见 `reference.md`）。
