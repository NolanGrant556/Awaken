# @awaken/ui

Awaken 的 UI-only React 组件库。这个包只负责组件渲染、局部交互、视觉 token、布局和跨平台 UI 适配，不接入 Agent runtime、Electron IPC 或后端 API。

当前 v2 设计 token 来源是 Run Workbench，而不是早期 HTML demo。`docs/component-library.md` 只作为历史档案；新增或调整 token 时优先从已进入 Desktop Workbench 的 sidebar、stage、inspector、composer、narrative、terminal/diff 状态中抽象。

## 边界

- `src/` 是组件库源码，包含 `components/`、`styles/`、`types.ts` 和公共入口 `index.ts`。
- `playground/` 是组件预览和验收台，不是 Awaken 产品入口。
- `demos/` 保存独立 HTML 原型，不参与包发布。
- Electron、`window.awaken`、preload、SSE/WebSocket、`/v1` API 调用都属于未来 `apps/desktop`，不能进入本包。

## 使用方式

```tsx
import { Composer, Sidebar, RunTimeline } from "@awaken/ui";
import "@awaken/ui/styles/tokens.css";
import "@awaken/ui/styles/components.css";
```

组件通过 props 接收数据，通过 callbacks 向业务层传出事件。业务层负责调用后端、Electron IPC 或 Agent runtime。

## 开发

从仓库根目录运行：

```bash
npm install
npm run dev:ui
npm run build:ui
npm run typecheck:ui
```

也可以直接进入包目录：

```bash
cd packages/ui
npm run dev
npm run build
npm run typecheck
```

## 发布出口

当前包仍为 `private: true`，但已经按可发布包设计出口：

- `@awaken/ui`
- `@awaken/ui/styles/tokens.css`
- `@awaken/ui/styles/components.css`

构建产物输出到 `dist/`，类型声明由 `tsconfig.build.json` 生成。

## 设计档案

组件库实现档案见：

```text
../../docs/component-library.md
```
