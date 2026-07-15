# Artifact 与演示文稿工具契约

## 1. 当前支持

Awaken 提供 typed artifact/presentation provider，用于创建、编辑、渲染和读取结构化交付物。它声明 `document.create`、`presentation.create/edit/render`、`spreadsheet.create`、`pdf.read/render` 等 capability，并在执行前报告当前用户环境中的具体依赖缺口。

typed provider 的价值是：稳定 schema、TaskScope、可观测 changes/artifacts、渲染验证和结构化失败。模型应在它 ready 且契约匹配时优先使用，但它不是唯一合法实现。Skill 脚本、用户机器工具或 MCP provider 也可以成为候选。默认 Shell 以当前登录用户权限运行脚本/本机 CLI，可访问网络和 workspace 外路径，但只返回进程事实及 unobserved side-effect receipt，不自动获得 typed artifact provider 的 schema、TaskScope、Trash、渲染或 preview 验证。需要这些产物保证时，Agent 再显式调用对应 typed inspect/render/publish provider。

Artifact v2 的用户可见角色按已观察事实决定：typed document/spreadsheet/presentation create/edit、`office_export_pdf`、显式 `artifact_publish`，以及 `apply_patch` / `text_edit` 创建、覆盖或修改的受支持文档声明为 `deliverable`。`apply_patch` / `trash_file` 的删除事实以同路径 `support + deleted` Artifact 覆盖此前交付记录；移动、源码/配置、渲染预览和过程文件保持 `support` / `preview`。因此“隐藏删除卡片”不能通过把整个文件 producer 降为 `support` 实现；同一 run 的多个合格 Artifact 必须保留独立 path/identity，并在终态先按路径折叠最后状态再筛选。

MCP provider 当前可通过 per-tool `io.awaken.tool/provider/v1.artifact_output` 返回有界 base64 bytes、lowercase SHA-256、workspace-relative output path、content type 和可选 title。宿主在 MCP transport 成功后验证 output schema、32 MiB host 上限（provider 可进一步收窄）、hash、扩展名/MIME、Artifact kind、PDF/Office/text 容器签名、TaskScope writable 与 output roots 交集、workspace real path、敏感/protected path 和 symlink；通过后只由 host 把 bytes 写入 `WorkspaceExecutionBroker` private snapshot。broker 只接受该 digest/size 对应的单文件 delta，replacement preimage 先进入 Trash，receipt 再决定 created/updated/published、changes 和 side-effect state。provider-local path 或“已经发布”的文字不能产生 ArtifactDescriptor，失败也不生成伪 artifact。

## 2. Presentation 路径

presentation create/edit/render 必须：

- 在 writable root 内写入；
- 保留稳定 artifact identity 与输出路径；
- 渲染时验证产物存在、格式和页级输出；
- 失败时不把半写文件或预览声明为成功 artifact；
- 将 provider/runtime dependency 和 side-effect state 投影到结果。

Office worker 的 managed LibreOffice 路径、timeout、独立 profile、进程组取消和原子发布由 [03 Office 后台渲染](../03_运行时与本地服务/Office后台渲染.md) 拥有。

### 2.1 当前 typed 入口

`src/tools/artifact_runtime` 当前注册 workspace dependencies/runtime preflight、artifact read/inspect/export/render、spreadsheet/document/presentation create/edit/render、Office export PDF、PDF extract、image inspect/convert 和 media probe 等工具；注册真相始终是 `src/tools/tools.go` 与各工具 `Definition()`，文档不固定工具总数。

复杂入口：

| 工具 | 用途 | 必需输入 |
| --- | --- | --- |
| `presentation_create` | semantic deck/slide spec -> PPTX，完整验证后发布 | `output_path` |
| `presentation_edit` | 对既有 PPTX 顺序执行增量 edits，完整验证后发布 | `source_path`、非空 `edits` |
| `presentation_render` | 既有 PPTX -> 已验证 PDF/PNG preview | `path` |

`artifact_render` 对 PPT/PPTX 复用 presentation 验证，对 PDF 走 Poppler PNG；其 Definition 当前未声明与 presentation tools 相同的 runtime requirements，因此 preflight 可能不先返回 `needs_runtime`，PDF 缺依赖也可能落成普通 execution failure。这是保留的错误分类缺口。

### 2.2 递归 schema 与 semantic deck

模型 schema 与执行前 validator 共用 `ToolParametersSchema/ToolParam`，支持 object properties/required/additionalProperties、array items/min/max、string enum/length、number/integer bounds 和 `oneOf`。内建 definition 默认拒绝未知字段；runtime 仍须二次验证，错误携带从 `$` 开始的 JSON path、expected 和 actual。

presentation 顶层只接受 output/title/theme/slides/render-preview 契约。每页公共字段为 `title/subtitle/body/notes/background/accent/layout/bullets/images/columns/metrics/timeline`；layout 固定为：

```text
title | section | bullets | two_column | image_right |
comparison | metrics | timeline | closing
```

输出固定 16:9 `13.333 × 7.5` 英寸。two-column/comparison 分别最多 2/3 列，metrics 最多 4 项，timeline 最多 5 步；结构化数组与 bullets fallback 不能同时非空。不适用于当前 layout 的字段必须按 path 拒绝，不能解析后忽略。

图片至少含 `path`，fit 只支持 contain/cover/stretch；坐标非负、宽高大于零。文件需在输入边界内且为 regular/non-empty；contain/cover 由 Pillow 解码，alt 写入 PPTX metadata。当前不支持自定义页面尺寸、原生 PowerPoint 表格/图表或任意 OOXML/helper 输入。

### 2.3 嵌入式引擎与编辑语义

`src/tools/artifact_runtime/engines/presentation_engine.py` 由 Go `go:embed` 随 Core 发布，调用时 materialize 到受控临时目录；managed Python 是该 typed provider 的依赖。模型只提交 semantic spec，引擎拥有九类布局、字体/CJK helper、图片、notes、overflow/blank warning、package reopen 与 edits。

`presentation_edit.edits` 按数组顺序作用于当前页序：update/replace/delete/insert。update 可靠支持 title/subtitle/body/bullets/images/notes；layout/columns/metrics/timeline/background/accent 等结构变更必须 replace。第三方 deck 缺少 Awaken semantic shape name 时 update 可能 unsupported；replace 当前可能保留旧 notes，custom-size deck 会被改为固定 16:9，因而不提供第三方 deck 无损保证。

source 始终先写同目录 staged PPTX；只有完整验证后才能替换原目标。失败不能静默退化为全量重建。

### 2.4 Verification、staging 与发布

`verification_level` 只有 package -> pdf -> png。`verified=true` 至少要求 PPTX ZIP/核心 entries/slide、LibreOffice PDF、Poppler 全页 PNG、页数一致，并且没有当前启发式 blank/页面越界/超长 text warning。它不证明 overlap、文本框内部裁切、字体替换、对比度、图片/业务内容、PowerPoint/WPS 兼容或完整可访问性；这些仍需真实逐页视觉验收。

MCP host bridge 的验证级别更低且必须明确区分：它只证明收到的有界 bytes 与 SHA-256 一致、扩展名/MIME/基础容器签名相符，并由 host recoverable commit 到正确 TaskScope。它不等价于 typed artifact runtime 的 schema、渲染、逐页视觉、业务语义或兼容性验证，也不能把 MCP 产物标为 `verified=true`。需要这些质量事实时仍须调用对应 typed inspect/render provider 并完成真实视觉验收。

`render_preview=false` 只是不保留 preview，技术验证仍执行。create/edit 使用同目录 staged PPTX/preview、ownership marker、fsync、可回滚 preview swap 和目标 rename。既有 preview 只有 marker、预期 PDF/PNG、全部 regular file 且无 symlink/无关文件时才能替换；否则 fail closed。验证失败不发布 staged 产物，不生成指向旧/不存在文件的假 Artifact。post-publish commit failure 仍可能形成 partial side effect，Outcome 必须按实际文件状态记录。

## 3. Skill 边界

Skill 是工作流和知识包，不是 artifact runtime 的权限 owner。它可以建议 typed presentation 工具、受控资源读取或其他已就绪 provider；`allowed-tools`/required runtime 只是提示，不能激活一个未进入父 run 快照的工具，也不能强制把 Skill 标为 unavailable。

旧“Skill 永远不得使用脚本、Python、soffice 或 OOXML”的绝对禁令不是当前契约。Agent 可在默认 direct Shell 中使用用户机的本机环境、网络、Skill root 和用户缓存；该路径的 raw 文件/外部副作用保持 unobserved。无论 Shell 成败，如果要将结果标为 verified artifact，仍必须通过对应 typed inspect/render/publish 的独立验证；文件只存在不等于渲染、格式或视觉质量已通过。

## 4. 当前缺口

- provider 候选解析当前主要用于诊断，尚无通用的运行中自动 fallback/迁移；
- 用户机器上的 Office/Python 包可被环境快照观察，但不等于经过 Awaken attestation；
- Windows managed Office runtime、真实渲染和 Desktop 交付物验收尚无本轮实机证据；
- 右侧 Inspector 尚未接入 artifact/tool 详情。
- artifact 部分写入还未全部统一 typed 文本工具的 sensitive/protected/symlink resolver；
- presentation create/edit 不严格要求 `.pptx`，错误扩展名可能承载 PPTX 字节；
- presentation assets/edit 临时目录未全部清理，Python 进程仍继承完整环境且没有独立 OS sandbox；
- slides/字符串/图片/源 deck 缺少统一总资源上限；第三方/custom-size deck、replace notes、ancestor replacement 和 post-publish commit failure 仍缺完整回归。

## 5. 代码 owner

- `src/tools/artifact_runtime` 与 presentation/artifact tools：typed provider。
- `src/runtime/officerender` 与 host environment resolver：复用用户 LibreOffice，同时保留隔离 profile、timeout、验证和原子发布。
- `src/tool/catalog_snapshot.go`：provider readiness 与 capability 候选诊断。
- `src/tool/schema.go`：递归 schema/validator。
- `src/mcpclient/provider_manifest.go`、`tool_safety.go`、`artifact_bridge.go`：MCP artifact output 声明、结果验证和 host bridge。
- `src/tool/execution_broker.go`、`execution_broker_workspace.go`：`host_artifact_publish` exact-delta、concurrent guard 与 Trash commit。

聚焦回归：

```bash
go test ./src/tool/... ./src/mcpclient ./src/tools/artifact_runtime ./src/runtime/officerender
go vet ./src/tool/... ./src/mcpclient ./src/tools/artifact_runtime ./src/runtime/officerender
```

真实 Desktop 至少创建五页中文 PPTX，覆盖适用布局、图片、notes 与一次 edit，逐页核对 PDF/PNG、字体、溢出和内容，重启后再核对 artifact identity/verified/preview。文件存在或旧截图不能替代。
