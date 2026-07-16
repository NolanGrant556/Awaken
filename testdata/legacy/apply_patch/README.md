# legacy apply_patch 事故 fixtures

这些去敏样本只用于兼容性与迁移测试；`apply_patch` 已不是 Awaken 的活跃模型工具。

| 文件 | 历史事故类别 |
|---|---|
| `missing_begin_patch.txt` | 缺 Begin Patch |
| `hunk_line_missing_prefix.txt` | hunk 行缺前缀 |
| `end_of_file_misuse.txt` | End of File 误用 |
| `context_mismatch.patch.txt` | context mismatch |
| `duplicate_patch_a.txt` / `duplicate_patch_b.txt` | exact duplicate payload |

目标文件路径使用相对占位符 `notes/sample.txt`。
