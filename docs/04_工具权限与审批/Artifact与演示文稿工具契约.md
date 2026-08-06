# Artifact 生成、发布与历史回放契约

## 1. 当前支持边界

Awaken 当前不提供内建 typed Office/PDF 生成或渲染 runtime。`artifact_runtime`、`read_artifact`、`runtime/officerender` 及 document/spreadsheet/presentation/PDF typed helper 不在注册表中，也不能写成当前能力。Agent Plan 的 `image_generate` / `video_generate` 是独立媒体工具，不属于本文档发布链路。

当前支持的是文档发布链路：

1. Agent 使用 `Bash`、`Write`、已激活 Skill 或 MCP provider 生成并自行验证文件；
2. 文件可位于本机任意路径；`output_roots` 是生成交付物的默认位置，不是发布或打开边界；
3. Agent 调用 `artifact_publish` 显式发布；
4. canonical `ArtifactDescriptor` 进入 SemanticJournal、AG-UI 和 Desktop 终态交付物列表。

`Write`/`Edit` 只产生文件 change/diff，永不自动生成 Artifact。`artifact_publish` 只读取既有普通文件，不修改内容；支持 `.md`、`.doc/.docx`、`.xls/.xlsx`、`.csv/.tsv`、`.pdf`、`.ppt/.pptx`，并校验 symlink、regular file、扩展名与已支持的基础格式签名。发布成功的 ready deliverable 以 durable Artifact identity 作为 Desktop open/reveal/媒体预览依据，不因所在目录被再次拒绝。

`artifact_publish` 明确不支持图片和视频。`image_generate` / `video_generate` 成功时已由工具 Outcome 直接产生 `role=deliverable + status=ready + kind=image|video` 的 Artifact，并进入 Workbench 专用媒体卡与预览；不得再调用 `artifact_publish` 二次发布。媒体工具未注册、调用失败，或图片/视频仅由 Bash、Skill、外部程序写入磁盘时，当前没有通用手动媒体发布入口，Agent 必须如实报告，不能把文件存在写成“媒体交付成功”。

图片组的 Artifact 使用 `group_id + group_index` 保持组内位置。部分失败后只允许以原结果的 `retry_group_id + retry_indices` 补做未 ready 的位置，不能替换已 ready 项，也不能把补图作为新的组追加。用户按界面序号引用既有媒体时，模型必须使用 Core 注入清单中的 `artifact_id`；`video_generate.source_artifact_id` 由 ToolRuntime 对当前 thread 的 ready 图片 Artifact 做精确解析。任意自造 ID、非 deliverable、非 ready、类型不符或已不存在的文件都 fail closed；`source_image_path` 只保留给本次 run 内新产物等尚无历史卡片 identity 的显式路径链路。

## 2. MCP Artifact bridge

MCP per-tool `io.awaken.tool/provider/v1.artifact_output` 可声明有界 base64 bytes、lowercase SHA-256、workspace-relative output path、content type 和 title。声明无效只停用 bridge，不阻断普通 MCP 调用。有效结果必须通过：

- 32 MiB host 上限、base64、digest、扩展名/MIME和基础容器签名；
- TaskScope writable/output roots、敏感/protected/symlink 检查；
- `WorkspaceExecutionBroker` private snapshot 的 exact single-file delta；
- replacement preimage Trash、并发 workspace recheck 和原子 publish。

该链只证明 bytes/hash、基础容器和 host-owned recoverable commit，不证明业务语义、版式、字体、公式或 Office 兼容性。MCP 原始 bytes 不进入模型、Journal、AG-UI 或日志。

## 3. 历史兼容

SemanticJournal 中旧 `presentation_create`、`document_create`、`spreadsheet_create`、`office_export_pdf`、`apply_patch` 等 producer 继续按通用 legacy Artifact/tool card 回放。Renderer 保留旧 producer 到文件类型/角色的兼容映射，但不得把它们重新注册为当前工具、重写用户历史或将旧 `verified` 事实外推到新文件。

同一路径 Artifact 的最终 deliverable 折叠、排序和安全打开由 01 域 [最终交付物展示与安全打开](../01_桌面工作台/中间交互区域/最终交付物展示与安全打开.md) 拥有。

## 4. Skill 与 Bash 边界

Skill 是生成工作流与依赖说明，不是 Artifact 权限 owner。模型应按 Skill 完整指令，通过 `Bash`/`Write` 生成 Word、Excel、PPT、PDF、Markdown，再调用 `artifact_publish`。raw Bash 成功只证明进程事实；发布前仍应使用实际生成工具或 Skill 提供的检查命令验证文件。

缺少 LibreOffice、Poppler、Python/Node 包或其他生成依赖时，当前不会由 typed artifact runtime 自动恢复。Agent 根据真实 Bash 错误使用包管理器或改用可用 Skill/provider；仍缺依赖时必须报告阻塞。

## 5. 当前不支持

- 内建 semantic deck/document/spreadsheet create/edit/render；
- 内建 Office -> PDF、PDF extract、图片转换、媒体 probe；
- 内建逐页 preview/视觉质量验证；
- 从文件存在或扩展名推断业务质量；
- 通过 `artifact_publish` 发布图片、视频，或手动把任意本地媒体文件提升为专用媒体交付物；
- Renderer 右侧 Inspector 的 Artifact/tool 详情。

## 6. 代码 owner 与验收

- `src/tools/artifact_publish`：显式 deliverable 发布。
- `src/tools/media_generate`：专用图片/视频生成与 ready 媒体 Artifact。
- `src/runtime/media_artifact_references.go`、`src/tool/permission.go`：界面媒体序号到稳定 Artifact identity 的宿主投影与精确路径复验。
- `src/tool/artifact_contract.go`：Artifact v2 与历史 producer 兼容。
- `src/mcpclient/artifact_bridge.go`、`provider_manifest.go`：MCP host bridge。
- `src/tool/execution_broker*.go`：dependency/git/MCP 的 staging、Trash 与原子 commit。
- `src/runtime`、`src/protocol/agui`、Desktop reducer/open：持久化、wire、卡片和安全打开。

自动化必须断言 retired runtime/目录/注册名不存在，同时保留 MCP bridge、旧 producer replay 和 Artifact 卡片回归。真实 Desktop 需用 Bash/Write/Skill 生成 Word、Excel、PPT、PDF、Markdown，逐个 `artifact_publish`，确认终态卡片可点击且重启后仍可回放；未完成真实视觉检查时不能声称文档版式已通过。
