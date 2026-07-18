# Awaken Design System

Awaken Design System 是 Awaken 的 UI-only 组件系统。它包含可复用 React 组件、CSS tokens、组件局部样式、可观测 demo states，以及用于肉眼检查组件真实形态的本地 Gallery。

这套组件必须可以从 Awaken Desktop 中抽离后继续复用。它不能依赖 Go runtime、Electron、AG-UI、A2UI、SSE、本地文件系统或 `window.awaken`。

## 目录职责

```text
Awaken Design System/
├── src/                         # 设计系统源码，只放可复用 UI 能力
│   ├── components/              # 独立组件目录，每个组件一个文件夹
│   ├── tokens/                  # 全局设计令牌与组件基础样式
│   ├── hooks/                   # UI-only React hooks
│   ├── assets/                  # 设计系统可复用静态资源
│   ├── index.ts                 # 包主出口，Desktop 与外部项目应从这里导入
│   └── types.ts                 # 设计系统共享类型
├── gallery/                     # 本地组件观测服务源码
│   ├── registry.ts              # 组件 URL 与 demo 注册表，组件观测入口的事实源
│   ├── AwakenGalleryApp.tsx     # Gallery 壳层与 hash 路由
│   ├── main.tsx                 # Gallery Vite 入口
│   └── styles.css               # Gallery 专用样式，不应变成组件样式事实源
├── smoke/                       # 自动化渲染冒烟检查
│   └── render-smoke.mjs         # server-render 所有 Awaken*Demo
├── dist/                        # 构建产物，由 build 生成，不手写维护
├── DESIGN_SYSTEM_GUIDE.md       # Awaken 视觉消费与 Agent 作者规则
├── index.html                   # Gallery HTML 入口
├── package.json                 # @awaken/design-system 包、脚本与依赖声明
├── vite.config.ts               # Gallery 与包构建配置
├── tsconfig.json                # 开发期 TypeScript 配置
└── tsconfig.build.json          # 类型声明构建配置
```

关键边界：

- `src/components/` 是组件真实实现；`gallery/` 只负责观测，不承载产品逻辑。
- `src/tokens/` 是颜色、间距、字体、边框、组件基础类等设计令牌事实源。
- `dist/` 是生成目录，可以删除后重建；不要在里面修 bug。
- `DESIGN_SYSTEM_GUIDE.md` 是后续视觉改动的作者规则，优先于临时审美判断。
- `gallery/registry.ts` 决定哪些组件拥有独立观测 URL。
- `src/index.ts` 决定哪些能力对 Desktop 或外部项目公开。

## 入口契约

Awaken Desktop 的正式平台入口由仓库根 `README.md` 唯一定义。macOS 当前从仓库根目录启动和关闭：

```bash
npm run open:desktop
npm run close:desktop
```

Windows 当前使用前台入口 `npm run dev:desktop`，并在对应终端按 `Ctrl+C` 关闭。平台验收范围和入口细节不在本文档重复维护。

组件独立观测入口：

从仓库根目录启动并打开 Gallery：

```bash
cd /Users/nolangrant/Awaken
npm run open:ui
```

如果只需要启动服务、不需要自动打开浏览器：

```bash
cd /Users/nolangrant/Awaken
npm run dev:ui
```

Gallery 地址：

```text
http://127.0.0.1:5173/#/components/<component-id>
```

然后打开具体组件 URL，例如：

```text
http://127.0.0.1:5173/#/components/awaken-composer-bar
http://127.0.0.1:5173/#/components/awaken-execution-feed
http://127.0.0.1:5173/#/components/awaken-light-inspector-rail
```

`npm run dev:ui` 只是启动组件观测服务的命令。真正的观测入口是每个组件自己的 URL。`http://127.0.0.1:5173/` 会自动落到某个组件 URL，本身不作为独立入口。

## 组件 URL 规则

每个组件的 `component-id` 由组件名转换为 kebab case：

```text
AwakenComposerBar -> awaken-composer-bar
AwakenExecutionFeed -> awaken-execution-feed
AwakenLightInspectorRail -> awaken-light-inspector-rail
```

已注册组件 URL 的唯一事实源是：

```text
Awaken Design System/gallery/registry.ts
```

## 组件文件夹契约

每个组件都应该独立放在自己的文件夹里：

```text
src/components/AwakenComponentName/
├── AwakenComponentName.tsx
├── AwakenComponentName.css
├── AwakenComponentName.states.ts
├── AwakenComponentName.demo.tsx
├── README.md
└── index.ts
```

文件职责：

- `*.tsx`：组件实现。
- `*.css`：组件局部样式入口。
- `*.states.ts`：可观测状态与 mock props。
- `*.demo.tsx`：由 Gallery 渲染的独立视觉 demo。
- `README.md`：给人和 Coding Agent 看的组件契约。
- `index.ts`：组件导出。

## Coding Agent 规则

- 组件必须保持 UI-only：不能调用 runtime、Electron IPC、AG-UI、A2UI、SSE、本地文件系统或 `window.awaken`。
- 数据通过 props 进入，行为通过 callbacks 传出。
- 组件 props 或视觉状态变化时，必须同步更新可观测 states。
- 每个新组件 demo 都必须注册到 `gallery/registry.ts`。
- Desktop 应优先通过 `@awaken/design-system` 包出口使用组件，不要从 `Awaken Design System/src/...` 做源码路径 import。
- 一个组件只有能通过自己的 `#/components/<component-id>` URL 打开时，才算完成。

## 验证

修改组件或 tokens 后运行：

```bash
npm --workspace @awaken/design-system run typecheck
npm --workspace @awaken/design-system run smoke:render
```

`smoke:render` 会 server-render 所有导出的 `Awaken*Demo` 组件。如果 demo 缺失或渲染失败，命令会失败。
