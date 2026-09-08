# Word 高保真编辑细则

仅在修改已有 DOCX、模板、批注或修订时读取。

## 路线选择

- 只有普通段落、简单表格，且用户接受样式重建：可以使用 `python-docx`。
- 含页眉页脚、文本框、域、目录、批注、修订、嵌入对象、自定义 XML：使用安全 OOXML 路线。
- 只需要接受全部修订：优先 `accept_changes.py`，不要用 Pandoc 往返宣称版式完全保留。

## OOXML 修改原则

- DOCX 是 ZIP 包；解包和打包必须使用 `ooxml_package.py`，拒绝路径逃逸、符号链接和特殊文件。
- 修改前保留原件，输出到新文件。
- Word 经常把一个可见词拆成多个 `<w:r>`；找不到完整短语时先运行 `merge_runs.py`，不要用全局正则重写所有 XML。
- 只替换目标 `<w:t>` 或目标 run，保留 `<w:rPr>`、段落属性和关系 ID。
- 文本有首尾空格时设置 `xml:space="preserve"`。
- 删除文本的修订节点使用 `<w:delText>`，不是 `<w:t>`。
- 修改含批注的文本时保留 `commentRangeStart`、`commentRangeEnd` 和 `commentReference` 的对应 ID。
- 不对 XML pretty-print；无关格式变化会扩大 diff，也可能改变签名或兼容扩展。

## 修订

- 插入和删除必须有唯一 ID、作者和时间。
- 删除整个段落时还要处理段落标记；仅删除所有文字可能留下空项目符号。
- `validate_docx.py --original` 用于识别新引入的包结构问题，不证明所有业务修改都已正确追踪。

## 修改完成后

1. 重新打包；
2. 与原件做结构基线校验；
3. 抽取文本对比；
4. 渲染并逐页检查；
5. 发布新文件，不覆盖用户原件。

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
