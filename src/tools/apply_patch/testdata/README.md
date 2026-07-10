# apply_patch 事故 fixtures

去敏后的最小结构样本，用于锁定真实会话中出现的 patch 失败类别。

| 文件 | 事故类别 | 预期当前行为 |
|---|---|---|
| `missing_begin_patch.txt` | 缺 Begin Patch | parser 语法失败 |
| `hunk_line_missing_prefix.txt` | hunk 行缺前缀 | parser 语法失败 |
| `end_of_file_misuse.txt` | End of File 误用 | parser 语法失败 |
| `context_mismatch.patch.txt` | context mismatch | 执行期找不到期望行 |
| `duplicate_patch_a.txt` / `duplicate_patch_b.txt` | 两组 exact duplicate payload | 内容完全相同，可被重复提交 |

目标文件路径使用相对占位符 `notes/sample.txt`；测试在临时目录展开。
