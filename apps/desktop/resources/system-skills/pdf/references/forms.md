# PDF 表单填写

本路线需要 `pypdf >= 6.10`，仅在加工／生成任务及入口能力边界允许时使用。扫描件没有 AcroForm 字段时，不把本路线当作 OCR。

## 定位与填写

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/extract_form_field_info.py" form.pdf > fields.json
```

结果按真实 widget 页面返回 `{field_id, page, type, current_value?}`；同字段出现在多页时每页一行，`field_id` 保留完整层级名。`page: null` 表示没有可定位的可见 widget，不能猜页码填写。

在副本 values.json 中给目标行添加字符串 `value`，保留 `field_id` 与真实 `page`：

```json
[{"field_id": "customer.name", "page": 2, "value": "张三"}]
```

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/fill_fillable_fields.py" form.pdf values.json filled.pdf
```

填充器拒绝未知字段、错误页码、同字段冲突值及已存在输出；一个字段跨页出现时同步更新全部 widget。写出前回读字段值，成功 JSON 返回实际 fields/pages。此结果证明字段数据已写入，不证明版式正确。

## 复核与交付

用 `Read` 检查成功 JSON 中实际修改的所有 pages，再发布新文件；不只看第一页。复选框／单选组的导出值取决于实际表单（例如 `/Yes` 或 `/On`），不要从显示文字猜测；不支持或无法验证的值会失败。原件保留，最终文件在 output root 并 `artifact_publish`。

字段树与 widget 的对应依据见 [pypdf 官方表单说明](https://pypdf.readthedocs.io/en/stable/user/forms.html)。
