# Slice 11 修复计划：Provider 错误脱敏 + UI 状态完善

## Summary

修复 slice11 代码审查发现的问题：对 `PROVIDER_REQUEST_FAILED` 错误进行脱敏（不暴露完整 URL、provider body、潜在 secret），补脱敏测试，在顶部栏显示 provider host，补齐 UI 状态 pill 样式。

## Current State Analysis

### 错误脱敏现状

LLM 包有两类 provider 错误路径，均会将敏感信息带入 `RunError.message`：

1. **Transport error**（`httpClient.Do` 失败）：4 处调用点使用 `fmt.Errorf("%w: %v", ErrRequestFailed, err)`，`%v` 会把 `url.Error` 完整字符串（含 full base URL）写入错误消息。
   - [openai_chat.go:114](file:///Users/nolangrant/Awaken/src/llm/openai_chat.go#L114) (Invoke)
   - [openai_chat.go:163](file:///Users/nolangrant/Awaken/src/llm/openai_chat.go#L163) (StreamInvoke)
   - [anthropic_messages.go:115](file:///Users/nolangrant/Awaken/src/llm/anthropic_messages.go#L115) (Invoke)
   - [anthropic_messages.go:161](file:///Users/nolangrant/Awaken/src/llm/anthropic_messages.go#L161) (StreamInvoke)

2. **Non-2xx response**：[client.go:168-178](file:///Users/nolangrant/Awaken/src/llm/client.go#L168-L178) `requestFailedError` 把 provider response body（最多 1000 字符）写入错误消息。provider body 可能包含 `sk-` key 片段、内部路径等敏感信息。

错误链路：LLM 包返回 error → `runner.go:116` 包装为 `fmt.Errorf("LLM stream call failed: %w", err)` → `runErrored` 用 `err.Error()` 作为 `RunError.message` → 经 agui mapper → SSE → UI reducer 显示为 `Provider 请求失败：{message}`。

### UI 现状

- [App.tsx:517-533](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/App.tsx#L517-L533) 已有 `Ready`/`Not ready`/`LLM_API_KEY required` pill，但使用的 CSS class `runtime-pill-connected`、`runtime-pill-warning`、`runtime-pill-disconnected` 在 [styles.css](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/styles.css) 中未定义，所有 pill 渲染为相同的灰色基础样式。
- 顶部栏未显示 provider host（只显示 runtime base URL、model、protocol）。
- `RuntimeInfo.baseUrlHost` 字段已存在于 [shared/runtime.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/shared/runtime.ts)，后端 `extractHost` 已实现，数据链路完整，仅前端未展示。

## Proposed Changes

### 1. [必须修] LLM 包错误脱敏

**文件**: `src/llm/client.go`

在 `requestFailedError` 函数中移除 provider body，仅保留 status code（安全白名单字段）。仍 drain body 以允许连接复用。

```go
func requestFailedError(resp *http.Response) error {
    // Drain body to allow connection reuse, but do not include body in error
    // message to avoid leaking provider-internal details or secrets.
    io.Copy(io.Discard, resp.Body)
    return fmt.Errorf("%w: status %d", ErrRequestFailed, resp.StatusCode)
}
```

新增统一 transport error helper（放在 `requestFailedError` 附近）：

```go
// transportError 返回脱敏的 transport 错误，不暴露完整 URL 或 url.Error 细节。
func transportError(err error) error {
    return fmt.Errorf("%w: transport error", ErrRequestFailed)
}
```

移除 `requestFailedError` 中不再使用的 `strings.TrimSpace` 逻辑（`detail` 变量不再需要）。`strings` import 若无其他引用需保留——确认 `client.go` 其他地方仍用 `strings`（`loadConfig` 中 `strings.TrimSuffix`），所以 import 不变。

**文件**: `src/llm/openai_chat.go`

将 2 处 `fmt.Errorf("%w: %v", ErrRequestFailed, err)` 替换为 `transportError(err)`：
- Line 114 (Invoke)
- Line 163 (StreamInvoke)

**文件**: `src/llm/anthropic_messages.go`

将 2 处 `fmt.Errorf("%w: %v", ErrRequestFailed, err)` 替换为 `transportError(err)`：
- Line 115 (Invoke)
- Line 161 (StreamInvoke)

**文件**: `src/agentcore/runner.go`

`runErrored` 无需代码改动。LLM 包脱敏后，`err.Error()` 已安全（形如 `LLM stream call failed: LLM request failed: status 503`）。`errors.Is(err, llm.ErrRequestFailed)` 仍成立（`%w` 包装 preserved）。

### 2. [必须修] 补脱敏测试

**文件**: `src/llm/invoke_test.go`（或新建 `src/llm/sanitize_test.go`，优先编辑现有文件）

新增测试用例：

```go
// TestInvokeOpenAINonOKDoesNotLeakResponseBody
// - provider 返回 502，body 含 "sk-test-secret"
// - 断言 errors.Is(err, ErrRequestFailed) 成立
// - 断言 err.Error() 不含 "sk-test-secret"
// - 断言 err.Error() 含 "status 502"

// TestInvokeOpenAITransportErrorDoesNotLeakURL
// - cfg.BaseURL = "http://127.0.0.1:1/invalid"
// - 断言 errors.Is(err, ErrRequestFailed) 成立
// - 断言 err.Error() 不含 "127.0.0.1:1"
// - 断言 err.Error() 不含 "/invalid"
// - 断言 err.Error() 含 "transport error"
```

对 Anthropic transport 做同等覆盖（可放在 `anthropic_test.go` 或同一文件，用 anthropic protocol config）。

**文件**: `src/agentcore/runner_test.go`

扩展现有 `TestRunEventEmitterRunErroredSetsRequestFailedCode` 或新增用例：
- 构造一个 transport error（bad base URL），通过 `runErrored` 发射事件
- 断言 `Code == "PROVIDER_REQUEST_FAILED"`
- 断言 `Error` 字段不含完整 URL

**文件**: `src/server/server_test.go`

扩展 `TestRunWithBadBaseURLEmitsProviderRequestFailed`：
- 新增断言：SSE `message` 不含 `127.0.0.1:1` 和 `/invalid`

新增测试 `TestRunWithProviderBodySecretEmitsSanitizedMessage`：
- 用 httptest server 返回 502 + body 含 `sk-test-secret`
- 断言 SSE `code == "PROVIDER_REQUEST_FAILED"`
- 断言 SSE `message` 不含 `sk-test-secret`
- 断言 SSE `message` 含 `status 502`

### 3. [应该修] 顶部展示 provider host

**文件**: `apps/desktop/src/renderer/App.tsx`

在 protocol pill 之后（line 528 附近）新增 provider host pill：

```tsx
<span className="runtime-pill runtime-pill-muted">
  {runtimeInfo.baseUrlHost || "unknown provider"}
</span>
```

不显示 full base URL，仅显示 host（后端 `extractHost` 已保证只返回 host）。

### 4. [应该修] 补 UI 状态 pill 样式

**文件**: `apps/desktop/src/renderer/styles.css`

在 `.runtime-pill-status` 规则之后新增：

```css
.runtime-pill-connected {
  background: rgba(34, 197, 94, 0.12);
  color: #166534;
}

.runtime-pill-warning {
  background: rgba(234, 88, 12, 0.12);
  color: #9a3412;
}

.runtime-pill-disconnected {
  background: rgba(107, 114, 128, 0.12);
  color: #374151;
}
```

这些颜色为内联 rgba 值（与现有 `.runtime-pill-status` 的 `rgba(0, 129, 242, 0.11)` 风格一致），不引入新 token。

### 5. [可选，不修] errorDescription 冗余变量

按用户指示，不为 this 单独改 [reducer.ts](file:///Users/nolangrant/Awaken/apps/desktop/src/renderer/runtime/reducer.ts)。

## Assumptions & Decisions

1. **脱敏粒度**：transport error 用通用文案 `transport error`（不区分 connection refused / timeout / DNS failure），non-2xx 仅保留 status code。理由：用户示例即为 `LLM request failed: status 503` / `LLM request failed: transport error`，且 status code 是安全白名单字段。
2. **`errors.Is` 兼容**：`fmt.Errorf("%w: ...", ErrRequestFailed, ...)` 保持 `%w` 包装，`errors.Is(err, ErrRequestFailed)` 不受影响。
3. **runner.go 不改代码**：LLM 包脱敏后，runner.go 的 `err.Error()` 已安全。用户列出该文件是因为它在错误链路中，但实际改动点在 LLM 包。
4. **Body drain**：`requestFailedError` 仍 `io.Copy(io.Discard, resp.Body)` 以允许 HTTP 连接复用，只是不把 body 放入 error message。
5. **CSS 颜色**：用内联 rgba 而非新 CSS variable，与现有 `.runtime-pill-status` 风格一致，避免引入 token 变更。
6. **测试文件选择**：LLM 包脱敏测试优先放入现有 `invoke_test.go`（已有 `TestInvokeOpenAINonOK` 模式可复用），不新建文件。

## Verification Steps

```bash
# Go 测试（含真实 LLM smoke，需 .env 有 key）
go test ./...

# Desktop typecheck
npm run typecheck:desktop

# Desktop 单元测试
npm --workspace @awaken/desktop run test

# Desktop 构建
npm run build:desktop

# 检查无空白符问题
git diff --check
```

验证要点：
- `go test ./src/llm/...` 新增脱敏测试通过
- `go test ./src/agentcore/...` runner_test 通过
- `go test ./src/server/...` 扩展后的 `TestRunWithBadBaseURLEmitsProviderRequestFailed` 和新增 body-secret 测试通过
- `go test ./src/server/... -run TestModelStatusSmoke` 真实 LLM smoke 通过（如有 key）
- `npm run typecheck:desktop` 无类型错误
- `npm --workspace @awaken/desktop run test` reducer 测试通过
- `npm run build:desktop` 构建成功
- 手动确认：顶部栏 Ready/Not ready pill 有清晰颜色区分，provider host 可见
