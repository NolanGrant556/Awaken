# Awaken 项目文件清单

本文档记录当前 Awaken 仓库的主要代码目录、文件职责和常用验证命令。它用于帮助 0 上下文 Coding Agent 或新接手开发者快速理解项目结构。

更新时间口径：基于当前仓库结构，`packages/ui` 已迁移为顶层 `Awaken Design System`，Desktop 通过 `@awaken/design-system` 包出口消费组件。

## 1. 当前状态

| 项目 | 当前情况 |
| --- | --- |
| 主分支 | `main` |
| UI 包 | `Awaken Design System/` |
| UI 包名 | `@awaken/design-system` |
| Desktop 主界面 | `apps/desktop/src/renderer/App.tsx` |
| 组件数量 | 62 个 `Awaken*` 组件 |
| 推荐验证 | `npm run typecheck:ui`、`npm run typecheck:desktop`、`npm run smoke:ui` |

## 2. 顶层结构

```text
Awaken/
├── Awaken Design System/      # UI-only 组件系统
├── apps/
│   └── desktop/               # Electron Desktop 应用
├── cmd/
│   └── awaken-core/           # 本地 runtime HTTP/SSE 进程
├── src/                       # Go Agent Core / runtime / tools / server
├── docs/                      # 当前文档与历史归档
├── scripts/                   # 构建与验收脚本
├── config.toml                # 非敏感 LLM/model 配置
├── package.json               # npm workspace 入口
├── package-lock.json
├── go.mod
└── AGENTS.md
```

一句话理解：

```text
Awaken Design System = UI-only 组件库
apps/desktop = 当前真实产品界面
src + cmd/awaken-core = 本地 Agent runtime
```

## 3. Awaken Design System

路径：

```text
Awaken Design System/
```

职责：

Awaken 的 UI-only 组件系统。它包含可复用 React 组件、CSS tokens、组件局部样式、可观测 demo states 和本地 Gallery。它不能依赖 Go runtime、Electron、AG-UI、A2UI、SSE、本地文件系统或 `window.awaken`。

### 3.1 目录

```text
Awaken Design System/
├── src/
│   ├── components/            # 62 个 Awaken* 独立组件目录
│   ├── hooks/                 # UI-only React hooks
│   ├── tokens/                # 全局设计令牌与组件基础样式
│   ├── assets/                # 设计系统可复用静态资源
│   ├── index.ts               # 包主出口
│   ├── types.ts               # 设计系统共享类型
│   └── vite-env.d.ts
├── gallery/                   # 本地组件观测服务源码
├── smoke/                     # 自动化渲染冒烟检查
├── dist/                      # 构建产物
├── index.html                 # Gallery HTML 入口
├── package.json               # @awaken/design-system 包配置
├── vite.config.ts
├── tsconfig.json
└── tsconfig.build.json
```

### 3.2 核心文件

| 文件 | 功能 |
| --- | --- |
| `src/index.ts` | 设计系统主出口，Desktop 和外部项目应从这里导入。 |
| `src/components/index.ts` | 所有组件出口。 |
| `src/types.ts` | 共享 UI 类型。 |
| `src/tokens/tokens.css` | 全局设计 token。 |
| `src/tokens/components.css` | 组件基础样式。 |
| `src/hooks/useAwakenAutoScrollTail.ts` | 自动滚动到运行流尾部的 UI hook。 |
| `src/hooks/useAwakenPlatform.ts` | 平台上下文 hook。 |
| `gallery/registry.ts` | 组件 URL 与 demo 注册表。 |
| `gallery/AwakenGalleryApp.tsx` | Gallery 壳层与 hash 路由。 |
| `gallery/main.tsx` | Gallery Vite 入口。 |
| `gallery/styles.css` | Gallery 专用样式。 |
| `smoke/render-smoke.mjs` | Server-render 所有 `Awaken*Demo` 的冒烟检查。 |
| `package.json` | `@awaken/design-system` 包配置、脚本和依赖声明。 |

### 3.3 单个组件目录格式

每个组件应独立放在自己的文件夹里：

```text
src/components/AwakenComponentName/
├── AwakenComponentName.tsx        # 组件实现
├── AwakenComponentName.css        # 组件局部样式
├── AwakenComponentName.states.ts  # 可观测状态 / mock props
├── AwakenComponentName.demo.tsx   # Gallery demo
├── README.md                      # 组件说明
└── index.ts                       # 组件导出
```

### 3.4 当前组件清单

```text
AwakenActivityLine
AwakenAppShell
AwakenApprovalCard
AwakenArtifactCard
AwakenArtifactGrid
AwakenAssistantMessage
AwakenAvatar
AwakenBrandRow
AwakenButton
AwakenCenterStage
AwakenComposer
AwakenComposerBar
AwakenComposerDock
AwakenComposerInput
AwakenContextButton
AwakenConversationList
AwakenCopyButton
AwakenDisclosure
AwakenDivider
AwakenDropdown
AwakenExecutionFeed
AwakenExploringGroup
AwakenFileChangeSummary
AwakenIcon
AwakenIconButton
AwakenInspectorRail
AwakenInspectorTabs
AwakenKbd
AwakenLightInspectorRail
AwakenMarkdownAnswer
AwakenModeToggle
AwakenModelSelector
AwakenNarrativeFeed
AwakenNewTaskButton
AwakenPill
AwakenPlatformProvider
AwakenPopover
AwakenPrimaryNav
AwakenProfileButton
AwakenProjectTree
AwakenPromptCard
AwakenQuickActionChip
AwakenQuickActionGrid
AwakenRunStage
AwakenRunStep
AwakenRunTimeline
AwakenSendButton
AwakenSidebar
AwakenStatusPill
AwakenThinkingBlock
AwakenThoughtTrace
AwakenToast
AwakenToolActionRow
AwakenToolTraceGroup
AwakenToolTraceLine
AwakenTooltip
AwakenTopBar
AwakenUserPromptBlock
AwakenWindowTitleBar
AwakenWorkbenchChrome
AwakenWorkbenchShell
AwakenWorkbenchSidebarFrame
```

## 4. Desktop 应用

路径：

```text
apps/desktop/
```

职责：

Electron + React 桌面端。它是 Awaken 当前真实产品入口，负责启动本地 `awaken-core`、通过 preload bridge 调 runtime，并把事件渲染成 Run Workbench。

### 4.1 目录

```text
apps/desktop/
├── src/
│   ├── main/
│   ├── preload/
│   ├── renderer/
│   └── shared/
├── resources/
├── index.html
├── package.json
├── vite.config.ts
├── vitest.config.ts
├── tsconfig.json
├── tsconfig.node.json
└── electron-builder.yml
```

### 4.2 `src/main/`

| 文件 | 功能 |
| --- | --- |
| `index.ts` | Electron main 入口；创建窗口、注册 IPC、转发 SSE。 |
| `runtime-supervisor.ts` | 启动、停止、重启 `awaken-core` binary。 |
| `config-bootstrap.ts` | 初始化本地 `config.toml` 和 `.env`。 |
| `runtime-supervisor.test.ts` | 测 runtime supervisor。 |
| `config-bootstrap.test.ts` | 测配置初始化。 |

### 4.3 `src/preload/`

| 文件 | 功能 |
| --- | --- |
| `index.ts` | 暴露 `window.awaken` bridge。 |

### 4.4 `src/shared/`

| 文件 | 功能 |
| --- | --- |
| `agui.ts` | Desktop 共享 AG-UI event 类型。 |
| `runtime.ts` | runtime wire type 与 Desktop camelCase type 转换。 |

### 4.5 `src/renderer/`

| 文件 | 功能 |
| --- | --- |
| `App.tsx` | Desktop 主产品界面：Run Workbench。 |
| `main.tsx` | React renderer 入口；导入 design-system tokens。 |
| `styles.css` | Desktop 专属样式。 |
| `workbenchPresentation.ts` | approval、diff、composer 展示逻辑。 |
| `App.test.ts` | App 行为测试。 |

### 4.6 `src/renderer/runtime/`

| 文件 | 功能 |
| --- | --- |
| `client.ts` | Runtime client；支持 Electron bridge 和 browser fallback。 |
| `reducer.ts` | 把 run、thread、AG-UI event 归约成 UI 状态。 |
| `agui.ts` | renderer runtime 使用的 AG-UI 类型再导出。 |
| `client.test.ts` | 测 runtime client。 |
| `reducer.test.ts` | 测 reducer。 |

### 4.7 `src/renderer/run-narrative/`

| 文件 | 功能 |
| --- | --- |
| `types.ts` | Run Narrative 状态和 item 类型。 |
| `projector.ts` | 把 AG-UI event 投影成 prompt、thinking、tool、approval、file、answer。 |
| `index.ts` | run-narrative 出口。 |

## 5. Go Runtime

路径：

```text
src/
```

职责：

Agent 执行核心、LLM provider、工具系统、runtime session、HTTP/SSE server、AG-UI 协议适配。

### 5.1 `src/agentcore/`

| 文件 | 功能 |
| --- | --- |
| `agent.go` | Agent 门面。 |
| `runner.go` | Agent 执行主循环。 |
| `tool_runtime.go` | 工具执行、权限、approval、artifact 事件。 |
| `request_builder.go` | 构建 LLM request。 |
| `action_parser.go` | XML fallback 工具调用解析。 |
| `events.go` | Awaken 内部事件模型。 |
| `event_sink.go` | 事件 sink 和 approval awaiter。 |
| `types.go` | Agent 配置和接口类型。 |
| `*_test.go` | agentcore 测试。 |

### 5.2 `src/context/`

| 文件 | 功能 |
| --- | --- |
| `context.go` | conversation、message、tool call 数据结构。 |
| `tool_pairing.go` | 校验 tool call 与 tool result 配对。 |
| `context_test.go` | 测上下文和工具配对。 |

### 5.3 `src/llm/`

| 文件 | 功能 |
| --- | --- |
| `client.go` | LLM 配置加载与统一调用入口。 |
| `types.go` | LLM 请求、响应、tool call、stream 类型。 |
| `openai_chat.go` | OpenAI Chat 协议实现。 |
| `anthropic_messages.go` | Anthropic Messages 协议实现。 |
| `config_file.go` | 解析 `config.toml`。 |
| `errors.go` | LLM 错误。 |
| `test-llm.go` | 手动 LLM 连通性测试。 |
| `*_test.go` | LLM 协议测试。 |

### 5.4 `src/prompt/`

| 文件 | 功能 |
| --- | --- |
| `templates/default_system.md` | 默认 system prompt。 |
| `loader.go` | 加载 prompt 模板。 |
| `runtime.go` | 生成 runtime info。 |
| `system_prompt.go` | 注入 runtime info。 |
| `effective.go` | 构造最终 prompt 的便捷函数。 |
| `system_prompt_test.go` | prompt 测试。 |

### 5.5 `src/runtime/`

| 文件 | 功能 |
| --- | --- |
| `run_service.go` | 管理 run、thread、workspace、cancel、approval。 |
| `session.go` | 单个 run 状态机和事件记录。 |
| `event_bus.go` | 支持历史回放的事件总线。 |
| `*_test.go` | runtime 测试。 |

### 5.6 `src/server/`

| 文件 | 功能 |
| --- | --- |
| `router.go` | 注册 HTTP 路由。 |
| `handlers.go` | 处理 runs、threads、events、cancel、approval。 |
| `sse.go` | SSE 编码。 |
| `runtime_info.go` | 暴露非敏感 runtime 信息。 |
| `server_test.go` | API、SSE、smoke 测试。 |

### 5.7 `src/protocol/`

| 路径 | 功能 |
| --- | --- |
| `agui/events.go` | AG-UI-compatible event 类型。 |
| `agui/mapper.go` | AwakenEvent -> AG-UI event。 |
| `agui/encoder.go` | JSON/SSE 编码。 |

### 5.8 `src/tool/`

| 文件 | 功能 |
| --- | --- |
| `tool.go` | 工具接口定义。 |
| `schema.go` | LLM tool schema。 |
| `registry.go` | 工具注册、查询、执行。 |
| `permission.go` | workspace / writable roots 权限模型。 |
| `*_test.go` | tool 测试。 |

### 5.9 `src/tools/`

| 路径 | 功能 |
| --- | --- |
| `tools.go` | 注册全部内置工具。 |
| `calculator/` | 数学计算。 |
| `file_read/` | 安全文本读取。 |
| `list_files/` | 文件列表。 |
| `grep_search/` | 正则搜索。 |
| `shell/` | shell 命令执行。 |
| `web_search/` | Web 搜索。 |
| `apply_patch/` | Codex patch 解析与执行。 |
| `internal/filematch/` | glob 匹配。 |
| `internal/filesafe/` | 二进制和噪声目录判断。 |

## 6. Runtime 入口

路径：

```text
cmd/awaken-core/
```

| 文件 | 功能 |
| --- | --- |
| `main.go` | 本地 runtime HTTP/SSE 服务入口。 |
| `main_test.go` | 默认 workspace 测试。 |

## 7. 脚本

路径：

```text
scripts/
```

| 文件 | 功能 |
| --- | --- |
| `build-binary.mjs` | 构建 `awaken-core` binary。 |
| `ensure-desktop-runtime.mjs` | 确保 Desktop runtime binary 存在。 |
| `open-desktop.mjs` | Awaken Desktop 正式启动入口，复用或启动受管 screen 会话并检查真实窗口。 |
| `close-desktop.mjs` | Awaken Desktop 正式关闭入口，停止受管 screen 会话并清理入口进程。 |
| `accept-desktop-hardening.mjs` | Desktop 发布前硬化验收。 |

## 8. 常用命令

```bash
npm run open:desktop
npm run close:desktop
npm run dev:ui

npm run typecheck:ui
npm run typecheck:desktop
npm run smoke:ui

npm run build:desktop
go test ./...
```

## 9. 改动边界提醒

- 改 Desktop 产品界面：优先看 `apps/desktop/src/renderer/App.tsx`。
- 改共享组件：优先看 `Awaken Design System/src/components/<ComponentName>/`。
- 改设计 token：看 `Awaken Design System/src/tokens/`。
- 改 runtime/API/SSE：看 `src/runtime`、`src/server`、`src/protocol`。
- 改工具执行和权限：看 `src/tool`、`src/tools`、`src/agentcore/tool_runtime.go`。
- `Awaken Design System` 必须保持 UI-only，不应直接依赖 runtime、Electron、AG-UI、A2UI、SSE 或 `window.awaken`。
