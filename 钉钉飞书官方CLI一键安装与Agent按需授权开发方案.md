# 钉钉、飞书官方 CLI 一键安装与 Agent 授权开发方案

> 第一次失败实现仍保留在当前未提交工作区，仅供失败参考；本文不把它写成当前能力。

## 1. 原始需求

1. 用户点击“安装 CLI”后，前端直接创建并进入一个普通新会话，自动向 Agent 发布指令。Agent 指引用户完成登录，并按 CLI 实际返回的信息输出登录链接或二维码。
2. 用户以后在其他会话使用 CLI，如果发生需要授权的情况，Agent 按本次 CLI 的实际返回输出链接、二维码或必要说明，等待用户完成操作后继续原任务。

这不是为钉钉、飞书单独创建一种“指引会话”或专用 Agent 流程。为了完成上述交互，Awaken 需要补齐通用的：

- 产品界面动作交接到普通 Agent 会话；
- 工具执行中请求用户完成外部操作；
- 任务等待、验证和恢复；
- 链接、二维码等受控交互内容在 Workbench 中的安全展示。

## 2. 官方 CLI 返回什么

### 2.1 钉钉 DWS

官方项目：[DingTalk Workspace CLI](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli)，[中文 README](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/README_zh.md)，[CHANGELOG](https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli/blob/main/CHANGELOG.md)。

- `dws auth login`：默认打开浏览器，通过本机 loopback 回调完成登录。
- `dws auth login --device`：提供短授权地址和 `user_code`。
- 业务命令需要 PAT 权限时：官方 host-owned 流返回退出码 `4` 和单行 JSON，其中可包含完整 `data.authorizationUrl`。
- DWS 没有与飞书相同的独立 PNG 二维码命令。Awaken 应使用 CLI 返回的原始授权 URL 生成临时二维码，同时保留可点击链接。
- 授权完成后，Awaken 验证状态并重放原命令一次。

### 2.2 飞书 Lark CLI

官方项目：[Lark CLI](https://github.com/larksuite/cli)，[官方 README](https://github.com/larksuite/cli/blob/main/README.md)。

- `lark-cli config init --new`：输出需要交给用户的配置链接，并等待用户完成配置。
- `auth login --scope/--domain ... --no-wait --json`：立即返回 `verification_url` 和 `device_code`。
- `auth qrcode <url> --output qr.png`：把同一个授权 URL 生成 PNG 二维码。
- `auth login --device-code <device_code>`：完成前面发起的授权。
- 业务权限不足时，JSON 错误可包含 `missing_scopes`、`console_url` 和 `hint`。
- user 缺 scope 时申请最小权限；bot 缺权限时展示 `console_url`，不能错误执行 user login。

### 2.3 统一结论

- Agent 必须依据本次 CLI 的实际结构化返回决定展示链接、二维码、code 或管理员入口，不能假设所有情况都返回二维码。
- URL 按 opaque string 处理，不重新编码或拼接。
- device code、token、secret、原始 stdout/stderr 和二维码本地路径不进入模型、普通日志、SemanticJournal、AG-UI 或 transcript。

## 3. Awaken 需要扩展的通用能力

### 3.1 界面动作交接给普通会话

连接器页点击“安装 CLI”后：

1. 发起受管 CLI 安装；
2. 创建并进入一个普通新会话；
3. 自动提交一次受控 Agent 指令，例如“完成钉钉 CLI 登录与授权”；
4. Agent 等待 CLI 可用，然后执行官方登录流程。

这复用正常 thread/run，不新增 CLI 专属会话类型。双击必须幂等，不能重复创建会话或重复发指令。

### 3.2 工具请求用户外部操作

Tool Runtime 增加通用、可恢复的 `interaction_required` outcome。它至少支持：

- HTTPS 链接；
- 受控二维码图片；
- 短 code 和说明；
- 等待、确认、过期、失败、取消、重新获取。

CLI 授权只是首个使用者。以后其他需要用户去浏览器或手机操作的工具也应复用该能力。

### 3.3 任务等待与恢复

- 当前 run 保持非终态，不生成最终回答，不丢失原 tool call。
- 用户完成操作后，由 Main/Core 验证真实授权状态。
- 验证成功后恢复原调用；同一调用最多自动重放一次。
- 再次返回相同授权要求时停止循环，提供可恢复错误。
- 用户取消、授权过期或 Desktop 重启时，不能由迟到事件错误恢复旧任务。

### 3.4 Workbench 安全展示

使用通用 `ExternalActionGate` 放入现有 `composer-dock-stack`：

- 不进入 ExecutionFeed、narrative、AG-UI、transcript 或历史 replay；
- 不改变 turn identity；
- 二维码容器固定尺寸，链接、加载、过期和重试原位替换；
- 复用现有 ResizeObserver、composer inset 和 scroll controller；
- 出现和移除不能破坏草稿、焦点、follow/paused、marker 或 return-to-latest；
- 页面往返后仍显示当前任务的 active gate，其他会话不显示。

### 3.5 CLI 安装与适配边界

- Electron Main 管理固定 CLI 安装、敏感授权 session 和子进程。
- Core/Tool Runtime 管理 Agent run、`interaction_required` 和 continuation。
- Renderer 只创建受控 product action 并展示公共安全视图，不能提交任意 command、argv、cwd 或 env。
- CLI 自动发现、验证、安装和修复；不得让用户选择 executable 或打开 Finder/Explorer。
- readiness 以版本、登录状态和真实所需命令为准；`dws doctor` 只能用于内部诊断，不能覆盖已验证的可用事实。

## 4. 开发计划

1. 定义通用 product-action handoff、`interaction_required`、continuation 和 `ExternalActionGate` 契约。
2. 实现“安装 CLI → 普通新会话 → 自动 Agent 指令”的完整 handoff。
3. 实现 Workbench 外部操作 gate，并验证滚动、草稿、焦点、页面往返和 replay。
4. 分别接入 DWS 与 Lark CLI 的真实返回适配器、临时二维码和授权验证。
5. 重做自动安装、readiness 和错误恢复，删除“选择 CLI”用户路径。
6. 使用隔离 `AWAKEN_HOME` 验证两条旅程：
   - 点击安装后在新会话完成登录；
   - 其他业务会话缺权后原位授权并继续任务。
7. 分别完成 macOS 与 Windows 真机安装、授权、过期、取消、重启和卸载验收后，才能声明完成。

## 5. 第一次开发失败记录

### 5.1 做了什么

第一次实现增加了：

- Connector `mcp | cli` 分类与旧 MCP 迁移；
- `CliConnectorManager`、固定版本 manifest、下载校验、安全解包和安装；
- DWS/Lark 配置、登录、状态、退出和卸载；
- CLI readiness IPC、临时二维码协议和 Workbench 授权卡；
- 相关 Go、Desktop 和 Renderer 测试。

这些代码仍保留在当前未提交工作区。

### 5.2 失败在哪里

- 强制 CLI 卡片横跨双列，破坏了原有一左一右布局。
- 把完整二维码放进连接器卡片，放错了交互位置。
- 增加“选择 CLI”并打开系统文件选择器，把内部路径问题转嫁给用户。
- 用 `dws doctor` 否定已安装、已授权的 CLI，产生错误 readiness。
- 直接显示 `installed / failed / authorized` 等互相冲突的内部状态。
- 点击安装后没有交接到普通新会话并自动向 Agent 发指令。
- 其他会话遇到 CLI 授权时，没有通用的 `interaction_required`、任务等待和恢复能力。
- 围绕 Connector 单独实现授权 UI，没有先补 Awaken 通用的会话 handoff、工具交互 outcome 和 Workbench gate。
- 自动化通过后过早声明完成，真实 Desktop 随即暴露上述问题；Windows 真机也未验收。
- 构建期间干扰运行中的 Desktop，出现 Vite 错误遮罩。

### 5.3 后续处理

- 失败实现代码不回退、不修复、不删除，继续作为失败参考。
- 下一次开发先实现第 3 章的 Awaken 通用能力，再接入 DWS/Lark。
- 不能只修 CSS、doctor 判断或文件选择器后继续沿用第一次设计。

## 6. 当前进度（截至续作）

> 本节省记任务账本，不是产品权威。当前产品事实以 `docs/07_连接器/` 与 Workbench 相关契约为准。

### 6.1 已完成

- **通用 `interaction_required`**：Core Tool Runtime 可在 Bash/CLI 返回授权信号时进入等待；支持确认完成、取消、重试；验证后最多自动重放原调用一次；敏感信息（device code / token / secret / 原始 stdout / 本地二维码路径）不进模型与 transcript。
- **Workbench `ExternalActionGate`**：放入 `composer-dock-stack`；有 HTTPS 出链接，有受控 `qrImageUrl` 出二维码；不进入 narrative / ExecutionFeed / 历史 replay。
- **CLI 受管安装与边界**：Electron Main `CliConnectorManager` + 固定 manifest；删除「选择 CLI」；`dws doctor` 不覆盖已验证授权就绪。
- **连接器产品卡**：品牌 logo（官方 App Icon）、钉钉/飞书名称与简介；已连接点开详情（状态、姓名/组织、退出登录）；不展示版本/ownership 等内部枚举。
- **两段式主操作（已相对 §1/§3.1 纠偏）**：
  - 未安装：右侧仅「+」，只触发受管安装，不跳转会话；
  - 安装中：同一位置为「取消」；
  - 安装完成且未授权：同一按钮变为「发起登录」；
  - 点击「发起登录」：进入普通新会话并自动提交受控 Agent 登录指令（幂等防双击）。
- **Agent 指令**：仅授权语义（不再写「请等待安装…」）；要求按本次 CLI 返回展示链接/二维码，禁止编造。
- **域文档已同步**：`docs/07_连接器/*`、Workbench 中间区契约中的 handoff / gate 描述已改为上述两段式。
- **自动化**：相关 Go / Desktop 单测与 typecheck 已在续作中执行通过；**未**宣称真实 Desktop / Windows 全旅程完成。

### 6.2 待做

- **官方 Skill 路径纳入 Awaken 读取**：用户将把 `~/.agents/skills`（及官方会写入的同类 Agent skills 目录）加入 Skills 发现/读取；当前受管安装**只装 CLI 二进制，不随官方脚本安装 skill**。官方路径备忘：
  - 钉钉 DWS：全局 `$HOME/.agents/skills/dws`（`install.sh` 还会铺到 `~/.claude/skills/dws`、`~/.cursor/skills/dws` 等）；项目 `./.agents/skills/dws`；可用 `dws skill setup`。
  - 飞书 lark-cli：安装流程要求同步 skill（`npx @larksuite/cli@latest install` 或 `npx skills add larksuite/cli -y -g`）；全局一般落在 `~/.agents/skills/` 下各 `lark-*`。
- **受管安装是否/如何附带官方 skill 包**：尚未设计；与「只读 `.agents/skills`」可分开推进。
- **真实 Desktop 验收**：隔离 `AWAKEN_HOME` 下「+ 安装 → 发起登录 → 新会话授权」与「业务会话缺权 → ExternalActionGate 原位继续」两条旅程；二维码占位、滚动/草稿/焦点、页面往返。
- **macOS / Windows 真机**：安装、授权、过期、取消、重启、卸载；Linux 保持 `unsupported`。
- **飞书配置步骤产品化**：`configuration === required` 时卡片现为「配置应用」本地动作，与「发起登录」的衔接是否还需产品打磨。
- **方案正文 §1 / §3.1 / §4 仍写旧「点安装即 handoff」**：以 §6 与 `docs/07_连接器` 为准；有空再回改方案前文以免误导。
