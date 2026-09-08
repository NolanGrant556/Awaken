# PPT 输出质量检查

## 三层 QA（必须）

### 1. 内容 QA

```bash
python3 -m markitdown output.pptx > output.check.md
grep -iE "xxxx|lorem|ipsum|TODO|占位|待填|insert.*(slide|page)" output.check.md || true
```

检查缺页、错序、错别字、数字与来源、演讲备注、残留模板说明，以及隐藏页是否残留不应交付的旧内容。

### 2. 结构 QA

```bash
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/validate_pptx.py" output.pptx --json > ./ppt-structure-result.json
```

模板改稿增加 `--original template.pptx`。随后用 `Read` 打开完整结果 JSON；Bash 非零、`status=failed` 或 `issues` 非空都必须回到生成器修复。校验包括 ZIP/XML、relationship、content type、页面注册、重复 shape ID、图表坐标轴引用和堆积图标签位置。

### 3. 视觉 QA

```bash
"<doctor 返回的 execution_plan.python>" -B "${AWAKEN_SKILL_DIR}/scripts/render_slides.py" output.pptx ./preview-ppt-1 --json > ./preview-ppt-1.result.json
```

先用 `Read` 打开完整结果 JSON；只有 Bash 成功且 `status=success` 才能继续。对 `images` 返回的每一张页图调用 `Read`，保留实际检查的页码清单；未读取的页不能计入“逐页视觉通过”。优先检查：

1. 文字溢出、裁切和过度换行；
2. 元素重叠、线条穿字；
3. 边距、列和卡片间距不一致；
4. 字体替代导致的布局变化；
5. 低对比度、图片变形、图表标签缺失；
6. 页脚/来源与正文碰撞；
7. 残留占位内容或模板装饰错位。

发现问题后修改源脚本或模板变换，再用新目录完整重渲染。不能只修改 PPTX 而复用旧 PDF/图片。

## 发布前检查

- [ ] 内容、顺序、数字、来源和备注符合要求
- [ ] 结构校验通过
- [ ] 每一张可见幻灯片已经视觉检查
- [ ] 发现的问题已修复并重新验证；首轮无问题时据实记录，不人为制造修复轮次
- [ ] 最终文件位于工作区 output root
- [ ] 已调用 `artifact_publish`
