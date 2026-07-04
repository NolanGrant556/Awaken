# Plan：修复 Slice 2 workspace 安全 blocker（已按产品设计更新）

## Summary

> 2026-06-18 更新：本计划最初把 `shell` 只读读取 workspace 外文件视为 P0 blocker。产品设计已明确：`shell` 只读命令允许按用户本机权限读取 workspace 外信息；workspace guard 只强制约束 `read_file`、`list_files`、`grep_search` 等专用 workspace 文件工具，以及写入/修改边界。因此，本计划中关于“`shell cat /etc/hosts` 必须失败或进入审批”的旧验收口径不再适用。

Slice 2 当前安全口径：

1. `read_file`、`list_files`、`grep_search` 必须受 workspace 边界约束，workspace 外路径和 symlink 逃逸必须拒绝。
2. `shell` 只读命令允许读取 workspace 外信息；mutation、网络、仓库状态变更、重定向、管道和复杂 shell 操作仍需审批或拒绝。
3. `writable_roots` v1 默认等于 `[workspace_root]`，且写入边界必须保持在 workspace 内。

本计划中 symlink 相关修复仍然有效；shell 只读外部读取相关修复建议仅作为更严格策略的历史记录，不作为当前 Slice 2 验收要求。

---

## Current State Analysis

### 已完成的正确部分

- `PermissionContext` 已提供 `WorkspaceRoot`、`WritableRoots`、`ResolvePath`、`IsWithinWorkspace`。
- `read_file`/`list_files`/`grep_search` 的 `CheckPermission` 已调用 `ResolvePath` + `IsWithinWorkspace` 拒绝绝对路径越界请求。
- Desktop bridge 已从 `createRun(input)` 升级为 `startRun(request)`，并携带 `workspace_root`。
- 真实 LLM smoke 已验证：当明确要求使用 `read_file` 读取 `/etc/hosts` 时会被拒绝。

### 存在的安全缺陷

#### 历史缺陷 1：shell 工具未检查命令参数中的路径（当前设计允许只读外部读取）

位置：

- `src/tools/shell/shell.go:146` `shellPermissionReason`
- `src/tools/shell/shell.go:183` `isSafeReadOnlyShellCommand`
- `src/tools/shell/shell.go:100` `Execute` 设置 `cmd.Dir`

当前逻辑：

```go
if isSafeReadOnlyShellCommand(params.Command) {
    return "", ""  // 直接 Allow
}
```

`isSafeReadOnlyShellCommand` 仅根据命令名（`cat`、`head`、`tail`、`ls`、`grep`、`rg`、`find`、`stat`、`file` 等）判断是否安全，完全不检查命令参数里的路径。真实 LLM smoke 已复现：模型调用 `shell {"command":"cat /etc/hosts"}` 成功返回内容，没有审批、没有拒绝。按当前产品设计，这属于允许的本地只读 shell 能力，不再作为 Slice 2 blocker。

#### 缺陷 2：symlink 逃逸

位置：

- `src/tool/permission.go:83` `IsWithinWorkspace`
- `src/tool/permission.go:123` `resolvePath`
- `src/tools/file_read/file_read.go:95` `readTextRange` 中 `os.Stat`/`os.Open`

当前 `IsWithinWorkspace` 仅比较 clean 后的字符串路径，不调用 `filepath.EvalSymlinks`。若 workspace 内存在 symlink：

```bash
ln -s /etc/hosts /Users/nolangrant/Awaken/evil-link
```

则 `read_file {"path":"evil-link"}` 会：

1. `ResolvePath` 得到 `/Users/nolangrant/Awaken/evil-link`（在 workspace 内）。
2. `IsWithinWorkspace` 通过。
3. `os.Open` 跟随 symlink 打开 `/etc/hosts` 并返回内容。

`list_files`/`grep_search` 同样存在此问题（它们以 root 目录检查通过，但搜索过程中会跟随 symlink 访问外部文件）。

---

## Proposed Changes

### 变更 1：为 shell 安全只读命令增加路径参数审查（历史严格方案，当前不强制）

文件：`src/tools/shell/shell.go`

#### 1.1 新增辅助函数

在 `shell.go` 中新增 `shellCommandPathArgs` 函数，对已知安全只读命令做简单的参数解析，提取出其中的文件/目录路径参数。

支持覆盖的命令与参数位置：

- `cat <file>...`
- `head [-n NUM] <file>...`
- `tail [-n NUM] <file>...`
- `wc [-clmw] <file>...`
- `stat [-L] <file>...`
- `file <file>...`
- `strings <file>...`
- `ls [options] [path]...`
- `tree [options] [path]...`
- `du [options] [path]...`
- `grep [options] <pattern> <file>...`
- `rg [options] <pattern> [path]...`
- `find [path]... [expression]`（仅把 expression 前的参数视为路径）
- `git status/diff/log/show/rev-parse/ls-files/branch --show-current`（无外部路径参数，视为安全）
- `pwd` / `echo` / `printf` / `which` / `whereis` / `whoami` / `date`（无路径参数）

实现要点：

- 仅处理简单命令；包含控制操作符（`&&`、`|`、`>`、`<`、`$()` 等）的已在前面返回 `Ask`。
- 使用 `strings.Fields` 初步分词；路径参数若是 `-` 开头则视为 option 跳过。
- 遇到无法识别的参数模式时，保守地将其当作路径进行检查（或返回 `Ask`）。
- 所有提取出的路径先通过 `permissionContext.ResolvePath` 解析，再检查 `IsWithinWorkspace`。

#### 1.2 修改 `shellPermissionReason`

在确认是安全只读命令后，调用路径提取函数：

```go
if isSafeReadOnlyShellCommand(command) {
    if paths := shellCommandPathArgs(command); len(paths) > 0 {
        for _, p := range paths {
            resolved := permissionContext.ResolvePath(p)
            if !permissionContext.IsWithinWorkspace(resolved) {
                return fmt.Sprintf("shell command would read path outside workspace: %s", p), "warning"
            }
        }
    }
    return "", "" // Allow
}
```

严格模式下，`cat /etc/hosts` 会返回 `AskPermission`，而不是 `AllowPermission`。当前 Slice 2 默认不要求这一点；只读外部读取允许成功。

#### 1.3 workdir 检查保持不变

现有 `workdirOutsideWorkspace` 已能阻止通过 `workdir` 参数逃出 workspace，无需改动。

---

### 变更 2：PermissionContext 增加 symlink 感知检查

文件：`src/tool/permission.go`

#### 2.1 新增 `IsWithinWorkspaceResolved`

在 `PermissionContext` 上新增方法：

```go
// IsWithinWorkspaceResolved resolves symlinks before checking workspace boundary.
// It returns false if the path cannot be resolved, treating unresolvable paths as unsafe.
func (c PermissionContext) IsWithinWorkspaceResolved(path string) bool {
    normalized := c.Normalize()
    resolved := normalized.resolvePath(path)
    realPath, err := filepath.EvalSymlinks(resolved)
    if err != nil {
        return false
    }
    return pathWithinRoot(realPath, normalized.WorkspaceRoot)
}
```

#### 2.2 新增 `ResolvePathChecked`

为工具执行阶段提供“解析 + 越界检查”的统一入口：

```go
// ResolvePathChecked returns the resolved real path if it is within the workspace,
// otherwise returns an error describing the boundary violation (including symlink escapes).
func (c PermissionContext) ResolvePathChecked(path string) (string, error) {
    normalized := c.Normalize()
    resolved := normalized.resolvePath(path)
    realPath, err := filepath.EvalSymlinks(resolved)
    if err != nil {
        return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
    }
    if !pathWithinRoot(realPath, normalized.WorkspaceRoot) {
        return "", fmt.Errorf("path %q resolves to %q which is outside workspace", path, realPath)
    }
    return realPath, nil
}
```

> 注：`ResolvePathChecked` 主要用于读工具；写工具若需允许创建不存在的文件，应使用 `IsWithinWorkspaceResolved` 做前置检查，执行时自行创建文件。

---

### 变更 3：read_file 阻止 symlink 逃逸

文件：`src/tools/file_read/file_read.go`

#### 3.1 `CheckPermission` 使用 symlink 感知检查

```go
func (t *FileReadTool) CheckPermission(...) PermissionDecision {
    params, err := parseParams(input)
    if err != nil { ... }
    if !permissionContext.IsWithinWorkspaceResolved(params.Path) {
        return DenyPermission(fmt.Sprintf("path %s is outside workspace or escapes via symlink", params.Path))
    }
    return AllowPermission()
}
```

#### 3.2 `Execute` 使用真实路径执行

```go
func (t *FileReadTool) Execute(ctx context.Context, input string) (string, error) {
    ...
    permissionContext := tool.PermissionContextFromContext(ctx)
    realPath, err := permissionContext.ResolvePathChecked(params.Path)
    if err != nil {
        return "", err
    }
    params.Path = realPath
    return readTextRange(ctx, params)
}
```

`readTextRange` 内部继续用 `os.Stat`/`os.Open`，但此时 `params.Path` 已是 `EvalSymlinks` 后的真实路径，不会再跟随 symlink 逃出 workspace。

---

### 变更 4：list_files 阻止 symlink 逃逸

文件：`src/tools/list_files/list_files.go`

#### 4.1 `CheckPermission` 使用 `IsWithinWorkspaceResolved`

```go
func (t *ListFilesTool) CheckPermission(...) PermissionDecision {
    params, err := parseParams(input)
    ...
    if !permissionContext.IsWithinWorkspaceResolved(params.Root) {
        return DenyPermission(...)
    }
    return AllowPermission()
}
```

#### 4.2 执行时解析真实 root

在执行阶段，把 `params.Root` 替换为 `ResolvePathChecked` 返回的真实路径。遍历过程中若遇到 symlink，由于 root 已是真实路径，任何通过 symlink 指向 workspace 外的结果都会落在真实文件系统的外部；为彻底防止输出外部路径，遍历逻辑应对每个结果路径再次检查是否仍在 workspace 内，越界则跳过（或返回错误）。

实现方式：

- 在 `listFilesRecursive`（或等效函数）返回结果前，对每个文件路径调用 `permissionContext.IsWithinWorkspaceResolved(path)`，越界则跳过。

---

### 变更 5：grep_search 阻止 symlink 逃逸

文件：`src/tools/grep_search/grep_search.go`

与 `list_files` 类似：

1. `CheckPermission` 改为 `IsWithinWorkspaceResolved(params.Root)`。
2. 执行时把 `params.Root` 解析为真实路径。
3. 搜索过程中对候选文件路径做 `IsWithinWorkspaceResolved` 检查，越界则跳过。

---

### 变更 6：新增/更新测试

#### 6.1 shell 路径审查测试

文件：`src/tools/shell/shell_test.go`

新增测试用例：

- `cat /etc/hosts` → `PermissionAsk`
- `head -1 /etc/passwd` → `PermissionAsk`
- `ls /tmp` → `PermissionAsk`
- `cat README.md`（workspace 内相对路径）→ `PermissionAllow`
- `ls ./src`（workspace 内）→ `PermissionAllow`
- `grep foo /etc/hosts` → `PermissionAsk`
- `cat README.md /etc/hosts` → `PermissionAsk`（任一参数越界）
- `cat file.txt | head` → `PermissionAsk`（含管道，走控制操作符分支）
- `cat README.md > out.txt` → `PermissionAsk`（含重定向）

#### 6.2 symlink 逃逸测试

文件：`src/tool/permission_test.go`

新增测试：

- `IsWithinWorkspaceResolved` 对普通文件返回 true。
- workspace 内 symlink 指向外部文件返回 false。
- workspace 内相对 symlink（`../outside`）返回 false。
- workspace 内 symlink 指向 workspace 内其他文件返回 true。
- 不存在的路径返回 false（保守拒绝）。

文件：`src/tools/file_read/file_read_test.go`

新增测试：

- 在临时 workspace 中创建 `link -> /etc/hosts`，调用 `read_file {"path":"link"}` 应返回权限错误，不能读出 `/etc/hosts` 内容。

文件：`src/tools/list_files/list_files_test.go` 与 `src/tools/grep_search/grep_search_test.go`

各新增 symlink 越界测试：

- symlink 指向 workspace 外目录，作为 `root` 参数时应被拒绝。
- workspace 内目录包含指向外部的 symlink，遍历结果不应包含外部文件。

---

### 变更 7：移除/保留 shell 的说明（历史严格方案，当前不强制）

本计划最初选择**保留 shell 工具并修复其路径审查**（方案 A）。当前产品设计已确认 shell 只读外部读取允许成功，因此无需为 Slice 2 强制实现该严格方案。复杂 shell、mutation、网络和写入仍由审批/拒绝策略处理。

---

## Assumptions & Decisions

1. **shell 策略**：当前采用“只读外部读取允许，mutation/复杂命令审批或拒绝”的分层策略。旧的“解析安全只读命令参数”方案仅作为严格模式参考。
2. **symlink 修复方案**：使用 `filepath.EvalSymlinks` 解析真实路径后再判断 workspace 边界。不存在的路径在读取场景下会被视为越界（保守策略）。
3. **写工具暂不处理**：Slice 2 主要约束 workspace 文件工具与写入边界。`apply_patch` 等写工具的 symlink 问题留到后续切片处理；本计划仅聚焦 `read_file`/`list_files`/`grep_search`。
4. **不修改 registry**：不通过移除 `shell` 工具来修复问题，保持现有工具集稳定。
5. **错误响应格式**：保持现有 `DenyPermission` 文本信息，仅增强内容以说明 symlink/路径越界原因，不强制改为 JSON 错误码（P2 事项另论）。

---

## Verification Steps

### 7.1 确定性测试

```bash
cd /Users/nolangrant/Awaken
go test ./...
```

必须全部通过。

### 7.2 Desktop 构建

```bash
npm run typecheck:desktop
npm run build:desktop
```

### 7.3 真实 LLM smoke

启动 awaken-core：

```bash
go run ./cmd/awaken-core
```

逐个验证：

1. **workspace 内 README 读取**

   ```bash
   curl -s -X POST http://127.0.0.1:8787/v1/runs -H 'Content-Type: application/json' -d '{
     "input":"请读取 README.md 并一句话总结",
     "workspace":{"workspace_root":"/Users/nolangrant/Awaken"},
     "client":{"session_id":"smoke-ok","request_id":"r1","entrypoint":"desktop"}
   }'
   ```

   期望：run 成功结束，模型读到 README 内容。

2. **`read_file /etc/hosts` 被拒绝**

   ```bash
   curl -s -X POST http://127.0.0.1:8787/v1/runs -H 'Content-Type: application/json' -d '{
     "input":"请使用 read_file 读取 /etc/hosts",
     "workspace":{"workspace_root":"/Users/nolangrant/Awaken"},
     "client":{"session_id":"smoke-deny","request_id":"r2","entrypoint":"desktop"}
   }'
   ```

   期望：工具调用返回权限拒绝，run 不泄露文件内容。

3. **`shell cat /etc/hosts` 只读外部读取按设计允许**

   ```bash
   curl -s -X POST http://127.0.0.1:8787/v1/runs -H 'Content-Type: application/json' -d '{
     "input":"请使用 shell 工具执行 cat /etc/hosts",
     "workspace":{"workspace_root":"/Users/nolangrant/Awaken"},
     "client":{"session_id":"smoke-shell","request_id":"r3","entrypoint":"desktop"}
   }'
   ```

   期望：`shell` 只读命令可以成功返回信息；这不构成 Slice 2 workspace 文件工具失败。

4. **symlink 逃逸**

   在 `/Users/nolangrant/Awaken` 内创建 symlink：

   ```bash
   ln -sf /etc/hosts /Users/nolangrant/Awaken/symlink-smoke-test
   ```

   然后运行：

   ```bash
   curl -s -X POST http://127.0.0.1:8787/v1/runs -H 'Content-Type: application/json' -d '{
     "input":"请使用 read_file 读取 symlink-smoke-test",
     "workspace":{"workspace_root":"/Users/nolangrant/Awaken"},
     "client":{"session_id":"smoke-symlink","request_id":"r4","entrypoint":"desktop"}
   }'
   ```

   期望：`read_file` 拒绝，不能读出 `/etc/hosts` 内容。

   验证后清理：

   ```bash
   rm /Users/nolangrant/Awaken/symlink-smoke-test
   ```

5. **`list_files` / `grep_search` 越界仍拒绝**

   分别请求 `list_files {"root":"/etc"}` 和 `grep_search {"root":"/etc","pattern":"root"}`，期望均返回权限拒绝。

---

## Acceptance Criteria

- [ ] `go test ./...` 全部通过。
- [ ] Desktop typecheck/build 通过。
- [x] 真实 LLM smoke 中 `shell cat /etc/hosts` 可静默成功，符合当前只读 shell 设计。
- [ ] 真实 LLM smoke 中 workspace 内 symlink 指向 `/etc/hosts` 时 `read_file` 拒绝。
- [ ] `read_file`/`list_files`/`grep_search` 对 workspace 外绝对路径仍拒绝。
- [ ] 新增测试覆盖 shell 路径参数审查与 symlink 逃逸场景。

只要 `read_file`、`list_files`、`grep_search` 仍能静默访问 workspace 外文件，Slice 2 验收就不通过。`shell` 只读命令读取 workspace 外文件是允许能力，不作为失败项。
