# Awaken 对齐 Claude Code 七类基础工具的重构开发计划

> 状态：开发计划（调研结论已逐条对照 Awaken 与 Claude Code 源码复核）
>
> 日期：2026-07-16
>
> 对照源码：`/Users/nolangrant/claudecode源代码`
> Awaken 仓库：`/Users/nolangrant/Awaken`

## 一、结论摘要

本次目标不是把 Awaken 的全部工具系统缩减成 Claude Code 的七个工具，也不是把 Claude Code 的工具名称、权限系统或产品品牌原样移植进 Awaken。

正确的改造边界是：

- 仅重构 Awaken 内置的七类基础工具，使其职责、输入协议、执行语义和结果结构对齐 Claude Code 的成熟设计；
- 对外名称使用 Awaken 自己的 `Bash`、`Read`、`Edit`、`Write`、`Glob`、`Grep`、`WebSearch`；
- 七类工具必须兼容 Windows，与 Claude Code 一致（默认 bash 语义、Windows 优先 Git Bash），并复用 Awaken 现有的跨平台宿主 shell 解析设计；
- 保留 Awaken 当前“无需审批、直接执行”的工具策略，不复制 Claude Code 的权限询问、沙箱审批和危险操作确认系统；同时删除 Awaken 自己当前未使用的审批设计残留（审批卡 metadata、Shell 的可选 restricted/staging 路径，见 2.4 边界清单；typed 工具依赖的 `WorkspaceExecutionBroker` 是安全合同，保留）；
- `Read` 的多模态返回能力由当前所选模型的显式能力决定，而不是写死在工具中，也不能通过模型名称猜测；
- 现有 MCP、Skills 及 `calculator`、`ask_user` 等其他内置工具不在本次替换范围内；
- 产物生成路线转向 Claude Code 设计：Word/Excel/PPT/PDF 等产物由 Bash + 脚本/Skills 生成，`apply_patch` 与 Office typed artifact 生成运行时确认退役，但退役在七工具切换验证稳定之后分阶段执行，不混入原子切换；
- Workbench 最终交付物卡片是核心产品功能，必须保留：`artifact_publish`、`tool.Artifact` 契约、`ArtifactCreated/ArtifactUpdated` 事件与 AG-UI 投影链路长期保留，Bash/Skills 生成的文件通过 `artifact_publish` 显式发布为可点击打开的交付物；
- Office 产物 Skills 由用户在七工具转型完成后自行建设和测试，不属于本计划的交付与验收范围；
- 保持 `src/tools` 中“一种工具一个文件夹”的现有整洁结构。

最终形态是：七个新基础工具 + MCP + Skills + 少量保留内置工具（`artifact_publish`、`trash_file`、`calculator`、`ask_user`、`dependency_install`、`git_remote`），Office 产物生成能力由用户后续以 Skills 承载，交付物发布仍走 `artifact_publish`。

## 二、本次范围边界

### 2.1 纳入重构的工具

| Awaken 目标工具 | 主要职责 | 参考 Claude Code 设计 |
| --- | --- | --- |
| `Bash` | 执行 shell 命令、维护会话工作目录、处理长输出和后台任务 | `BashTool` |
| `Read` | 读取文本、图片和 PDF，并按模型能力返回多模态内容 | `FileReadTool` |
| `Edit` | 对已读取文件执行精确文本替换 | `FileEditTool` |
| `Write` | 新建文件或完整覆写已读取文件 | `FileWriteTool` |
| `Glob` | 按 glob 模式定位文件 | `GlobTool` |
| `Grep` | 使用 ripgrep 搜索文件内容 | `GrepTool` |
| `WebSearch` | 搜索互联网并返回结构化来源 | `WebSearchTool` |

### 2.2 确认退役、但不混入七工具原子切换的部分

以下三块已确认按 Claude Code 路线退役，但必须在七工具切换验证稳定之后按独立阶段执行（见阶段七、阶段八、阶段九），不得与七工具原子切换混在同一次变更：

- `apply_patch`：功能被 `Edit`/`Write`/`Bash`/`trash_file` 组合完全覆盖，Claude Code 本身没有等价工具；但它当前被 `src/` 下约 50 个文件引用，包括 `agentcore/auto_tool_policy.go` 的零审批 file-chain 策略、`executors/file_chain.go`、`tool/profile/intent.go`、audit、prompt 模板与回放 testdata，退役是一次独立的策略层重构；
- Office typed artifact 生成运行时：`document_create/edit`、`spreadsheet_create/edit`、`presentation_create/edit/render`、`office_export_pdf`、`pdf_extract`、`image_inspect`、`artifact_render`、`artifact_export`、`runtime_preflight`、`workspace_dependencies`、`read_artifact` 及 `src/runtime/officerender` 的 Office worker。产物生成改由 Bash + 脚本/Skills 承载，与 Claude Code 一致。注意：退役的是“生成产物的 typed 工具”，不是交付物发布链路——`artifact_publish` 与 Workbench 交付物卡片是核心产品功能，明确保留（见 2.3）；
- 当前未使用的审批设计残留：审批卡 metadata 链路（`tool.DecisionMetadata` 的 Title/Description/RiskTone、`AllowPermissionWithMetadata`/`DenyPermissionWithMetadata`、shell 的 `shellApprovalDescription` 与 ShellLane 审批文案），以及 Shell 的可选 restricted/staging 执行路径（supervised execution、`ErrExecutionSandboxRequired`/`ErrExecutionTransformRequired` 等 shell outcome 分支）。产品策略是零审批直接执行，这些机制没有真实使用面。注意边界：`WorkspaceExecutionBroker`（`tool/execution_broker*.go`）本身不退役——`dependency_install`、`git_remote` 和 MCP artifact bridge 在默认路径上依赖它做暂存、校验、Trash 与原子提交，这是活跃的安全合同，不是审批（已确认决策：保留现有 broker，只删 Shell 可选路径与审批 metadata；不做“抽成 typed staging”的改名重构）。

### 2.3 明确不在本次范围内

以下部分不得因本计划被删除、合并、改名或改写：

- Workbench 最终交付物卡片全链路（核心产品功能，明确保留）：`artifact_publish` 工具、`tool.Artifact` 事实契约、`ArtifactCreated/ArtifactUpdated` 语义事件、AG-UI artifact 投影、Desktop renderer 的 deliverable 卡片与点击打开行为，以及 MCP artifact metadata bridge；
- Awaken 现有 MCP 客户端、MCP 工具注册、MCP 服务发现与调用协议；
- Awaken 现有 Skills 的发现、安装、目录结构、运行时与提示词注入设计（Office 产物 Skills 化正是依赖这套体系）；
- `calculator`、`ask_user`、`dependency_install`、`git_remote`、`trash_file` 等其他内置工具；
- 安全验证不变量（不是审批，不得随审批设计一起删除）：workspace boundary、writable/output roots、敏感路径拒绝、trash 可恢复删除、guardian 只读约束、`auto_tool_policy` 的 allow/deny 决策本身；
- 与本计划无直接关系的 Desktop、协议和业务模块。

### 2.4 退役边界清单

退役工作以下表为准，逐项核对，避免“顺手多删”或“漏删残留”：

| 对象 | 处置 | 阶段 |
| --- | --- | --- |
| `src/tools/shell`、`src/tools/file_read`、`src/tools/text_edit`、`src/tools/text_file_create`、`src/tools/list_files`、`src/tools/grep_search` | 被七工具定点替换后删除 | 阶段六 |
| `src/agentcore/shell_transform.go`（shell→apply_patch 自动变形） | 停用并删除 | 阶段六 |
| `src/tools/apply_patch` 及全部引用（策略、prompt、testdata） | 删除，策略不变量迁移到 Edit/Write/Bash | 阶段七 |
| `src/tools/artifact_runtime`、`src/tools/read_artifact`、`src/runtime/officerender` | 删除 | 阶段八 |
| `src/tools/artifact_publish`、`tool.Artifact` 契约、`ArtifactCreated/ArtifactUpdated` 事件、AG-UI artifact 投影、Desktop 交付物卡片、MCP artifact bridge | 不动（核心产品功能：交付物发布链路） | — |
| `tool.DecisionMetadata`（Title/Description/RiskTone/PatchSummary）、`AllowPermissionWithMetadata`、`DenyPermissionWithMetadata`、`DecisionMetadataDescribingTool`、shell 的审批文案与 wire/renderer 中对应字段 | 删除 | 阶段九 |
| Shell 的可选 restricted/staging 执行路径与对应 outcome 分支（supervised execution、`ErrExecutionSandboxRequired`/`ErrExecutionTransformRequired` 等）、shell 结果中的 `broker_receipt` 暴露 | 删除 | 阶段九 |
| `WorkspaceExecutionBroker`（`tool/execution_broker*.go`）及 `dependency_install`、`git_remote`、MCP artifact bridge 的 staged 执行合同 | 不动（活跃安全合同，非审批） | — |
| MCP、Skills、`trash_file`、`calculator`、`ask_user`、`dependency_install`、`git_remote` | 不动 | — |
| workspace boundary、敏感路径、trash 可恢复、guardian 只读、allow/deny 决策 | 不动（安全不变量） | — |
| Word/Excel/PPT 产物 Skills 的建设与测试 | 用户自行完成，不在本计划内 | — |

这意味着本次开发不能借“对齐 Claude Code”之名，一次性重做 Awaken 的整个工具池或工具注册框架；收敛是分阶段、可验证的，每个阶段结束时产品完整可用。

## 三、Claude Code 源码调研结论

### 3.1 目录与公共工具契约

Claude Code 的七类工具各自拥有独立目录，例如 `BashTool`、`FileReadTool`、`FileEditTool`、`FileWriteTool`、`GlobTool`、`GrepTool` 和 `WebSearchTool`。这一组织方式与 Awaken 当前 `src/tools` 下“一种工具一个文件夹”的结构相容，不需要破坏 Awaken 已经工整的目录设计。

Claude Code 的核心价值不只是工具名称，而是统一工具生命周期：

1. 定义输入 schema；
2. 校验输入；
3. 执行工具；
4. 生成进度；
5. 映射模型结果与用户展示结果；
6. 声明是否只读、是否允许并发；
7. 对执行错误和截断结果使用稳定结构。

主要源码入口：

- [`src/Tool.ts`](/Users/nolangrant/claudecode源代码/src/Tool.ts)
- [`src/services/tools/toolExecution.ts`](/Users/nolangrant/claudecode源代码/src/services/tools/toolExecution.ts)

Awaken 应复制这类“职责清楚、结果可组合”的工具契约思路，但不应复制 Claude Code 的品牌命名和整套执行外围。

### 3.2 不应复制的 Claude Code 设计

以下机制与 Awaken 已确认的产品策略冲突，不应移植：

- `checkPermissions` 等逐工具审批路径；
- 沙箱授权和危险命令二次确认；
- `dangerouslyDisableSandbox` 一类绕过沙箱的模型参数；
- Claude Code 自身的 Skill 发现、MCP 通知和 VS Code 集成钩子；
- Anthropic 专属品牌文案、模型判断和服务端工具耦合；
- 把所有七类工具强制绑定到 Anthropic API 的实现方式。

Awaken 应保持自己的模型供应商抽象、会话运行时和零审批策略。

## 四、逐工具目标设计

## 4.1 Bash

### Claude Code 的关键设计

Claude Code `BashTool` 的主要输入包括：

- `command`
- `timeout`
- `description`
- `run_in_background`
- `dangerouslyDisableSandbox`

值得吸收的不是最后一个危险参数，而是以下行为：

- 一个 Bash 调用执行一段完整命令；
- 会话拥有逻辑工作目录，`cd` 的结果可以影响后续调用；
- 只读命令可以按策略并发；
- 长时间命令提供持续进度；
- 大输出具有明确截断、持久化或后续读取路径；
- 前台和后台执行的结果语义分开。

Windows 侧的对应设计（源码已核实）：

- Claude Code 的默认 shell 在所有平台都是 bash，Windows 上通过 Git Bash 执行（`resolveDefaultShell.ts` 明确“不自动切到 PowerShell”），并做 Windows/POSIX 路径互转和 `2>nul` 重定向纠偏（`bashProvider.ts`）；
- `PowerShellTool` 是独立的 Windows-only 工具，由 `isPowerShellToolEnabled()` 按平台加环境变量开关决定是否注册，不与 `BashTool` 混合在一个工具里。

### Awaken 的目标设计

建议新建 `src/tools/bash`，对模型暴露 `Bash`，并替换现有 `shell`。

建议输入：

```json
{
  "command": "rg --files src",
  "timeout": 120000,
  "description": "列出 src 中的文件",
  "run_in_background": false
}
```

Awaken 特有约束：

- 保留当前登录用户身份和无需审批的直接执行策略；
- 不暴露 `dangerouslyDisableSandbox`；
- 工作目录归属于 thread/session，不得使用进程级全局 `chdir`；
- 明确超时、退出码、stdout、stderr、截断和后台任务标识；
- 不再把普通 shell 命令自动变形成另一种工具调用。

Windows 兼容（已确认为本次范围内要求）：

重构前 Awaken 的 `shell` 已具备跨平台宿主 shell 解析基础；实现后，这部分能力迁入 [`src/tool/host_environment.go`](/Users/nolangrant/Awaken/src/tool/host_environment.go) 与 [`src/tools/bash/bash.go`](/Users/nolangrant/Awaken/src/tools/bash/bash.go)。新 `Bash` 工具复用这条解析链，不重写第二套平台适配。

与 Claude Code 对齐的目标行为：

- Windows 上 `Bash` 优先解析 Git Bash（`bash.exe`），与 Claude Code “默认 shell 在所有平台都是 bash”的策略一致；找不到 Git Bash 时按 Awaken 现有链路回退到 `pwsh`/`powershell`/`cmd.exe`，并在结果中如实报告实际使用的 shell；
- 本次不新增独立的 PowerShell 工具（Claude Code 的 `PowerShellTool` 是 Windows-only、按环境变量开关的可选工具，不属于七类基础工具）；
- Windows 路径与 POSIX 路径的互转、`2>nul` 一类 CMD 风格重定向的纠偏，参考 Claude Code `bashProvider.ts` 的做法，只在确认落入 Git Bash 执行时应用；
- 按根 README 的现状，本仓库可复核的真实 Desktop 验收基线只覆盖 macOS；Windows 行为以聚焦测试和 `runtime.GOOS` 分支单测为最低验证，实机验收作为已知缺口如实报告。

重构前 `src/agentcore/shell_transform.go` 中存在 shell 到 `apply_patch` 的转换行为。为获得真实、可预测的 Bash 语义，七工具切换时停用并删除这条自动转换链路；`apply_patch` 工具本身按阶段七独立退役。

## 4.2 Read

### Claude Code 的关键设计

Claude Code `FileReadTool` 的输入包括：

- `file_path`
- `offset`
- `limit`
- `pages`

它不是单纯返回字符串，而是能产生文本、图片、PDF、Notebook、复合内容块和“文件未变化”等不同结果。对本次 Awaken 改造最重要的能力有：

- 文本支持分页读取；
- PNG、JPEG、GIF、WebP 等图片可以返回原生图片内容块；
- PDF 在模型原生支持时返回文档块；
- 模型不支持原生 PDF、但支持视觉时，将指定 PDF 页渲染为图片；
- 单次 PDF 读取设置页数上限，避免无界上下文膨胀；
- 会话维护文件读取状态，重复读取未变化文件时可以返回 `file_unchanged`；
- 读取状态同时为后续 Edit/Write 提供安全前置条件。

### Awaken 的目标设计

建议新建 `src/tools/read`，对模型暴露 `Read`，替换现有 `read_file`（注意：现有工具目录是 `src/tools/file_read`，但对模型暴露的名称是 `read_file`，两者不同，切换与验收都必须以 `read_file` 为准）。

建议输入：

```json
{
  "file_path": "/absolute/path/to/file.pdf",
  "offset": 1,
  "limit": 2000,
  "pages": "1-5"
}
```

目标结果应区分：

- `text`
- `image`
- `document`
- `parts`
- `file_unchanged`
- `unsupported_media`

`Read` 不应自行根据 `kimi`、`claude` 等模型名称决定返回格式。它只读取当前运行上下文传入的显式能力。（Claude Code 自己的 `isPDFSupported()` 恰恰是按模型名排除 `claude-3-haiku` 实现的，这是要改进而不是照搬的部分。）

PDF 页渲染的系统依赖必须明确：Claude Code 的页渲染路径依赖 poppler-utils 的 `pdftoppm`，缺失时返回带安装指引的明确错误。根 README 已冻结“Desktop 启动不检查或安装 Poppler”，因此 Awaken 的 `Read` 在页渲染依赖缺失时必须返回同样明确的 `unsupported_media` 或依赖缺失错误（附安装建议），不得静默降级为路径字符串，也不得因此阻塞其他文件类型的读取。

## 4.3 Edit

### Claude Code 的关键设计

Claude Code `FileEditTool` 的输入是：

- `file_path`
- `old_string`
- `new_string`
- `replace_all`

其重要约束包括：

- 编辑已有文件前，必须在当前会话中读取过该文件（Claude Code 只检查 `readFileState` 中存在该文件且 mtime 未过期，用 offset/limit 部分读取同样满足；不要求“完整读取”，否则超过单次读取 token 上限的大文件将永远无法编辑）；
- 默认要求 `old_string` 唯一匹配；
- 多处替换必须显式使用 `replace_all`；
- 写入前检查文件内容或修改时间，防止基于过期内容覆盖外部修改；
- 校验与写入应形成原子或等价的安全操作；
- 保持原文件编码和换行风格；
- 返回结构化 diff，而不是只有“成功”字符串。

### Awaken 的目标设计

建议新建 `src/tools/edit`，对模型暴露 `Edit`，替换现有 `text_edit`。

建议输入：

```json
{
  "file_path": "/absolute/path/to/file.go",
  "old_string": "old value",
  "new_string": "new value",
  "replace_all": false
}
```

当前 Awaken `text_edit` 使用模型提供的 `expected_sha256` 参与并发保护。新设计应改为由宿主维护 `Read` 产生的文件状态，模型不再负责计算或传递哈希。这样既能降低模型调用复杂度，也能把文件一致性责任放回运行时。

## 4.4 Write

### Claude Code 的关键设计

Claude Code `FileWriteTool` 的输入保持极简：

- `file_path`
- `content`

主要规则是：

- 新文件可以直接创建；
- 已存在文件必须先在当前会话中读取；
- Write 表示完整覆写，不混入 append、发布或 Artifact 生成语义；
- 覆写前同样需要检查读取状态是否过期。

### Awaken 的目标设计

建议新建 `src/tools/write`，对模型暴露 `Write`，替换现有 `text_file_create`。

建议输入：

```json
{
  "file_path": "/absolute/path/to/report.md",
  "content": "# Report\n"
}
```

现有 `text_file_create` 中的 append、publish 等字段不应进入新的 Write 协议：

- 追加文本使用 `Read + Edit` 或 `Read + Write`；
- Write 本身不携带任何发布或 Artifact 语义；
- 产物发布统一由长期保留的 `artifact_publish` 负责：Write/Bash/Skills 生成的文件要成为 Workbench 交付物卡片，必须显式调用 `artifact_publish`，系统提示词需写明这一步。

## 4.5 Glob

### Claude Code 的关键设计

Claude Code `GlobTool` 的主要输入是：

- `pattern`
- `path`（可选）

其行为特点是：

- 只负责按 glob 模式找文件，不搜索文件内容；
- 属于只读工具，可安全并发；
- 底层用 `rg --files --sort=modified` 实现，结果按修改时间排序（旧在前），不是稳定的路径排序；
- 有明确结果数量上限和截断说明（默认 100 条并返回 `truncated` 标志）；
- 默认包含隐藏文件且不遵守 .gitignore（可用环境变量翻转）。

### Awaken 的目标设计

建议新建 `src/tools/glob`，对模型暴露 `Glob`，替换现有 `list_files`。

建议输入：

```json
{
  "pattern": "src/**/*.go",
  "path": "/Users/nolangrant/Awaken"
}
```

结果应至少包含匹配路径、是否截断、匹配总量或已返回数量。默认上限可参考 Claude Code 的 100 条设计，但应作为 Awaken 自己的运行时常量和测试契约。排序策略由 Awaken 自行冻结：采用稳定的路径排序（现有 `list_files` 已是稳定排序），不照搬 Claude Code 的修改时间排序，避免同一查询在测试和回放中结果顺序漂移。

## 4.6 Grep

### Claude Code 的关键设计

Claude Code `GrepTool` 使用真实 ripgrep，而不是在 Go/JavaScript 中重新实现弱化版搜索。主要输入覆盖：

- `pattern`
- `path`
- `glob`
- `type`
- `output_mode`
- `-A`、`-B`、`-C`
- `-n`
- `-i`
- `head_limit`
- `offset`
- `multiline`

### Awaken 的目标设计

建议新建 `src/tools/grep`，对模型暴露 `Grep`，替换现有 `grep_search`。

现有参数可以按下表迁移：

| 现有 Awaken 参数 | 目标 Grep 参数 |
| --- | --- |
| `query` | `pattern` |
| `root` | `path` |
| `include` | `glob` |
| `max_results` | `head_limit` |
| `max_line_chars` | 运行时输出截断策略，不建议作为模型主参数 |

目标实现应尽量忠实透传 ripgrep 的搜索能力，同时由 Awaken 控制可执行文件发现、超时、输出大小和错误分类。

注意一处默认行为差异：Claude Code `GrepTool` 的 `output_mode` 默认是 `files_with_matches`（只返回文件路径），而 Awaken 现有 `grep_search` 默认返回匹配行（相当于 `content` 模式）。切换时必须显式冻结 Awaken 自己的默认值——建议默认 `content`，与现有模型使用习惯一致，并在 schema 描述中写明。

## 4.7 WebSearch

### Claude Code 的关键设计

Claude Code `WebSearchTool` 的主要输入是：

- `query`
- `allowed_domains`
- `blocked_domains`

其实现依赖 Anthropic 的 `web_search_20250305` 服务端工具，并通过嵌套模型调用组织结果。这一实现不能直接复制到多模型、多供应商的 Awaken。

### Awaken 的目标设计

保留现有 `src/tools/web_search` 目录，但重写并收敛其输入、结果和错误协议，对模型暴露 `WebSearch`。

建议输入：

```json
{
  "query": "Go os/exec CommandContext documentation",
  "allowed_domains": ["go.dev"],
  "blocked_domains": []
}
```

应该吸收的 Claude Code 设计包括：

- 简洁稳定的查询 schema；
- `allowed_domains` 和 `blocked_domains` 的互斥、格式校验；
- 返回带标题、URL、摘要的结构化来源；
- 明确搜索进度、无结果和供应商错误；
- 输出可被最终回答引用的来源信息。

应该保留的 Awaken 自有设计包括：

- 搜索供应商抽象；
- 多模型兼容；
- 无需审批执行；
- WebSearch 仍是 Awaken 内置工具，不转成 MCP，也不依赖 Anthropic 专属服务端工具。

## 五、Read 多模态与模型能力路由

### 5.1 Awaken 当前已具备的基础

当前 [`src/runtime/run_service.go`](/Users/nolangrant/Awaken/src/runtime/run_service.go) 的 `ModelProfileConfig` 已存在视觉和视频能力字段；模型配置中也已经能表达 `supports_vision`。因此，Kimi 等配置了视觉能力的模型已有能力元数据基础。

但当前链路仍存在关键断点：

- profile 转换为 agentcore 模型配置时，没有完整传递视觉能力；
- [`src/agentcore/types.go`](/Users/nolangrant/Awaken/src/agentcore/types.go) 的运行模型配置缺少完整的多模态能力表达；
- 工具结果通常通过 [`src/agentcore/runner.go`](/Users/nolangrant/Awaken/src/agentcore/runner.go) 的字符串结果路径回送模型；
- 当前 [`src/llm/openai_chat.go`](/Users/nolangrant/Awaken/src/llm/openai_chat.go) 与 [`src/llm/anthropic_messages.go`](/Users/nolangrant/Awaken/src/llm/anthropic_messages.go) 主要在用户附件入口处理图片，尚未形成通用的“工具结果多模态内容块”协议。

因此，“Kimi 配置支持视觉”不等于当前 `Read` 已经能把图片真正送入 Kimi。必须补齐从模型配置到工具结果、再到供应商消息适配器的整条链路。

### 5.2 目标能力模型

不要用模型名称、供应商名称或协议类型判断能力。建议至少显式表达：

```go
type ModelCapabilities struct {
    SupportsVision    bool
    SupportsNativePDF bool
}
```

`SupportsNativePDF` 进入 profile 的方式已确认决策：新增 `supports_native_pdf` 字段与 Desktop 高级开关，存量 profile 一律默认 `false`（此时视觉模型自动走 Poppler 页渲染兜底），只对显式打开的 profile 启用原生 PDF 文档块；内置模型注册表可为已知支持的条目（如 Anthropic 官方 Claude 模型）预置 `true`，这仍是显式配置。不按协议自动判断——OpenAI-compatible 网关后可能挂着支持原生 PDF 的模型，Anthropic 协议也可能接第三方模型，协议不等于模型能力。

如果后续还需要视频、音频或其他模态，可以继续扩展，但本次七工具重构只需要解决 Read 所需能力。

目标传播链路：

```text
Model Profile
  -> Agent ModelConfig
  -> Run/Tool Context
  -> Read result routing
  -> Provider message adapter
  -> Selected model
```

任何一层丢失能力字段，都会导致工具虽然读到了图片，模型却只收到路径或描述字符串。

### 5.3 建议的路由规则

| 文件类型 | 模型能力 | Read 返回给模型的内容 |
| --- | --- | --- |
| 文本 | 任意 | 文本内容块 |
| 图片 | `SupportsVision=true` | 原生图片内容块 |
| 图片 | `SupportsVision=false` | 明确的 `unsupported_media` 错误 |
| PDF | `SupportsNativePDF=true` | 原生文档内容块 |
| PDF | 仅 `SupportsVision=true` | 将请求页渲染为图片内容块 |
| PDF | 两项均不支持 | 明确的 `unsupported_media` 错误 |

不得默默将图片降级为“文件路径字符串”，因为这会让模型看起来执行成功，实际上没有获得图像内容。

### 5.4 供应商适配建议

这是基于当前 Awaken 适配器结构作出的目标设计推断，需要在正式开发时用各供应商实际请求测试确认：

- Anthropic Messages API 可以把图片内容放入 `tool_result` 的复合内容块；
- OpenAI-compatible/Kimi 链路可能需要在工具文本结果之后，补充一条由运行时生成的多模态 user message，用 `image_url` 或供应商支持的等价格式承载图片；
- PDF 原生文档块必须由明确的 `SupportsNativePDF` 控制，不能因为模型支持视觉就假设它原生支持 PDF；
- 工具调用 ID、消息顺序和下一轮 assistant 请求必须继续满足不同供应商的协议约束。

验收重点不是 schema 中出现 `supports_vision`，而是目标模型在真实调用中能够描述图片内容。

## 六、现有工具的精确替换规划

| 目标工具 | 取代的现有 Awaken 工具 | 目录操作 | 处理方式 |
| --- | --- | --- | --- |
| `Bash` | `shell` | 新增 `src/tools/bash`，切换后删除 `src/tools/shell` | 重写 |
| `Read` | `read_file`（目录为 `src/tools/file_read`） | 新增 `src/tools/read`，切换后删除 `src/tools/file_read` | 重写 |
| `Edit` | `text_edit` | 新增 `src/tools/edit`，切换后删除 `src/tools/text_edit` | 重写 |
| `Write` | `text_file_create` | 新增 `src/tools/write`，切换后删除 `src/tools/text_file_create` | 重写 |
| `Glob` | `list_files` | 新增 `src/tools/glob`，切换后删除 `src/tools/list_files` | 重写 |
| `Grep` | `grep_search` | 新增 `src/tools/grep`，切换后删除 `src/tools/grep_search` | 重写 |
| `WebSearch` | `web_search` | 保留 `src/tools/web_search` | 原目录内重构 |

删除动作只能发生在新工具完成注册、测试、真实运行验证之后。开发过程中应先让新旧实现并存但只在测试或受控入口启用新实现，最终再做一次原子名称切换，避免半迁移状态污染正式工具池。

### 6.1 七工具切换期内不得删除或修改的目录

- `src/mcpclient`
- `src/skills`
- `src/skillruntime`
- 现有 MCP 注册与调用相关目录
- Skills catalog、安装和运行时相关目录
- `src/tools/artifact_publish`（长期保留，交付物卡片唯一发布入口）
- `src/tools/artifact_runtime`、`src/tools/read_artifact`、`src/runtime/officerender`、`src/tools/apply_patch`、审批 metadata 与 Shell 可选 restricted/staging 路径（均已确认退役，但只能在阶段七、八、九按各自前置条件删除，切换期内保持原样；`WorkspaceExecutionBroker` 本身保留）
- 七类目标之外的其他工具目录

## 七、建议开发顺序

### 阶段一：建立七工具目标契约

1. 为七个目标工具定义 Awaken 名称、输入 schema、结果结构和错误类型；
2. 新建六个目标目录，保留并重构 `web_search` 目录；
3. 暂不删除旧工具，也不修改 MCP 与 Skills 注册；
4. 为新 schema 添加单元测试和快照。

### 阶段二：先打通 Read 多模态基础设施

1. 补齐 profile 到 agentcore 的模型能力传播；
2. 给工具结果增加文本、图片、文档和复合内容块；
3. 分别适配 Anthropic 与 OpenAI-compatible/Kimi 消息协议；
4. 实现 PDF 原生和页面渲染两条路径，页面渲染依赖 `pdftoppm`（poppler），缺失时返回带安装指引的明确错误；
5. 在真实 Kimi profile 下验证模型确实看见图片。

Read 是 Edit、Write 的读取状态来源，也是本次唯一跨越工具层与模型协议层的改造，应优先稳定。

### 阶段三：实现会话文件状态

1. 每个 thread/session 维护 `readFileState`；
2. 保存规范化路径、文件内容摘要、大小、修改时间和读取范围；
3. Edit/Write 在写入前验证文件没有变化；
4. 避免跨线程共享状态；
5. 明确会话关闭后的清理策略。

### 阶段四：实现 Bash、Glob、Grep

1. Bash 建立会话逻辑 cwd、超时、进度、截断和后台任务协议；
2. Bash 复用现有跨平台宿主 shell 解析链，Windows 优先 Git Bash、回退 pwsh/cmd，并补齐 `runtime.GOOS` 分支的聚焦测试；
3. Glob 建立稳定排序、结果上限和截断标志；
4. Grep 以 ripgrep 为执行基础，映射 Claude Code 风格参数，显式冻结默认 `output_mode`；
5. 关闭 shell 到其他工具的隐式命令变形。

### 阶段五：重构 WebSearch

1. 对齐查询和域名过滤 schema；
2. 保留 Awaken 自有搜索供应商；
3. 返回结构化来源与错误；
4. 验证与模型供应商无关，不引入 Anthropic 专属依赖。

### 阶段六：原子切换与旧工具清理

1. 在统一注册点把七个正式名称切到新实现；
2. 确认模型不再看到七个旧名称（旧读取工具名是 `read_file`，不要按目录名 `file_read` 检查）；
3. 删除六个被替换的旧目录；
4. `web_search` 在原目录完成重构，不重复建第二套；
5. 更新切换后仍注册工具（含待阶段七/八退役的 `apply_patch` 与 typed artifact 工具，以及长期保留的 `artifact_publish`、`trash_file` 等）描述与 recovery 文案中对旧工具名的引用；
6. 检查文档、测试和代码中是否仍引用旧名称；
7. 复跑 MCP 与 Skills 回归测试，证明它们未被改变。

### 阶段七：apply_patch 退役

前置条件：七工具已完成原子切换，并在真实 Desktop 验收稳定。

1. 把 `agentcore/auto_tool_policy.go` 的 file-chain 零审批放宽策略、`executors/file_chain.go` 和 `tool/profile/intent.go` 中围绕 `apply_patch` 的决策迁移或改写到 `Edit`/`Write`/`Bash` 语义上，保持工作区边界、敏感路径与 trash 可恢复不变量不丢失；
2. 从工具注册、prompt 模板（`src/prompt/templates/default_system.md`）和保留工具描述中移除 `apply_patch`；
3. 删除 `src/tools/apply_patch` 目录；
4. 处理引用它的测试与回放 testdata（约 50 个引用文件逐一核对）；
5. 更新 `docs/` 中对应功能域权威文档，复跑受影响测试。

### 阶段八：Office typed artifact 生成运行时退役

前置条件：阶段七完成。Office 产物 Skills 的建设与测试由用户在七工具转型完成后自行进行，不是本阶段的前置条件，也不在本计划验收范围内。

明确保留：`artifact_publish`、`tool.Artifact` 契约、`ArtifactCreated/ArtifactUpdated` 事件、AG-UI artifact 投影、Desktop 交付物卡片与 MCP artifact bridge。本阶段只退役“生成产物”的 typed 工具，交付物发布链路一行不动。

1. 从注册中移除 `document_create/edit`、`spreadsheet_create/edit`、`presentation_create/edit/render`、`office_export_pdf`、`pdf_extract`、`image_inspect`、`artifact_render`、`artifact_export`、`runtime_preflight`、`workspace_dependencies`、`read_artifact`；
2. 删除 `src/tools/artifact_runtime`、`src/tools/read_artifact` 与 `src/runtime/officerender`（Office worker）；
3. 更新系统提示词：Bash/Write/Skills 生成的最终产物必须显式调用 `artifact_publish` 发布为交付物卡片，这是交付物出现在 Workbench 的唯一途径；
4. 清理 prompt 模板与保留工具描述中对 typed artifact 工具、`read_artifact` 的全部引用（当前 `file_read`、`list_files`、`web_search` 等描述中也有引用，注意这些目录届时已被七工具替换）；
5. 更新 `docs/` 权威文档：04 域的《Artifact 与演示文稿工具契约》、03 域的 Office worker 章节，同一任务完成；
6. 复跑回归，并在真实 Desktop 确认：用 Bash/Write 产出文件后经 `artifact_publish` 发布，交付物卡片正常出现且可点击打开；历史会话中的旧 typed artifact 记录回放不崩溃。

### 阶段九：未使用审批设计退役

前置条件：阶段七、八完成（先收敛工具面，再删横切机制，缩小回归面）。

已确认决策：保留现有 `WorkspaceExecutionBroker`，只删 Shell 的可选 restricted/staging 路径和审批 metadata。`dependency_install`、`git_remote` 与 MCP artifact bridge 的 staged 执行合同（暂存、校验、Trash、原子提交、receipt）原样继续使用，不做“抽成 typed staging”的改名重构。

1. 删除审批卡 metadata 链路：`tool.DecisionMetadata`、`DecisionMetadataDescribingTool`、`AllowPermissionWithMetadata`/`DenyPermissionWithMetadata` 的 metadata 载荷、各工具 `CheckPermission` 中构造审批文案的代码（如 shell 的 `shellApprovalDescription` 与 ShellLane 审批描述）；
2. 同步清理 wire 与 Desktop：AG-UI 事件、`apps/desktop` 的 `agui.ts`/`reducer.ts` 中对应的 metadata 字段与展示分支；
3. 删除 Shell（阶段六后为 `Bash`）的可选 restricted/staging 执行路径：supervised execution、`ErrExecutionSandboxRequired`/`ErrExecutionTransformRequired` 等 outcome 分支，以及 Bash 结果中的 `broker_receipt` 暴露；
4. 不动 `tool/execution_broker*.go`：`dependency_install`、`git_remote`、MCP artifact bridge 对 `WorkspaceExecutionBroker` 的默认使用及其测试保持原样；
5. 保留并重申安全不变量：workspace boundary、writable/output roots、敏感路径拒绝、trash 可恢复、guardian 只读、`auto_tool_policy` 的 allow/deny 决策不受影响，相关测试必须继续通过；
6. 更新 04 域权威文档（《Shell 本机执行面》《工具决策、Outcome 与失败监督》等）删去 Shell restricted 路径与审批 metadata 章节，typed 工具的 broker 合同章节保留；
7. 复跑 Core 全量测试与 Desktop smoke，确认工具执行、事件流和历史回放无残留字段错误。

## 八、关键工程风险

### 8.1 误把分阶段收敛做成一次性大爆炸

这是最大范围风险。apply_patch、Office typed artifact 生成运行时与未使用审批设计虽已确认退役，但它们的删除各有前置条件（阶段七、八、九），不能与七工具原子切换合并成一次变更。切换期内工具注册、提示词和文档必须继续允许 MCP、Skills、Artifact 与其他内置工具并存；每个阶段结束时产品都处于可验收的完整状态。退役对象以 2.4 边界清单为准，清单外的模块一律不动，交付物卡片链路任何阶段都不得中断。

### 8.2 多模态只在 UI 或 profile 上标记，未真正进入模型请求

仅显示“支持视觉”或让 Read 能读取图片文件都不够。必须抓取或测试真实 provider request，确认图片字节或受支持的图片 URL 已进入下一轮模型输入。

### 8.3 Edit/Write 的读取状态被模型参数替代

如果仍要求模型传哈希、mtime 或完整旧内容，模型容易构造错误并产生伪并发保护。读取状态应由宿主维护，模型只表达编辑意图。

### 8.4 Bash 逻辑 cwd 污染整个进程

Electron/Core 可能同时运行多个会话。不得通过全局 `os.Chdir` 维护 Bash 工作目录，必须在会话上下文中保存并为每次命令显式设置 cwd。

### 8.5 直接复制 Claude WebSearch

Claude Code 的 WebSearch 与 Anthropic 服务端工具绑定。直接复制会破坏 Awaken 的多供应商目标，也可能让 WebSearch 隐性依赖特定模型。只能复制协议和结果设计。

### 8.6 新旧工具长期双轨

长期同时暴露 `shell/Bash`、`read_file/Read` 等重复工具会让模型选择不稳定。并存只应用于开发与测试，正式切换必须一次性完成名称迁移。

### 8.7 交付物卡片断供或误删安全不变量

Workbench 最终交付物卡片是核心产品功能。typed artifact 生成工具退役后，卡片的生产者只剩 `artifact_publish`（和 MCP artifact bridge）：Bash/Write/Skills 直接写文件不会产生 `ArtifactCreated/ArtifactUpdated` 事件，如果系统提示词不明确要求发布步骤，用户侧“最终交付物”会静默变空，这是本阶段最容易踩的产品回归。其二，审批 metadata 与 Shell 可选 restricted/staging 路径属于“未使用的审批设计”，但 `WorkspaceExecutionBroker`（typed 工具的暂存/校验/Trash/原子提交合同）、workspace boundary、敏感路径拒绝、trash 可恢复、guardian 只读等安全机制不是审批，混在同一批代码附近，删除时必须以 2.4 清单为界逐项核对，安全不变量的测试一条不许少。Office 产物的结构化生成能力在阶段八之后、用户 Skills 建好之前存在明确空窗（Agent 只能用 Bash 脚本产出 Office 文件），这是用户已接受的过渡状态；但空窗期内交付物卡片链路必须始终可用。

### 8.8 Windows 行为只靠假设

Windows 上 Git Bash 不一定存在，`pwsh`/`cmd.exe` 的引号、路径和重定向语义与 bash 不同。若只在 macOS 验证就宣称 Bash 跨平台可用，会把平台差异留给线上。必须为 shell 解析回退链和 `runtime.GOOS` 分支写聚焦测试，并把 Windows 实机验收缺口如实写进完成声明。

## 九、测试与验收清单

### 9.1 工具注册

- 模型正式可见的名称包含 `Bash`、`Read`、`Edit`、`Write`、`Glob`、`Grep`、`WebSearch`；
- `shell`、`read_file`、`text_edit`、`text_file_create`、`list_files`、`grep_search` 和旧 `web_search` 协议不再暴露；
- 保留工具（如 `apply_patch`、`trash_file`）的描述、WhenToUse/WhenNotToUse 和 failure recovery 文案中对旧工具名的引用（当前 `apply_patch` 内有十余处）已改为新名称；
- 七类目标之外的 Awaken 内置工具仍正常注册；
- MCP 工具数量、名称、schema 和调用行为不变；
- Skills 的发现、安装、加载和运行行为不变。

### 9.2 Bash

- 无审批弹窗或等待状态；
- 使用当前登录用户执行；
- `cd` 只影响当前会话后续 Bash，不影响其他会话；
- timeout、非零退出码、stdout/stderr、截断均可辨识；
- 后台命令返回可追踪任务标识；
- 普通 Bash 命令不再被隐式改写成 `apply_patch`；
- Windows：宿主 shell 解析优先 Git Bash、回退 pwsh/cmd 的分支有聚焦单测覆盖，结果中如实报告实际 shell；实机验收缺口按 README 现状如实报告，不写“全平台通过”。

### 9.3 Read 多模态

- 文本 offset/limit 行为稳定；
- 支持视觉的 Kimi profile 能真实描述所读图片内容；
- 不支持视觉的模型不会收到图片内容块，并获得明确错误；
- 支持原生 PDF 的模型收到文档块；
- 仅支持视觉的模型收到指定页渲染图片；
- PDF 页数上限和非法 pages 参数有明确错误；
- 重复读取未变化文件能返回稳定的 unchanged 语义。

### 9.4 Edit/Write

- Edit 已有文件前必须在当前会话 Read 过该文件（允许部分读取，读取状态未过期即可）；
- Write 新文件不要求预读；
- Write 覆写已有文件前必须 Read；
- 文件被外部修改后，Edit/Write 拒绝过期写入；
- 非唯一 `old_string` 默认拒绝，`replace_all=true` 才允许多处替换；
- 编码和换行风格得到保留；
- 返回结构化 diff 或覆写摘要。

### 9.5 Glob/Grep

- Glob 与 Grep 职责不混淆；
- Glob 支持相对/绝对搜索根、稳定排序、上限和截断；
- Grep 使用真实 ripgrep，覆盖 glob、type、上下文、大小写、多行、offset 和 head limit；
- 文件名含空格、Unicode 和特殊字符时行为正确。

### 9.6 WebSearch

- allowed 与 blocked domains 校验正确；
- 返回标题、URL、摘要等结构化来源；
- 无结果、超时、供应商失败可区分；
- 不依赖 MCP、Skills 或 Anthropic 专属 server tool；
- 在不同 Awaken 模型供应商下保持相同工具协议。

### 9.7 回归验收

- 运行 Core/工具相关 Go 测试；
- 运行受影响 package 的 typecheck/build；
- 运行 MCP 与 Skills 的既有聚焦测试，结果与改造前一致；
- 按根 README 的正式入口启动真实 Awaken Desktop；
- 在真实 Desktop 中分别验证七类工具，而不是只调用底层 API；
- 检查工具调用记录、消息历史恢复和重新打开会话后的展示；
- 执行 `git diff --check` 并检查旧工具名残留。

### 9.8 阶段七/八/九退役验收

- 阶段七完成后：模型不再看到 `apply_patch`；file-chain 零审批策略的工作区边界、敏感路径与 trash 可恢复不变量在 Edit/Write/Bash 路径上有等价测试；`src/` 无 `apply_patch` 残留引用；
- 阶段八完成后：全部 typed artifact 生成工具与 `read_artifact` 不再注册，`src/tools/artifact_runtime`、`src/tools/read_artifact`、`src/runtime/officerender` 已删除；`artifact_publish` 仍正常注册，在真实 Desktop 中用 Bash/Write 产出文件、经 `artifact_publish` 发布后交付物卡片出现且可点击打开（Word/Excel/PPT/PDF/Markdown 至少各验一次）；历史会话中的旧 typed artifact 记录回放不崩溃；
- 阶段九完成后：`DecisionMetadata`、审批文案、Shell/Bash 的 restricted/staging 路径与 `broker_receipt` 暴露在 `src/` 与 `apps/desktop` 均无残留；`WorkspaceExecutionBroker` 及 `dependency_install`、`git_remote`、MCP artifact bridge 的 staged 合同测试原样通过；workspace boundary、敏感路径、trash 可恢复、guardian 只读等安全不变量测试全部继续通过；
- Office 产物 Skills 的建设与真实场景测试由用户自行完成，不作为本计划的验收项；
- 每个退役阶段单独复跑 MCP 与 Skills 回归，`docs/` 权威文档与代码同一任务更新。

## 十、正式开发前的冻结决策

以下内容为本计划已确认边界，开发中不再重新讨论：

1. 对齐的是 Claude Code 的工具职责、协议和执行语义，不复制 Claude 品牌；
2. Awaken 正式工具名称为 `Bash`、`Read`、`Edit`、`Write`、`Glob`、`Grep`、`WebSearch`；被替换的旧名称以代码中 `Name()` 返回值为准（读取工具为 `read_file`，不是目录名 `file_read`）；
3. 保持零审批，不引入 Claude Code 权限和沙箱确认系统；
4. Read 多模态由显式模型能力控制，Kimi 等视觉模型应接收真实图片内容；PDF 页渲染依赖 poppler，缺失时返回明确错误而不静默降级；`supports_native_pdf` 作为显式 profile 字段加 Desktop 高级开关，存量 profile 默认 false，不按模型名或协议自动判断；
5. Bash 必须兼容 Windows：复用现有跨平台宿主 shell 解析链，Windows 优先 Git Bash、回退 pwsh/cmd；本次不新增独立 PowerShell 工具；
6. Edit/Write 的前置条件是“当前会话读取过且未过期”，允许部分读取，不要求完整读取；
7. Grep 默认 `output_mode` 冻结为 `content`，与现有模型使用习惯一致；Glob 冻结为稳定路径排序，不照搬 Claude Code 的修改时间排序；
8. MCP 与 Skills 体系本身完全冻结，不纳入重构；Office 产物 Skills 的建设与测试由用户在七工具转型完成后自行进行，不属于本计划交付；
9. `apply_patch`、Office typed artifact 生成运行时、未使用的审批设计（审批卡 metadata 与 Shell 可选 restricted/staging 路径）确认退役；`WorkspaceExecutionBroker` 及 typed 工具的 staged 安全合同保留，不删也不改名；退役只能按阶段七、八、九的前置条件执行，七工具原子切换期内不动，处置以 2.4 边界清单为准；
10. Workbench 最终交付物卡片是核心产品功能：`artifact_publish`、`tool.Artifact` 契约、`ArtifactCreated/ArtifactUpdated` 事件、AG-UI 投影、Desktop 卡片与 MCP artifact bridge 长期保留，Bash/Skills 产物经 `artifact_publish` 显式发布；`calculator`、`ask_user`、`dependency_install`、`git_remote`、`trash_file` 等其他内置工具不因本计划被删除；workspace boundary、敏感路径拒绝、trash 可恢复、guardian 只读、allow/deny 决策等安全不变量不得随审批设计一起删除；
11. 保持 Awaken `src/tools` 一工具一目录；
12. 新工具通过验证后，才删除被定点替换的六个旧目录，并同步清理保留工具中对旧名称的引用；
13. WebSearch 在现有目录内重构，但继续使用 Awaken 自有搜索供应商；
14. 正式完成声明必须同时包含自动化测试和真实 Desktop 验收证据（macOS 为当前验收基线，Windows 缺口如实报告）。

## 十一、最终建议

建议把本项目定义为“七类基础工具定点换代 + 三块既有能力的分阶段退役”，而不是一次性的工具系统推倒重建。

其中最先需要解决的不是目录改名，而是两条基础契约：

1. 工具结果从纯字符串升级为可承载文本、图片、文档和复合内容的类型化协议；
2. Read、Edit、Write 共享由会话宿主管理的文件读取状态。

这两条稳定后，Bash、Glob、Grep 和 WebSearch 都可以在不触碰 MCP 与 Skills 的前提下独立迁移；七工具经真实 Desktop 验收后，再按阶段七退役 `apply_patch`、阶段八退役 Office typed artifact 生成运行时、阶段九清理未使用的审批设计。最终 Awaken 的工具面收敛为：七个 Claude Code 风格基础工具、MCP、Skills、以 `artifact_publish` 为入口的交付物卡片链路和少量保留内置工具，Office 产物生成由用户后续以 Skills 承载，同时保留自己的名称、多模型架构和真正干净的零审批执行链路。
