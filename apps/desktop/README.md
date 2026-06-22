# Awaken Desktop

`apps/desktop` 是 Awaken 的 Electron + React 桌面端。Desktop 是唯一用户入口：Electron main 管理本机 `awaken-core` 生命周期，renderer 只通过 `window.awaken` bridge 发起 run、订阅事件、取消 run、处理 approval，并预览只读 `a2ui_surface` artifacts。

当前主界面是 Run Workbench：左侧 Sidebar 管理 runs/threads，中间 Run Narrative 展示 prompt、thinking、tool、approval、file change、answer，右侧 Inspector 展示 Changes/Browser/Terminal/Files，底部 Composer 发起 run、follow-up 和 stop/cancel。

## 目录

```text
apps/desktop/
├── src/main
├── src/preload
├── src/renderer
└── src/shared
```

## Bridge 契约

`window.awaken` 暴露最小桌面桥接：

```ts
type AwakenBridge = {
  platform: "macos" | "windows" | "linux";
  getRun(runId: string): Promise<RuntimeRun>;
  listThreads(): Promise<RuntimeThread[]>;
  startRun(request: StartRunRequest): Promise<RuntimeRun>;
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
    afterSeq?: number,
  ): Promise<string>;
  unsubscribeRun(subscriptionId: string): void;
  getRuntimeInfo(): Promise<RuntimeInfo>;
  pickWorkspace(defaultPath?: string): Promise<WorkspacePickResult>;
  validateWorkspace(path: string): Promise<WorkspaceValidationResult>;
  getSupervisorStatus(): Promise<{ status: "stopped" | "starting" | "running" | "crashed" | "stopping" }>;
  onSupervisorStatusChange(
    callback: (info: { status: "stopped" | "starting" | "running" | "crashed" | "stopping" }) => void,
  ): () => void;
};
```

Renderer 内部再把 bridge 包装成 React 使用的 client。`@awaken/design-system` 仍保持 UI-only；desktop 只通过包出口 import，不读 `Awaken Design System/src/...`。

## 开发流程

```bash
npm run dev:desktop
```

这个命令会：

- 构建 `@awaken/design-system`
- 确保当前平台的 `awaken-core` binary 存在，缺失时自动构建
- watch 编译 Electron `main` / `preload`
- 启动 Vite renderer（`127.0.0.1:5174`）
- 启动 Electron，并通过 `window.awaken` 走桌面 bridge

## 发布前验收

```bash
npm run acceptance:desktop-hardening
```

这个 gate 会确保当前平台 runtime binary 存在，跑 Desktop typecheck、Desktop unit tests、Go runtime/server tests、真实 LLM smoke，并完成 packaged Desktop build。最后仍需实际打开 dev Desktop 和 `Awaken.app` 各完成一次真实 run，作为发布证据。

Run Workbench 收口验收常用命令：

```bash
npm --workspace @awaken/desktop run test
npm run typecheck:desktop
npm run build:desktop
npm run typecheck:ui
go test ./src/server -run 'TestPlainQASmoke|TestReadOnlyFileSmoke|TestFileMutation(Approval|Reject|Artifact|PatchAndTurnDiff)Smoke' -count=1 -v
go test ./src/server -run 'TestRunsEndpointsCreateReplayAndCancel|TestRunEventsAfterSeq|TestRunEventsAfterSeqReconnectMultiFrame' -count=1
git diff --check
```

旧主界面关键词扫描命令记录在仓库根部 handoff 文档，避免 Desktop 文档自身被产品路径扫描误命中。

视觉 QA 要打开 Desktop，确认第一屏是左 Sidebar、中间 Run Narrative、右 Inspector、底部 Composer；不要把 UI playground 或 mock run 当作真实验收。

## 验收边界

- Desktop 消费以下 runtime endpoints：
  - `GET /health`
  - `GET /v1/runtime/info`
  - `POST /v1/runs`
  - `GET /v1/runs/{run_id}`
  - `GET /v1/runs/{run_id}/events`
  - `GET /v1/threads`
  - `POST /v1/runs/{run_id}/cancel`
  - `POST /v1/runs/{run_id}/approvals/{approval_id}/resolve`
- Renderer 消费标准 AG-UI run/text/reasoning/tool 事件，以及 Awaken extension events：
  - `TOOL_CALL_ARGS_DELTA`
  - `TOOL_PROGRESS`
  - `AWAKEN_COMMAND_OUTPUT`
  - `AWAKEN_PATCH_UPDATED`
  - `AWAKEN_TURN_DIFF`
  - `AWAKEN_APPROVAL_REQUESTED`
  - `AWAKEN_APPROVAL_RESOLVED`
  - `AWAKEN_ARTIFACT_CREATED`
  - `AWAKEN_ARTIFACT_UPDATED`
- Desktop 只负责 bridge、SSE、renderer state 和 UI 交互，不在 desktop 里实现新的 policy/domain logic。
- Approval request 会附带一个 `a2ui_surface` artifact，renderer 只读预览，不执行 surface action，不替代 `ApprovalCard`。
