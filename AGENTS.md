# AGENTS.md

## 测试与验收

测试、验收可以调用真实 LLM。对 Awaken 的 runtime、agent loop、工具执行、approval、SSE 等真实产品路径，缺少 LLM 参与的验证通常不能代表实际可用性；需要使用真实 provider 做 smoke 或 acceptance 验证。

## 前端 Workbench 契约

涉及 Awaken Desktop 中间工作区（`AppShell -> WorkbenchShell -> RunStage -> ComposerDock / ExecutionFeed`）的布局、滚动、turn marker、composer、narrative/turn 渲染或相关样式改动时，先读 `/Users/nolangrant/Awaken/docs/frontend/workbench-contract.md`，并按 `/Users/nolangrant/Awaken/docs/frontend/review-checklist.md` 复核。

中间区域现有设计与交互是产品契约：`RunStage` 是唯一滚动/定位/overlay/composer 容器，`ExecutionFeed` 只承载 narrative/turn，composer 形态目前只承认 `center` 与 `dock`。不要为了单个功能绕过这些边界；如确实需要改变契约，必须同步更新契约文档、补充相应测试，并提供真实运行截图或录屏作为验收证据。

## 打开 / 启动 Awaken

启动规则以 `/Users/nolangrant/Awaken/README.md` 的“入口契约”和“本地运行”为唯一来源。Coding Agent 需要打开、检查、诊断 Awaken 启动状态时，先读根 README，不要在 `AGENTS.md` 里维护第二套命令。

当用户说“打开 Awaken”时，默认含义、正式入口、状态检查、持续运行方式和成功确认标准，都按根 README 执行。除非用户明确说“打开设计系统”或“打开组件库”，不要切到组件 Gallery 入口。
