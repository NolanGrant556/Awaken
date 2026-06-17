# Awaken Desktop

这里预留给未来真正的 Awaken 桌面客户端。

当前仓库已经把 UI 组件库独立到 `packages/ui`，本目录后续负责 Electron 应用本身：

- main process
- preload bridge
- renderer application
- `window.awaken` adapter
- awaken-core / Agent runtime 接入
- 本地文件权限、审批、任务运行状态和流式事件接入

## 边界

桌面端可以 import UI 组件：

```tsx
import { Composer, Sidebar, RunTimeline } from "@awaken/ui";
```

但不要引用组件库源码路径：

```tsx
// 不允许
import { Composer } from "../../packages/ui/src/components/inputs/Composer";
```

Electron IPC、后端 API、SSE/WebSocket 和本地 runtime 调用只能存在于本应用或 adapter 层，不能进入 `@awaken/ui`。

## 当前状态

尚未实现 Electron 应用。本 README 只用于固定未来产品客户端与 UI 组件库的包边界。
