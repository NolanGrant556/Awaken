# Awaken Desktop

`apps/desktop` 承载当前最小可运行的 Electron + React + Vite desktop consumer。它连接本地 `awaken-core` runtime，消费 AG-UI-compatible SSE events，处理 approval resolve，并预览只读 `a2ui_surface` artifacts：

- `main` 负责创建 `BrowserWindow`，并通过 Node/Electron 侧 HTTP 请求访问 `awaken-core`
- `preload` 暴露 `window.awaken`，屏蔽 renderer 对 Electron IPC 的感知
- `renderer` 通过 adapter 创建 run、订阅 SSE、取消 run、resolve approval，并把 AG-UI events reduce 成 view state
- `renderer/a2ui` 只识别 `artifactKind: "a2ui_surface"` 的受限 JSON payload，并映射到白名单 React 组件
- `@awaken/ui` 仍然保持 UI-only；desktop 只通过包出口 import，不读 `packages/ui/src/...`

## 目录

```text
apps/desktop/
├── src/main
├── src/preload
├── src/renderer
└── src/shared
```

## Runtime / adapter 契约

`window.awaken` 暴露最小桌面桥接：

```ts
type AwakenBridge = {
  platform: "macos" | "windows" | "linux";
  runtimeBaseUrl: string;
  createRun(input: string): Promise<RuntimeRun>;
  cancelRun(runId: string): Promise<RuntimeRun>;
  resolveApproval(
    runId: string,
    approvalId: string,
    decision: "approved" | "rejected",
  ): Promise<void>;
  subscribeRun(
    runId: string,
    handlers: {
      onEvent: (event: AguiEvent) => void;
      onError?: (message: string) => void;
      onClose?: () => void;
    },
  ): Promise<string>;
  unsubscribeRun(subscriptionId: string): void;
};
```

renderer 内部再把它包装成更适合 React 使用的 runtime client；浏览器直接跑 Vite 时则退回 `fetch` + `EventSource` fallback。

## 开发流程

### 1. 启动 runtime

默认地址是 `http://127.0.0.1:8787`：

```bash
go run ./cmd/awaken-core
```

如果要改地址，可以设置：

```bash
AWAKEN_CORE_ADDR=127.0.0.1:9999 go run ./cmd/awaken-core
AWAKEN_DESKTOP_RUNTIME_URL=http://127.0.0.1:9999 npm run dev:desktop
```

### 2. 启动 desktop

第一次或 `packages/ui` 有改动时，先构建 UI 包：

```bash
npm run build:ui
```

然后启动桌面端：

```bash
npm run dev:desktop
```

这个命令会：

- watch 编译 Electron `main` / `preload`
- 启动 Vite renderer（`127.0.0.1:5174`）
- 启动 Electron，并通过 `window.awaken` 走桌面 bridge

如果只想看 renderer fallback，也可以单独开：

```bash
npm --workspace @awaken/desktop run dev:renderer
```

默认 Vite 会把 `/health`、`/v1/*` 代理到 `http://127.0.0.1:8787`，所以浏览器 fallback 不依赖 CORS 改动。

## 验收边界

- Desktop 消费以下 runtime endpoints：
  - `GET /health`
  - `POST /v1/runs`
  - `GET /v1/runs/{run_id}/events`
  - `POST /v1/runs/{run_id}/cancel`
  - `POST /v1/runs/{run_id}/approvals/{approval_id}/resolve`
- renderer 消费的 runtime event 除标准 AG-UI run/text/reasoning/tool 事件外，还包括 Awaken extension events：
  - `AWAKEN_APPROVAL_REQUESTED`
  - `AWAKEN_APPROVAL_RESOLVED`
  - `AWAKEN_ARTIFACT_CREATED`
  - `AWAKEN_ARTIFACT_UPDATED`
- `@awaken/ui` 仍保持 UI-only；desktop 负责 bridge、SSE、reducer、approval resolve 调用
- Desktop 只负责 bridge、SSE、renderer state 和 UI 交互，不在 desktop 里实现新的 policy/domain logic
- Approval request 会附带一个 `a2ui_surface` artifact，renderer 只读预览，不执行 surface action，不替代 `ApprovalCard`
