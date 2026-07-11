# Artifact 与演示文稿工具契约

## 1. 权威范围

本文唯一拥有 typed Artifact producer 的输入校验、演示文稿 semantic schema、嵌入式生成/编辑引擎、技术与视觉验证门、原子发布和失败结果合同。

本文不拥有：

- Python、LibreOffice、Poppler 的安装、active revision 与 worker 生命周期；见 [03 本地主运行时](../03_运行时与本地服务/本地主运行时准备链路.md) 和 [03 Office 后台渲染](../03_运行时与本地服务/Office后台渲染.md)。
- Artifact 的 journal/wire/Desktop 交付物投影；分别见 [03 SemanticJournal](../03_运行时与本地服务/SemanticJournal与重放.md)、[09 事件字段](../09_AGUI与A2UI协议/事件与字段契约.md) 和 [01 最终交付物](../01_桌面工作台/中间交互区域/最终交付物展示与安全打开.md)。
- Skill 包是否允许绕过平台 typed Artifact；见 [08 安装与激活契约](../08_技能/安装与激活契约.md)。

## 2. 当前能力与入口

[`src/tools/artifact_runtime`](../../src/tools/artifact_runtime) 提供 workspace dependency/runtime preflight、artifact read/inspect/export/render、spreadsheet/document/presentation create/edit/render、Office export PDF、PDF extract、image inspect/convert 和 media probe 等 typed 工具。注册真相是 [`src/tools/tools.go`](../../src/tools/tools.go) 与各工具 `Definition()`；文档不以固定工具总数代替注册核对。

本文件详细固定复杂度最高的三条演示文稿入口：

| 工具 | 当前用途 | 必要输入 |
| --- | --- | --- |
| `presentation_create` | 从 semantic deck/slide spec 创建 PPTX，并完成验证后发布 | `output_path`；`title/theme/slides/render_preview` 可选 |
| `presentation_edit` | 对既有 PPTX 按顺序做增量编辑，并在完整验证后发布 | `source_path`、非空 `edits`；`output_path/theme/render_preview` 可选 |
| `presentation_render` | 为既有 PPTX 生成并验证 PDF/PNG preview | `path`；`output_dir` 可选 |

当前不支持把页面尺寸、原生 PowerPoint 表格、原生图表、任意 XML/helper 源码或未声明字段作为 semantic slide spec。模型需要这些能力时必须得到结构化不支持/校验结果，不能让未识别字段被静默丢弃。

`artifact_render` 对 PPT/PPTX 复用 presentation 验证链，对 PDF 只走 Poppler PNG。但该通用工具的 Definition 当前没有声明 presentation tools 同等的 runtime requirements，因此 permission/preflight 可能不提前返回 `needs_runtime`，PDF 分支缺依赖时还可能落成普通 `tool_execution_failed`。这是当前错误分类缺口。

## 3. 递归 schema 与执行前校验

[`src/tool/schema.go`](../../src/tool/schema.go) 的 `ToolParametersSchema` / `ToolParam` 是模型侧 definition 与执行前 validator 共用的 schema 结构。当前支持：

```text
object properties / required / additionalProperties
array items / minItems / maxItems
string enum / minLength / maxLength
number|integer minimum / maximum
oneOf
```

内置 definition 默认 `additionalProperties=false`。RequestBuilder 发送给模型的 schema 与 ToolRuntime 执行前校验来自同一 resolved definition；外部输入仍必须在 runtime 再校验，不能信任模型已遵守 schema。

错误必须携带从 `$` 开始的精确 JSON path、expected 与 actual 语义，例如：

```text
$.slides[2].images[0].width expected number, got string
```

Go schema 只验证可表达的结构；布局字段组合、颜色、图片可解码性和编辑语义还会由嵌入式 presentation engine 二次验证。两层任一失败都必须形成结构化 validation failure，且在生成/发布副作用前停止。

## 4. `presentation_create` semantic schema

### 4.1 Deck 与主题

顶层允许：`output_path`、`title`、`theme`、`slides`、`render_preview`。`theme` 只允许：

```text
background, surface, primary, accent, text, muted, font_latin, font_ea
```

颜色必须是 3 或 6 位十六进制值。`title` 在没有 slides 时成为默认标题页；有 slides 时还写入 PPTX 文档标题 metadata。

### 4.2 Slide 公共字段

每页只允许：

```text
title, subtitle, body, notes, background, accent, layout,
bullets, images, columns, metrics, timeline
```

`title`、`subtitle`、`body` 是所有布局的公共文本；`notes` 写入 speaker notes。当前 layout enum 固定为九类：

```text
title | section | bullets | two_column | image_right |
comparison | metrics | timeline | closing
```

layout 省略时按内容推断：`metrics` → `timeline` → `columns`（3 项及以上为 comparison，否则 two_column）→ `images` → `bullets` → 只有 subtitle 时 section；否则回到 title/bullets 默认。输出页面固定为 `13.333 × 7.5` 英寸 16:9；当前不提供自定义 page size。

### 4.3 布局与内容组合

| layout | 专用内容 | 当前上限与组合规则 |
| --- | --- | --- |
| `title`、`section` | 无 | 只接受公共标题、副标题与正文 |
| `bullets` | `bullets` | 项目列表 |
| `two_column` | `columns` 或 bullets fallback | columns 最多 2；两种内容源不能同时非空 |
| `image_right` | `images`、`bullets` | 图片在右侧，正文/项目在内容区 |
| `comparison` | `columns` 或 bullets fallback | columns 最多 3；两种内容源不能同时非空 |
| `metrics` | `metrics` 或 bullets fallback | 指标最多 4；两种内容源不能同时非空 |
| `timeline` | `timeline` 或 bullets fallback | 步骤最多 5；两种内容源不能同时非空 |
| `closing` | `bullets` | 结束页公共文本与可选项目 |

`columns[]` 只接受 `title/bullets`；`metrics[]` 只接受 `label/value`；`timeline[]` 只接受 `title/detail/body`。布局不接受的非空字段必须以准确 path 拒绝，不能解析后忽略。

### 4.4 图片

图片至少包含 `path`，可带 `alt/caption/fit/x/y/width/height`。`fit` 只支持 `contain|cover|stretch`；坐标不得为负，宽高必须大于零。`contain/cover` 依赖 Pillow 解码真实图片；文件不存在、为空、不可解码或解析后逃出允许输入边界时失败。`alt` 写入 PPTX 图片替代文本 metadata。

## 5. 版本化引擎与所有权

[`presentation_engine.py`](../../src/tools/artifact_runtime/engines/presentation_engine.py) 由 [`presentation_engine.go`](../../src/tools/artifact_runtime/presentation_engine.go) 使用 `go:embed` 随 `awaken-core` 发布，调用时才 materialize 到受控临时目录；Python 必须来自 Awaken managed runtime。

模型只提交 semantic spec，不提交 Python、OOXML、ZIP 或字体 helper。引擎集中拥有：

- 九类布局的渲染；
- `set_run_font`、text box、bullets、images、notes；
- 唯一的 Latin/East Asian/complex-script 字体 XML helper；
- `validate_overflow` 与 blank/clipping/overflow warning；
- package 保存与重新打开校验；
- 增量编辑。

CJK helper 使用正确的 `lxml.etree.SubElement` 方式写入 `a:latin/a:ea/a:cs`，中文标题、正文、项目符号和 notes 必须由 OOXML 与真实渲染回归保护。复制第二套 helper 或让模型手写 OOXML 会重新引入字体和结构漂移，禁止作为恢复路径。

## 6. `presentation_edit` 增量语义

`edits` 按数组顺序执行；每个 `slide_index` 都基于前一项完成后的当前页序。允许：

| op | 语义 |
| --- | --- |
| `update` 或省略 op | 更新指定字段；至少提供一个字段，未指定内容和图片保持不变 |
| `replace` | 用完整 `slide` 替换该页 |
| `delete` | 删除当前页序的一页；不得附带无关更新字段 |
| `insert` | 在当前页序插入完整 `slide` |

`update` 当前可靠支持 `title/subtitle/body/bullets/images/notes`。`layout/columns/metrics/timeline/background/accent` 属于结构变更，update 会返回 `unsupported_edit`，必须改用 replace。title/subtitle/body 更新依赖 Awaken semantic shape name；第三方 deck 通常没有这些标记，可能必须 replace。images update 只移除 Awaken 命名的图片/说明，第三方未标记图片可能保留并与新图片并存。

replace 清除可见 shapes 后重画，但当前不会先清空 notes；replace payload 未带 notes 时原 notes 可能残留。“完整 slide”目前只要求非空 object，并不强制 title/layout。insert 允许 index `0..len`，delete/update/replace 只允许 `0..len-1`；删除最后一页会在 package gate 失败并拒绝发布。编辑任何 deck 都把全局 slide size 改为固定 16:9，特殊 template 还依赖预期 blank layout，因此对第三方/custom-size deck 不是无损编辑保证。

`source_path` 始终保持为调用者原 deck；即使 `output_path` 等于 source，编辑也先写同目录 staged PPTX。只有完整验证通过才原子替换。输入、edit 组合或验证失败时不得静默退化成全量重建，也不得改变原 deck。

## 7. 验证等级与 `verified`

当前 `verification_level` 只有：

```text
package -> pdf -> png
```

`verified=true` 需要全部满足：

1. 生成的 PPTX 非空，ZIP 可打开，包含 `[Content_Types].xml`、`ppt/presentation.xml` 和至少一份 slide XML；
2. managed LibreOffice 成功转成 PDF；
3. managed Poppler 成功把全部页转成 PNG；
4. slide/PDF/PNG 数量一致；
5. semantic engine 没有当前启发式 warning：空白页、shape 越出页面约 0.01 英寸以上，或单个 text shape 超过 1200 字符。

`visual` 不是当前 `verification_level` 枚举。`verified=true` 也不证明 PNG 已被人或视觉模型核对，不检测 shape 重叠、文本框内部裁切、字体替换、低对比度、图片/caption/业务内容正确性、PowerPoint/WPS 兼容性或完整可访问性。真实像素质量必须按 03/04 验收用中文样本逐页人工核对。

`render_preview=false` 只是不保留/返回 preview 路径，技术验证仍完整执行。LibreOffice/Poppler 不可用时返回 needs-runtime 或未验证结果；Poppler 失败、数量不一致或视觉 warning 均不得产生 ready/verified 交付物。

## 8. Staging、原子发布与失败恢复

create/edit 先在目标同目录创建 staged PPTX 和 staged preview。验证通过后的发布顺序是：

1. 规范化 staged preview 中的最终路径；
2. 为 preview 写入 Awaken ownership marker 并 fsync；
3. 若保留 preview，先启动可回滚的目录 swap；
4. 备份既有目标并以 rename 原子发布 PPTX；
5. fsync 后提交 preview swap；失败时回滚目标/preview，备份无法恢复则保留并报告路径。

既有 preview 目录只有同时满足 ownership marker、预期 PDF/PNG 名称、全部 regular file、无 symlink/特殊节点/无关文件时才可替换。否则 fail closed，不能清理用户目录来“帮忙恢复”。

生成、编辑、warning、render 或发布任一步失败时：

- 不发布 staged PPTX/preview；
- 原有 deck 和 managed preview 保持不变；
- canonical outcome 保留 validation/runtime/verification 事实；
- 不生成指向不存在路径或旧文件的假 Artifact 卡片。

极少数 preview swap commit 后置失败（例如已发布后删除 backup 或 sync 失败）可能在返回失败时留下新 deck/preview；因此“验证前零发布”成立，但 post-publish commit error 不能被宣称为绝对零 partial side effect，outcome 必须以实际文件状态为准。

## 9. 路径与跨域安全边界

Artifact producer 必须使用 writable/output roots；但当前 `artifact_export` 等部分写入只做 root 包含检查，可直接覆盖，尚未统一使用 typed 文本工具的 sensitive/protected/symlink 强 resolver。这是 [TaskScope 与路径安全](TaskScope与路径安全.md) 中的已知缺口。

演示文稿还有以下当前缺口：

- create/edit 不强制 `.pptx`：无扩展名时会补 `.pptx`，已有任意扩展名则原样写入；例如 `.ppt` 可能实际包含 OOXML/PPTX 字节并被按扩展名投影。edit/render source 也没有扩展名门禁。
- `awaken-ppt-assets-*`、`awaken-ppt-edit-*` 临时目录当前未清理，`fit=cover` 的裁剪 JPEG 可能残留系统 temp。
- Python 进程继承完整 `os.Environ()`，没有 OS sandbox；模型不能注入脚本并不等于进程环境最小化。
- slides、字符串、图片数量/总大小和 source PPTX 没有整体资源上限；超时不能完全阻止内存/磁盘耗尽。
- Go 图片前检只要求 regular/non-empty；`contain/cover` 到 Pillow 才验证解码，`stretch` 到 python-pptx 插图时才失败。
- Python 的 `unsupported_edit/unsupported_field` code 进入 Go 后折叠为通用 validation error，只保留 path/reason。

真实验收只能使用隔离 workspace/output root，不得以 `.git`、`.env`、凭据目录或用户真实交付物测试覆盖行为。Shell 不得直接启动 Office，Skill 也不得用 pip/npm、动态 `generate.py`、直接 soffice/pdftoppm 或手写 Office ZIP/XML 绕过本契约。

## 10. 代码 owner 与回归门

| 责任 | Owner / 测试 |
| --- | --- |
| 递归 schema/validator | `src/tool/schema.go`、`schema_test.go` |
| presentation schema | `src/tools/artifact_runtime/presentation_schema.go` |
| Go producer、staging、verification、publish | `src/tools/artifact_runtime/artifact_runtime.go`、`artifact_runtime_test.go` |
| embedded engine 与 CJK/layout/edit | `src/tools/artifact_runtime/presentation_engine.go`、`engines/presentation_engine.py`、`presentation_engine_test.go` |
| managed Office worker | `src/runtime/officerender/` |
| Skill platform conflict | `src/skills/legacy_conflict.go`、`schema_v2_test.go`、`src/runtime/skill_activation_test.go` |

最低自动化：

```bash
go test ./src/tool/... ./src/tools/artifact_runtime ./src/runtime/officerender ./src/skills ./src/runtime
go vet ./src/tool/... ./src/tools/artifact_runtime ./src/runtime/officerender ./src/skills ./src/runtime
```

真实 Desktop 使用 typed presentation tool 创建至少五页中文 PPTX，覆盖九类布局中的适用组合、图片、notes 与一次增量 edit；逐页核对 PDF/PNG、字体、溢出和内容无损，重启后再核对 Artifact identity/verified/preview 投影。不得以“文件存在”或旧截图代替。

当前自动化尚未覆盖错误扩展名、sensitive/protected output、路径检查后的 ancestor/symlink 替换、Python temp 清理、任意第三方/custom-size deck、replace notes 残留和 post-publish commit failure；涉及这些边界的代码变更必须先补测试。
