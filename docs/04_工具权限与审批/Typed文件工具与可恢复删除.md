# Typed 文件工具与可恢复删除

## 权威范围

本文唯一定义本地只读工具、typed 文本修改、raw patch 和可恢复删除的当前合同。Artifact producer 的 schema、演示文稿引擎、验证和发布由 [Artifact 与演示文稿工具契约](Artifact与演示文稿工具契约.md) 唯一拥有；路径授权由 [TaskScope 与路径安全](TaskScope与路径安全.md) 拥有。

## 工具分层

| 能力 | 当前工具 | 主要边界 |
| --- | --- | --- |
| 文本读取 | `read_file` | 文本、大小/行数限制、digest；当前不受 readable roots 硬限制 |
| 枚举/搜索 | `list_files`、`grep_search` | 本地只读；结果有界 |
| 精确文本替换 | `text_edit` | writable roots、唯一匹配/replace-all、expected digest、原子替换 |
| 新建/覆盖/追加文本 | `text_file_create` | writable roots、UTF-8、大小；发布另受 output roots |
| 复杂 patch | `apply_patch` | 默认隐藏；多文件/移动/删除；非全局事务 |
| 可恢复删除 | `trash_file` | 项目内外、文件/目录、平台 trash adapter；当前缺少 TaskScope/敏感路径硬门禁 |
| artifact | artifact/office/spreadsheet/document/presentation/pdf/image/media typed tools | 见 [Artifact 与演示文稿工具契约](Artifact与演示文稿工具契约.md) |

历史名 `file_read` 不是当前模型合同；当前名称是 `read_file`。

## 本地只读工具

[`src/tools/file_read/file_read.go`](../../src/tools/file_read/file_read.go) 读取 UTF-8/文本文件，默认最大 256 KiB、上限 1 MiB、最多 5,000 行，并返回 size、mtime、SHA-256 等事实；二进制输入拒绝或路由到对应 artifact/image 工具。

[`src/tools/list_files`](../../src/tools/list_files) 和 [`src/tools/grep_search`](../../src/tools/grep_search) 提供文件枚举与内容搜索，输出必须限制数量/长度并保留“截断”提示。

三者当前可解析 OS 允许的绝对路径，不受 `ReadableRoots` 硬限制；这是现状，不得写成 workspace-only。attached file 可按路径、display name/basename 解析，同名时必须报歧义。

## `text_edit`

[`src/tools/text_edit`](../../src/tools/text_edit) 适合单文件可验证替换：

- `old_string` 与 `new_string` 必须不同；
- 默认要求唯一匹配，`replace_all` 才允许全量替换；
- `expected_sha256` 可防止读取后文件已变化；
- 通过同目录临时文件、fsync、rename 原子替换并保留原 mode；
- outcome 包含 before/after digest、diff 和 observed change。

匹配歧义、digest mismatch、symlink escape 或敏感路径时不写入，并提供下一步诊断。

## `text_file_create`

[`src/tools/text_file_create`](../../src/tools/text_file_create) 支持创建、覆盖和追加 UTF-8 文本，当前最大 2 MiB。创建/覆盖要求 writable root；显式发布最终文本交付物还要求 output root，当前发布类型限 Markdown/CSV/TSV 等合同允许类型。

当前实现的非 append 分支使用 `os.WriteFile`，append 使用 `OpenFile`；它没有 `text_edit` 的临时文件原子替换保证。需要 crash-safe 覆盖时应改用具备原子合同的实现，不能从 typed 名称推断已原子化。

## `apply_patch`

[`src/tools/apply_patch`](../../src/tools/apply_patch) 支持多文件 add/update/move/delete 和上下文匹配。普通 provider 默认隐藏，仅在 compat/grammar/all 等明确 exposure 下使用。

每个 mutation 均检查 writable root、sensitive path 和 symlink identity；文件删除走 trash，不允许永久删除目录。多 operation 不是全局事务：前序 operation 成功、后序失败时 outcome 必须列出 observed partial side effects，不能伪造整体回滚。

## `trash_file`

[`src/tools/trash_file/trash_file.go`](../../src/tools/trash_file/trash_file.go) 可处理文件或目录，并有意允许项目内外路径。它不受 writable roots 约束并使用 [`src/tool/trash`](../../src/tool/trash) adapter；失败时不做永久删除 fallback，无法确认的 side effect 保持 unknown。

当前实现只用 `ResolvePath`/`filepath.Abs` 解析目标。intent extractor 不提取 `trash_file.path`，FileChainExecutor 因此也不检查 writable root、sensitive/protected path 或 symlink-resolved identity。也就是说，可恢复移动已实现，但其他 typed mutation 的路径防线尚未覆盖 `trash_file`。这是安全缺口，不得以“会进 trash”掩盖；在代码修复前，真实验收只能使用隔离测试路径，不能尝试用户凭据、`.git`、`.awaken` 或其他敏感目录。

- macOS adapter 通过 rename 移入 `~/.Trash`。
- Windows 当前通过 rename 移入 `%USERPROFILE%/.trash`，还不是 Explorer Recycle Bin 集成。
- 其他未实现平台 fail closed。

因此产品可承诺“可恢复删除/移入 Awaken 支持的 trash 位置”，不能笼统声称所有平台都进入系统原生回收站。

## Artifact 继续读档

Artifact 的 producer/schema/publish 进入 [Artifact 与演示文稿工具契约](Artifact与演示文稿工具契约.md)，运行时进入 [03 主运行时](../03_运行时与本地服务/本地主运行时准备链路.md) 与 [03 Office 后台渲染](../03_运行时与本地服务/Office后台渲染.md)，耐久性进入 [03 SemanticJournal](../03_运行时与本地服务/SemanticJournal与重放.md)，wire 和 Desktop 打开分别进入 09 与 01。本文不重复这些事实。

## 异常、恢复与不变量

- `text_edit`、`text_file_create`、`apply_patch` 在执行器内再次检查强路径边界；`trash_file` 和部分artifact写入当前缺少同等检查，必须作为缺口保留。
- expected digest 冲突要求重新读取，不自动覆盖用户并发修改。
- 删除只走可恢复 adapter，失败不降级永久删除。
- partial side effects 必须逐项记录 observed/inferred/unknown。
- output root 是发布合同，不能通过普通 writable root 绕过。
- raw patch 不应取代可表达同一意图的 `text_edit`/`text_file_create`。

## 测试与真实 Desktop 验收

自动化 owner：各工具目录 `*_test.go`，重点包括 [`src/tools/text_edit/text_edit_test.go`](../../src/tools/text_edit/text_edit_test.go)、[`src/tools/text_file_create/text_file_create_test.go`](../../src/tools/text_file_create/text_file_create_test.go)、[`src/tools/apply_patch/apply_patch_test.go`](../../src/tools/apply_patch/apply_patch_test.go)、[`src/tools/trash_file/trash_file_test.go`](../../src/tools/trash_file/trash_file_test.go)、[`src/tool/trash/trash_test.go`](../../src/tool/trash/trash_test.go)、artifact contract/runtime 测试。

真实 Desktop 使用隔离 workspace 覆盖读取、精确编辑、并发 digest 冲突、新建/追加、patch partial failure、项目内外 trash 和最终 artifact。确认 UI 展示真实 diff/status、trash 通知与交付物；不得删除用户真实文件，也不得用 Shell 模拟 typed 路径后宣称通过。
