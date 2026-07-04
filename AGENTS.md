# AGENTS.md

## 上下文管理工程

本项目的 `docs/` 是 Awaken 的上下文管理工程：它不是普通资料堆放区，而是给 Coding Agent 和后续接手者按功能域读取上下文的档案库。根目录 `README.md` 是项目入口，根目录 `AGENTS.md` 是行为约束入口；具体 PRD、架构、约束、验收、历史切片与修复记录，应沉淀到对应功能域目录。

当前 `docs/` 目录树：

```text
docs/
├── 01_桌面工作台/
├── 02_主智能体闭环/
├── 03_运行时与本地服务/
├── 04_工具权限与审批/
├── 05_模型与供应商配置/
├── 06_MCP运行时/
├── 07_连接器/
├── 08_技能/
├── 09_AGUI与A2UI协议/
└── 10_历史资料/
```

每新增一个功能或架构域，必须先在 `docs/` 下开对应中文功能档案目录，再沉淀该功能需要的 PRD、架构说明、编码约束、验收流程、实施计划、复核清单或历史记录。不要把新功能文档散落到临时目录、工具目录或根目录。

## 文档命名标准

新增或整理项目文档时，Markdown 文档文件名和承载文档的文件夹名尽量使用中文，优先使用能直接说明内容、状态和范围的中文名称。每个功能或架构域独立开一份文件夹，不按 PRD、验收、约束这类文档类型把同一功能拆散。

除 `README.md`、`AGENTS.md`、`SKILL.md`、代码内 prompt 模板等生态或工具强约定名称外，不要新建英文文档名、英文文档目录名或拼音缩写目录名。已有英文约定路径不要在无明确整理任务时顺手重命名。

## 测试与验收

Awaken 的 runtime、agent loop、工具执行、approval、SSE、模型配置等真实产品路径，验收时必须按根 `README.md` 的入口契约打开 Awaken Desktop，并在 Desktop UI 中发送真实任务完成 smoke 或 acceptance 验证。

由于当前 LLM API 配置已经更改，旧的直接 provider/API smoke 不再代表实际产品可用性；除非任务只要求验证底层库或纯函数，否则不要把绕过 Desktop UI 的 LLM 调用当成最终验收。

## 前端 Workbench 契约

涉及 Awaken Desktop 中间工作区（`AppShell -> WorkbenchShell -> RunStage -> ComposerDock / ExecutionFeed`）的布局、滚动、turn marker、composer、narrative/turn 渲染或相关样式改动时，先读 `/Users/nolangrant/Awaken/docs/01_桌面工作台/架构契约.md`，并按 `/Users/nolangrant/Awaken/docs/01_桌面工作台/复核清单.md` 复核。

中间区域现有设计与交互是产品契约：`RunStage` 是唯一滚动/定位/overlay/composer 容器，`ExecutionFeed` 只承载 narrative/turn，composer 形态目前只承认 `center` 与 `dock`。不要为了单个功能绕过这些边界；如确实需要改变契约，必须同步更新契约文档、补充相应测试，并提供真实运行截图或录屏作为验收证据。

## 打开 / 启动 Awaken

启动规则以 `/Users/nolangrant/Awaken/README.md` 的“入口契约”和“本地运行”为唯一来源。Coding Agent 需要打开、检查、诊断 Awaken 启动状态时，先读根 README，不要在 `AGENTS.md` 里维护第二套命令。

当用户说“打开 Awaken”时，默认含义、正式入口、状态检查、持续运行方式和成功确认标准，都按根 README 执行。除非用户明确说“打开设计系统”或“打开组件库”，不要切到组件 Gallery 入口。
