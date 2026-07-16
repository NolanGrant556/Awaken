# Typed 文件工具与可恢复删除

## 1. 当前支持

模型基础文件工具固定为 `Read`、`Edit`、`Write`、`Glob`、`Grep`。它们与 `Bash` 分开：路径统一由 `PermissionContext` 的 readable/writable roots、附件授权、敏感路径和 symlink 检查约束；`Bash` 不继承这些保证。

| 工具 | 当前合同 |
| --- | --- |
| `Read` | 文本从 1 开始分页，默认/上限 2000 行；PNG/JPEG/GIF/WebP 进入模型媒体；PDF 默认 1-5 页、单次最多 20 页，按模型能力走原生 PDF 或 Poppler 144 DPI；Office/Notebook 返回 `unsupported_media` |
| `Edit` | 既有文本必须在当前 thread 成功 `Read` 且宿主快照未变化；默认要求 `old_string` 唯一匹配，只有 `replace_all=true` 可替换多处 |
| `Write` | 新文件无需预读并写 UTF-8；覆盖既有文件必须预读且未外部变化；保留可识别的编码/BOM、换行和权限 |
| `Glob` | 使用 run snapshot 固定的真实 `rg`，默认 hidden/no-ignore，稳定排序，最多返回 100 条并报告 total/truncated |
| `Grep` | 使用真实 `rg` argv，支持 glob/type/output mode/context/line/ignore-case/head/offset/multiline，保留真实无匹配和截断事实 |

缺少 `rg` 时 `Glob`/`Grep` 返回 `needs_runtime`，不回退到语义较弱的 Go 搜索。`Edit`/`Write` 使用同目录临时文件、fsync 和原子替换；未知/二进制编码拒绝覆盖。失败不刷新 thread 文件状态，成功会记录新的 SHA-256/size/mtime/type 快照。

删除统一使用 `trash_file`，支持单路径或明确路径列表。成功结果返回 `original_path`、`method`，并在平台可观察时返回 `trashed_path`，同时写 observed `trashed` change；批量中途失败标为 partial/unknown，要求先检查原路径和 Trash，禁止自动改用永久删除。

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
