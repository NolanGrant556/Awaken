# Word 输出质量检查

## 三层 QA（必须）

### 1. 内容 QA

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/read_docx.py" output.docx --format json --output output.check.json
```

用 `Read` 检查 `output.check.json` 的 `markdown`、表格 `blocks` 与 `warnings`，核对缺段、错序、占位符、乱码、错误链接、空白项目符号和用户要求的关键字段。

### 2. 结构 QA

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/validate_docx.py" output.docx --json
```

模板改稿必须增加 `--original 原文件.docx`。脚本检查 ZIP 路径、必需部件、XML、关系目标、批注引用和修订元数据；它不把“结构可打开”冒充“内容正确”。

### 3. 视觉 QA

每轮使用一个新的预览目录：

```text
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/render_docx.py" output.docx ./preview-word-1 --json > ./preview-word-1.result.json
```

先用 `Read` 打开完整结果 JSON；只有 Bash 成功且 `status=success` 才能继续。对 `images` 返回的每张页图调用 `Read`，逐页检查：文字溢出、分页孤行、表格跨页、中文字体回退、页眉页脚碰撞、图片变形、目录与页码、异常空白。保留实际检查的页码清单；发现问题后修复，使用新的预览目录重新生成并复核受影响页。未读取的页不能计入“逐页视觉通过”。

## 发布前检查

- [ ] 用户要求的正文、表格、图片、修订或批注都已完成
- [ ] 没有 lorem、TODO、xxxx、待填等占位内容
- [ ] 结构校验通过
- [ ] 已逐页视觉检查；无法视觉检查时明确说明，不能伪称通过
- [ ] 最终文件位于工作区 output root
- [ ] 已调用 `artifact_publish`，Workbench 出现可点击交付物卡片
