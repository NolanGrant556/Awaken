# Plan：Slice 2 workspace 安全修复收尾（按当前产品策略更新）

## Summary

> 2026-06-18 更新：`shell` 只读命令读取 workspace 外文件已确认为产品设计允许项，不再作为 P0 blocker。当前 P0 安全边界是：`read_file/list_files/grep_search` 不能越界或通过 symlink 逃逸；写入/修改只能在 `writable_roots` 内并按审批策略执行。

Slice 2 的 workspace 文件工具安全修复（`read_file/list_files/grep_search` symlink 逃逸）已在仓库中完成。本次计划的任务是：

1. 通过确定性测试 + Desktop 构建 + 真实 LLM smoke 完成 workspace 文件工具验收。
2. 完成 P2 改进：Desktop 使用系统目录选择器、main 侧校验 workspace 路径、create-run 返回结构化错误。
3. 完成 P3 清理：统一 `pathWithinRoot` 重复实现、更新 Desktop README 契约文档。

## Current State Analysis

### 已完成的 workspace 文件工具修复（无需再改实现，只需验证）

- `src/tool/permission.go`
  - 新增 `IsWithinWorkspaceResolved`：用 `filepath.EvalSymlinks` 解析 symlink 后判断边界。
  - 新增 `ResolvePathChecked`：解析并检查边界，越界返回错误。
- `src/tools/shell/shell.go`
  - `shell` 只读命令可按用户本机权限读取 workspace 外信息；mutation、网络、仓库状态变更、重定向、管道和复杂 shell 操作仍由审批/拒绝策略处理。
- `src/tools/file_read/file_read.go`
  - `CheckPermission` 使用 `IsWithinWorkspaceResolved`。
  - `Execute` 使用 `ResolvePathChecked`。
- `src/tools/list_files/list_files.go` / `src/tools/grep_search/grep_search.go`
  - `CheckPermission` 使用 `IsWithinWorkspaceResolved`。
  - 遍历过程中跳过 symlink 逃逸的文件。
- 对应测试已新增在：
  - `src/tool/permission_test.go`
  - `src/tools/shell/shell_test.go`
  - `src/tools/file_read/file_read_test.go`
  - `src/tools/list_files/list_files_test.go`
  - `src/tools/grep_search/grep_search_test.go`

### 待完成的 P2/P3

- **P2-1：workspace picker 仍是文本输入**
  - 位置：`apps/desktop/src/renderer/App.tsx:338`
  - 当前是 `<input>`，用户可手输任意路径。
- **P2-2：前端路径校验过弱**
  - 位置：`apps/desktop/src/renderer/App.tsx:135`
  - 仅用 `/^\//` 判断绝对路径，不跨平台、不检查目录存在性。
- **P2-3：create-run 错误非结构化**
  - 位置：`src/server/handlers.go:31`
  - `ErrInvalidInput` 返回纯文本 400。
- **P3-1：Desktop README 仍是旧 bridge 契约**
  - 位置：`apps/desktop/README.md:29`
  - 仍写 `createRun(input)`，实际已是 `startRun(request)`。
- **P3-2：`pathWithinRoot` 重复实现**
  - 位置：`src/tool/permission.go:225`、`src/runtime/run_service.go:216`
  - 两份 `pathWithinRoot` / `absoluteCleanPath` 逻辑高度重复。

---

## Proposed Changes

### 变更 1：P0 验证（不修改业务代码）

#### 1.1 确定性测试

```bash
cd /Users/nolangrant/Awaken
go test ./...
```

#### 1.2 Desktop 构建

```bash
npm run typecheck:desktop
npm run build:desktop
```

#### 1.3 真实 LLM smoke

启动 awaken-core：

```bash
go run ./cmd/awaken-core
```

逐个验证：

1. workspace 内 `README.md` 可读取并总结。
2. `read_file /etc/hosts` 被拒绝。
3. `shell cat /etc/hosts` 可成功，符合当前只读 shell 设计。
4. workspace 内 symlink -> `/etc/hosts` 不能通过 `read_file` 读出内容。
5. `list_files / grep_search` 对 workspace 外路径仍拒绝。

### 变更 2：Desktop workspace 系统目录选择器 + main 侧校验

#### 2.1 `apps/desktop/src/main/index.ts`

- 引入 `dialog` 与 `fs/promises`。
- 新增 IPC handler：

```ts
ipcMain.handle(
  "awaken:pick-workspace",
  async (_event, defaultPath?: string) => {
    const result = await dialog.showOpenDialog({
      properties: ["openDirectory"],
      defaultPath,
    });
    if (result.canceled || result.filePaths.length === 0) {
      return { canceled: true };
    }
    const selected = result.filePaths[0];
    try {
      const stat = await fs.stat(selected);
      if (!stat.isDirectory()) {
        return { canceled: true, error: "Selected path is not a directory" };
      }
      return { canceled: false, path: selected };
    } catch {
      return { canceled: true, error: "Cannot access selected directory" };
    }
  },
);
```

#### 2.2 `apps/desktop/src/shared/runtime.ts`

扩展 `AwakenBridge` 类型：

```ts
export type WorkspacePickResult =
  | { canceled: true; error?: string }
  | { canceled: false; path: string };

export type AwakenBridge = {
  // ...existing fields
  pickWorkspace: (defaultPath?: string) => Promise<WorkspacePickResult>;
};
```

#### 2.3 `apps/desktop/src/preload/index.ts`

在 `bridge` 对象中新增：

```ts
pickWorkspace: (defaultPath?: string) =>
  ipcRenderer.invoke("awaken:pick-workspace", defaultPath) as Promise<WorkspacePickResult>,
```

#### 2.4 `apps/desktop/src/renderer/App.tsx`

- 把 workspace 输入区从纯 `<input>` 改为：
  - 一个显示当前路径的只读/可编辑输入框（保留原有视觉）。
  - 一个 "Choose..." 按钮，点击调用 `bridge.pickWorkspace(workspacePath || runtimeInfo?.defaultWorkspace)`。
  - 选择成功后更新 `workspacePath`。
- 新增 main 侧校验入口：可复用 `pickWorkspace` 的校验逻辑，或在 main 中新增 `awaken:validate-workspace` IPC，用于手动输入时校验。
- 建议实现：
  - 保留 `isWorkspaceValid` 本地非空检查。
  - 在 `handleSubmit` 前调用 `runtimeClient.pickWorkspace(workspacePath)` 的校验分支（或直接信任 picker 返回值）。
  - 若路径无效，通过 `setNotice` 提示用户。

### 变更 3：create-run 返回结构化错误

#### 3.1 `src/server/handlers.go`

新增错误响应类型与 helper：

```go
type errorResponse struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

func writeErrorJSON(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}
```

修改 `handleCreateRun`：

```go
func (h runHandlers) handleCreateRun(w http.ResponseWriter, r *http.Request) {
    var req awruntime.StartRunRequest
    if err := decodeJSON(r, &req); err != nil {
        writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }

    run, err := h.runService.StartRun(req)
    if err != nil {
        if errors.Is(err, awruntime.ErrInvalidInput) {
            writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
            return
        }
        writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start run")
        return
    }

    writeJSON(w, http.StatusCreated, run)
}
```

#### 3.2 `apps/desktop/src/main/index.ts`

修改 `readErrorMessage`，优先解析 JSON 错误：

```ts
async function readErrorMessage(response: Response): Promise<string> {
  const fallback = `Runtime request failed (${response.status})`;
  const body = (await response.text()).trim();
  if (!body) return fallback;
  try {
    const parsed = JSON.parse(body);
    if (parsed.code && parsed.message) {
      return `${parsed.code}: ${parsed.message}`;
    }
  } catch {
    // fall through
  }
  return body;
}
```

#### 3.3 `apps/desktop/src/renderer/runtime/client.ts`

同步修改浏览器 fallback 的 `readErrorMessage`：

```ts
async function readErrorMessage(response: Response): Promise<string> {
  const body = (await response.text()).trim();
  if (!body) return `Runtime request failed (${response.status})`;
  try {
    const parsed = JSON.parse(body);
    if (parsed.code && parsed.message) {
      return `${parsed.code}: ${parsed.message}`;
    }
  } catch {
    // fall through
  }
  return body;
}
```

#### 3.4 `src/server/server_test.go`

在 `TestRunsEndpointsValidationAndNotFound` 中断言空 input 返回 JSON：

```go
var errResp errorResponse
if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
    t.Fatalf("expected JSON error response, got %q", rec.Body.String())
}
if errResp.Code != "INVALID_INPUT" {
    t.Fatalf("expected INVALID_INPUT code, got %q", errResp.Code)
}
```

### 变更 4：统一 `pathWithinRoot` 实现

#### 4.1 `src/tool/permission.go`

将以下函数导出（首字母大写）：

- `pathWithinRoot` -> `PathWithinRoot`
- `absoluteCleanPath` -> `AbsoluteCleanPath`

并更新文件内所有调用处。

#### 4.2 `src/runtime/run_service.go`

- 删除本地 `pathWithinRoot` 和 `absoluteCleanPath`。
- 导入 `awaken/src/tool`。
- 在 `normalizeWorkspace` 中改为：

```go
if !tool.PathWithinRoot(w, root) {
    w = root
}
```

以及需要绝对路径清理的地方使用 `tool.AbsoluteCleanPath`。

### 变更 5：更新 Desktop README

#### 5.1 `apps/desktop/README.md`

- 将 bridge 类型中的 `createRun(input: string)` 改为 `startRun(request: StartRunRequest)`。
- 更新说明文字，使其与当前 `startRun` 契约一致。

---

## Assumptions & Decisions

1. **Shell 策略不回滚**：`shell` 工具保留；只读外部读取允许，mutation/复杂命令审批或拒绝。
2. **系统选择器为主、手动输入为辅**：保留输入框用于显示和微调，但真正的路径选择走 `dialog.showOpenDialog`，并由 main 侧校验目录存在性。
3. **错误码稳定**：`create-run` 统一返回 `{code, message}`，Desktop 侧解析后展示 `CODE: message`。
4. **路径策略函数放在 `tool` 包**：将 `PathWithinRoot` / `AbsoluteCleanPath` 从 `tool` 包导出，供 `runtime` 复用，避免新增内部包带来的额外文件。
5. **LLM smoke 依赖真实 API key**：若当前环境无法调用 LLM，则完成确定性测试和构建后，记录 smoke 阻塞原因。

---

## Verification Steps

### 阶段 1：确定性测试

```bash
cd /Users/nolangrant/Awaken
go test ./...
```

必须全部通过。

### 阶段 2：Desktop 构建

```bash
npm run typecheck:desktop
npm run build:desktop
```

必须全部通过。

### 阶段 3：真实 LLM smoke

```bash
go run ./cmd/awaken-core
```

依次执行并确认：

1. workspace 内 README 可读取总结。
2. `read_file /etc/hosts` 被拒绝。
3. `shell cat /etc/hosts` 可静默成功，符合只读 shell 设计。
4. workspace 内 symlink -> `/etc/hosts` 不能通过 `read_file` 读出。
5. `list_files / grep_search` 对 workspace 外路径仍拒绝。

### 阶段 4：P2/P3 功能验证

- Desktop 点击 workspace "Choose..." 能打开系统目录选择器。
- 选择非目录文件会被拒绝。
- 手动输入不存在的路径会被 main 侧校验拒绝。
- `create-run` 空 input 时返回 `{"code":"INVALID_INPUT","message":"..."}`。
- `go test ./...` 在统一 `pathWithinRoot` 后仍通过。
- README 中的 bridge 类型与代码一致。

---

## Acceptance Criteria

- [ ] `go test ./...` 全部通过。
- [ ] `npm run typecheck:desktop` / `npm run build:desktop` 通过。
- [ ] workspace 文件工具 smoke 5 项全部通过，或明确记录 API key 不可用导致的阻塞。
- [ ] Desktop workspace 选择器使用系统目录对话框且校验目录存在性。
- [ ] `create-run` 返回结构化 JSON 错误。
- [ ] `pathWithinRoot` 只保留一份实现。
- [ ] Desktop README 契约与代码一致。
