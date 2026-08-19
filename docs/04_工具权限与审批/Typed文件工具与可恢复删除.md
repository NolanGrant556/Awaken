# Typed 文件工具与可恢复删除

## 1. 当前支持

模型基础文件工具固定为 `Read`、`Edit`、`Write`、`Glob`、`Grep`。它们与 `Bash` 分开：路径统一由 `PermissionContext` 解析，roots 提供默认作业位置和搜索范围，附件映射、敏感路径和 symlink 检查仍由宿主强制；用户明确点名的外部绝对路径不因越出 roots 被拒绝。`Bash` 不继承 typed 文件工具的快照、原子写入和结构化结果保证。

| 工具 | 当前合同 |
| --- | --- |
| `Read` | 文本从 1 开始分页，默认/上限 2000 行；PNG/JPEG/GIF/WebP 进入模型媒体；PDF 在 `pdfinfo` 可知且 ≤10 页时省略 `pages` 会整份读取（原生整份或 Poppler 渲全部页），>10 页的渲染页路线省略 `pages` 时由宿主自动归一化为首段 `1-10` 并在成功结果标记 `pages_defaulted=true`，不先返回分页参数异常；`route=native_pdf` 代表原生整份已完整交给模型。渲染页单次最多 20 页，越界页在已知总页数时夹紧忽略。模型从首次成功结果的 `route/page_count` 决定是否读取剩余范围。原生 PDF 或视觉（Poppler 144 DPI）任一可用时阅读，两者皆无返回 `model_pdf_unsupported` 并要求请用户换模型，不得以 Skill/Bash/装包绕过；Office/Notebook 返回 `unsupported_media` |
| `Edit` | 既有文本必须在当前 thread 成功 `Read` 且宿主快照未变化；默认要求 `old_string` 唯一匹配，只有 `replace_all=true` 可替换多处 |
| `Write` | 新文件无需预读并写 UTF-8；覆盖既有文件必须预读且未外部变化；保留可识别的编码/BOM、换行和权限 |
| `Glob` | 使用 run snapshot 固定的真实 `rg`，默认 hidden/no-ignore、稳定排序；默认每页 100 条、`limit` 最大 1000，以 opaque `cursor/next_cursor` 继续枚举并报告 total/truncated |
| `Grep` | 使用真实 `rg` argv，支持 glob/type/output mode/context/line/ignore-case/head/offset/multiline，保留真实无匹配和截断事实；`head_limit/offset` 提供稳定续读 |

缺少 `rg` 时 `Glob`/`Grep` 返回 `needs_runtime`，不回退到语义较弱的 Go 搜索。两者默认按 TaskScope 精确 roots 搜索；共同父目录或项目外路径必须通过 TaskScope 文档定义的 `scope_mode` 明确区分，不得靠退回 `Bash find` 绕过分页。`Edit`/`Write` 使用同目录临时文件、fsync 和原子替换；未知/二进制编码拒绝覆盖。失败不刷新 thread 文件状态，成功会记录新的 SHA-256/size/mtime/type 快照。

删除统一使用 `trash_file`，支持单路径或明确路径列表。canonical schema 和 executor 都要求 `path` 与 `paths` 必须且只能提供一个；为兼容拒绝根级组合关键字的 OpenAI Chat/Responses 网关，wire schema 会把两项显示为带互斥说明的可选属性，但 ToolRuntime 仍按 canonical `oneOf` 在权限与执行前拒绝缺失或同时提供。成功结果返回 `original_path`、`method`，并在平台可观察时返回 `trashed_path`，同时写 observed `trashed` change；批量中途失败标为 partial/unknown，要求先检查原路径和 Trash，禁止自动改用永久删除。

上述是 typed 文件工具的可恢复合同。默认 raw Shell 不再将 `rm`/`rmdir`/`unlink` 等命令转换为 `trash_file`，它按当前登录用户的 Terminal 语义直接执行；UI/Outcome 不得伪造可恢复 change。

## 2. 平台实现

| 平台 | 当前实现 | 恢复语义 |
| --- | --- | --- |
| macOS | `rename` 到 `~/.Trash` | 同卷移动；返回实际路径；不是 NSWorkspace 完整 Finder 集成 |
| Windows | PowerShell 调用 Microsoft.VisualBasic FileIO | 发送到 Explorer Recycle Bin；目标仅经环境变量传入，失败则 fail closed；当前不返回可见 trashed path |
| Linux/其他 | fail closed | 不永久删除 |

macOS/Windows 都拒绝 symlink 和特殊文件。macOS 处理同名碰撞，跨卷 rename 返回 typed failure，不能 copy-then-delete；Windows 调用后复查原路径确已消失，否则判失败。恢复仍依赖系统回收站状态，Awaken 当前没有通用“还原”工具。

## 3. 文件状态与媒体

- 部分文本读取也建立完整宿主文件快照，因此可用于后续 `Edit`/`Write` stale check。
- 同一路径的 mutation 由 thread state 串行；compare-and-write 防止外部修改或并发覆盖。
- 再次读取未变化文件返回 `file_unchanged`，仍刷新当前读取状态。
- 媒体 Base64 只存在于运行内存；持久化只保存 `storage_key`，由 03 域定义水合与缺失恢复。
- `Edit`/`Write` 产生 canonical change/diff，但永不自动发布 Artifact；最终交付文件必须显式调用 `artifact_publish`。

## 4. 当前缺口

- Windows 当前通过 PowerShell/FileIO 接入 Explorer Recycle Bin，但仍缺 `IFileOperation` 原生 adapter、实际回收站路径和 Windows 实机验收；
- macOS 尚未接入 NSWorkspace，跨卷 Trash 不支持；
- Linux 没有 freedesktop Trash adapter；
- 没有跨平台通用 restore tool 和本轮 Windows/Linux 实机证据。

## 5. 代码 owner

- `src/tools/read`、`edit`、`write`、`glob`、`grep`：五个模型基础文件工具。
- `src/tools/internal/textfile`、`filemutation`、`rgexec`：编码/原子写入/固定 rg 复用层。
- `src/tool/session_state.go`、`model_content.go`：thread 文件状态与模型媒体合同。
- `src/tools/trash_file`、`src/tool/trash`：可恢复删除入口和平台 adapter。

自动化必须覆盖外部修改、部分读取、重复匹配、`replace_all`、BOM/CRLF/权限、同路径并发、readable/writable/symlink 边界、`rg` 缺失/无匹配与跨平台 test binary 编译。
