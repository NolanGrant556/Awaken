# Typed 文件工具与可恢复删除

## 1. 当前支持

文件读、列举、搜索、创建、结构化编辑、patch、artifact 读取与删除由 typed tools 提供。每个工具声明输入/输出 schema、capability、side effects 和失败模式，并在 TaskScope 内执行。

删除统一使用 `trash_file`，支持单路径或明确路径列表。成功结果返回 `original_path`、`method`，并在平台可观察时返回 `trashed_path`，同时写 observed `trashed` change；批量中途失败标为 partial/unknown，要求先检查原路径和 Trash，禁止自动改用永久删除。

上述是 typed 文件工具的可恢复合同。默认 raw Shell 不再将 `rm`/`rmdir`/`unlink` 等命令转换为 `trash_file`，它按当前登录用户的 Terminal 语义直接执行；UI/Outcome 不得伪造可恢复 change。

## 2. 平台实现

| 平台 | 当前实现 | 恢复语义 |
| --- | --- | --- |
| macOS | `rename` 到 `~/.Trash` | 同卷移动；返回实际路径；不是 NSWorkspace 完整 Finder 集成 |
| Windows | PowerShell 调用 Microsoft.VisualBasic FileIO | 发送到 Explorer Recycle Bin；目标仅经环境变量传入，失败则 fail closed；当前不返回可见 trashed path |
| Linux/其他 | fail closed | 不永久删除 |

macOS/Windows 都拒绝 symlink 和特殊文件。macOS 处理同名碰撞，跨卷 rename 返回 typed failure，不能 copy-then-delete；Windows 调用后复查原路径确已消失，否则判失败。恢复仍依赖系统回收站状态，Awaken 当前没有通用“还原”工具。

## 3. Typed 写入

- 新建/覆盖只写 TaskScope writable roots；
- patch 必须先解析并展示/记录明确变更，删除文件时转入 Trash；
- 输出 schema 与实际文件观察不一致时 Outcome 失败；
- 工具失败不生成 artifact/change；side effect 不确定时标 unknown。

## 4. 当前缺口

- Windows 当前通过 PowerShell/FileIO 接入 Explorer Recycle Bin，但仍缺 `IFileOperation` 原生 adapter、实际回收站路径和 Windows 实机验收；
- macOS 尚未接入 NSWorkspace，跨卷 Trash 不支持；
- Linux 没有 freedesktop Trash adapter；
- 没有跨平台通用 restore tool 和本轮 Windows/Linux 实机证据。

## 5. 代码 owner

- `src/tools/file_*`、`src/tools/apply_patch`、`src/tools/text_file_create`：typed 文件操作。
- `src/tools/trash_file`、`src/tool/trash`：可恢复删除入口和平台 adapter。
