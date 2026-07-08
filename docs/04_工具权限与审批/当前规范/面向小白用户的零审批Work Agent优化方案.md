# 面向小白用户的零审批 Work Agent 优化方案

## 文档定位

本文档在《[零审批自动执行策略与落地路线](/Users/nolangrant/Awaken/docs/04_工具权限与审批/当前规范/零审批自动执行策略与落地路线.md)》之上，进一步收敛 Awaken 面向小白用户与重 work 场景的正式优化方向。

本文档重点回答三个问题：

- 小白用户不想审批，但又怕风险，系统应该怎么做。
- work 场景需要大量处理 Excel、Word、PDF、PPT、图片、视频与连接器任务，Awaken 的能力面应该怎么重构。
- 用户明确希望可修改工作区外文件时，系统如何在不依赖频繁审批的前提下仍保持内建安全边界。

本文档不讨论具体代码实现细节，而定义目标产品模型、运行时边界、能力分层与分阶段落地路线。

## 背景结论

基于当前 Awaken 现状与对 Codex、Claude Code 源码模式的对照，可以得出三个结论：

### 结论 1：问题不只是审批，而是能力模型错误

Awaken 现在的主链仍然偏向：

- `read_file`
- `list_files`
- `grep_search`
- `apply_patch`
- `shell`

这套链路适合代码和文本任务，但不适合作为通用 work Agent 的基础能力面。对于 Excel、Word、PDF、PPT、图片、视频、连接器工作流，系统不应要求模型先尝试 `shell` 或 `read_file` 再失败。

### 结论 2：最优解不是单纯放开 shell / MCP

如果只把 `shell`、`MCP`、外部文件写入全部默认放开，用户虽然少点了审批，但系统会把高风险判断重新交给模型与运气，无法形成稳定产品。

真正更优的方向是：

- 用 Codex 风格的能力分层，解决“该走哪条能力链”。
- 用 Claude Code 风格的权限分层，解决“哪些边界允许自动执行”。

### 结论 3：工作区不是唯一边界，任务作用域才是

“是否允许修改文件”不应只由 `workspace_root` 决定。对小白用户来说，更自然的问题是：

- 这个文件是不是我当前任务明确交给 Awaken 的。
- 这个目录是不是我当前任务允许 Awaken 处理的。
- 这个结果是不是应该输出到一个明确可见的地方。

因此，Awaken 需要从“工作区边界”升级到“任务作用域边界”。

## 目标

- 默认保持零审批体验，不把审批弹窗作为主产品路径。
- 让 work 文件处理成为一等能力，而不是代码工具的附属能力。
- 让 MCP / connector 成为一等业务能力，而不是默认危险能力。
- 允许修改工作区外文件，但前提是这些文件被明确纳入当前任务作用域。
- 真正危险的动作由系统直接阻断或收口，而不是交给小白用户逐步判断。
- Mac 与 Windows 共享同一套上层契约，但允许不同平台用不同执行后端。

## 非目标

- 不追求“所有动作都能自动执行”。
- 不把“无审批”误解为“无边界”。
- 不要求基础聊天输入层直接支持所有办公文件格式。
- 不要求第三方任意 MCP 默认获得高信任文件与网络能力。

## 核心方案

### 一、把工作区模型升级为任务作用域模型

每次 run 都不再只携带单一 `workspace_root`，而是携带完整 `TaskScope`：

- `scope_id`
- `workspace_root`
- `readable_roots`
- `writable_roots`
- `output_roots`
- `protected_roots`
- `attached_files`
- `connector_resources`
- `temporary_work_root`

其中：

- `readable_roots` 记录本次任务的建议读取范围，并继续服务 connector/file bridge；Awaken 内置本地读工具不再用它拒绝项目外本机路径。
- `writable_roots` 定义本次任务允许写入的文件范围。
- `output_roots` 定义允许 Awaken 创建结果产物的目录。
- `protected_roots` 定义无论何种模式都默认受保护的路径。

这意味着：

- 工作区外文件可以改。
- 但它们必须先被加入当前任务作用域。
- 系统不再在“工作区内可改 / 工作区外全拒绝”之间二选一。

### 二、把能力面拆成 4 条正式能力链

Awaken 不再把所有任务都压到通用文件工具和 shell 上，而是显式拆成以下能力链：

### 1. 文件产物链

面向：

- `xlsx/xls/csv/tsv`
- `docx`
- `pdf`
- `pptx`
- `md/txt/json`
- 图片
- 视频

职责：

- 解析
- 提取结构化内容
- 创建与修改产物
- 生成中间格式
- 验证最终结果

要求：

- 不依赖 `read_file` 直接读取二进制文件。
- 不要求模型先用 shell 猜环境。
- 每种格式都有对应 executor 或 adapter。

### 2. 连接器能力链

面向：

- MCP
- 官方连接器
- 浏览器型 connector
- 文档/表格/云盘/协同系统

职责：

- 查询外部系统
- 写入外部系统
- 上传/下载文件
- 拉取受控资源

要求：

- 不再把所有 connector 默认标成 destructive。
- 必须有显式能力分类与信任分层。
- 必须有鉴权恢复链。

### 3. 本地受限计算链

面向：

- Python
- Node
- ffmpeg
- pandoc
- libreoffice
- poppler
- OCR
- 本地转换和分析脚本

职责：

- 把本地文件转换成可分析中间态
- 做结构化提取
- 做批处理、统计、渲染与导出

要求：

- 这条链不再等同于“危险 shell”。
- 它是单独的受限执行器。
- 只允许访问任务作用域内文件。
- 默认禁止任意联网。

### 4. 系统级能力链

面向：

- 系统设置修改
- 全局安装
- 广域网络动作
- 外部发布
- 高风险浏览器/系统交互

要求：

- 这不是默认主路径。
- 默认拒绝或仅在高信任模式放开。
- 不再让小白用户通过一堆审批弹窗来理解它。

### 三、把零审批改成“内建能力分层 + 内建边界”

Awaken 的零审批目标不应再依赖：

- 模型自觉
- prompt 提醒
- 用户临时判断
- 工具随意返回 `ask`

新的决策顺序应为：

1. 先按能力链路由。
2. 再按 `TaskScope` 判断文件与资源范围。
3. 再按工具/连接器风险分类决定执行器。
4. 再按 profile、sandbox、network、sensitive path 做内建约束。
5. 无法安全自动处理时直接 `deny` 或要求显式切换到高信任模式。

也就是说，运行时决策主结果依然是：

- `allow`
- `deny`
- `transform`

但 `ask` 不再是主用户交互面，而是只作为内部兼容态，逐步退出主路径。

### 四、重新定义 MCP / Connector 的风险模型

MCP 与 connector 不能再只靠：

- `read_only`
- `destructive`
- `open_world`
- `requires_auth`

这四个低维标签不足以承载真实 work 场景。

推荐补充为 `CapabilityClass`：

- `connector_read`
- `connector_write`
- `browser_read`
- `browser_interaction`
- `artifact_import`
- `artifact_export`
- `external_publish`
- `system_bridge`

同时保留：

- `requires_auth`
- `network_scope`
- `file_bridge_scope`
- `trusted_origin`

其中最关键的是 `trusted_origin`：

- 官方内建 connector
- 官方托管 connector
- 用户自接第三方 MCP

这三类不应共享同一默认权限。

### 五、引入官方文件桥与非官方隔离桥

对于文档、表格、PDF、PPT、图片这类 work 文件，系统应允许高信任能力链直接拿到：

- 本地文件句柄
- 上传后的 file payload
- 中间产物目录
- 渲染校验产物

但这条桥不应默认开放给任意第三方 MCP。

建议规则：

- 官方内建/官方托管 connector：可申请文件桥。
- 普通第三方 MCP：默认无文件桥，只能处理结构化入参或受限资源。
- 需要文件桥的第三方集成，必须显式纳入高信任配置。

### 六、把 shell 从“通用工具”降级为“兜底执行器”

shell 不应再是 work 场景的主入口。

更合理的职责是：

- 只读检查
- 本地受限计算链中的某些执行后端
- 系统级能力链的底层承载
- 少量不可替代的工程命令

新规则：

- 能走文件产物链的，不走 shell。
- 能走 connector 链的，不走 shell。
- 能走本地受限计算链的，不走普通 shell。

shell 继续保留，但从“用户看见的能力”退居到“系统内部执行后端”。

### 七、删除语义统一为回收站，不做永久删除

删除规则需要扩展到全部能力链，而不是只覆盖 `apply_patch`：

- 文件产物链删除：移入回收站
- 本地受限计算链生成的中间文件清理：进入受控临时目录回收策略
- connector 同步落地文件删除：优先走本地回收站语义

只有系统级高信任模式才允许永久删除。

### 八、建立敏感路径黑区

即使允许工作区外文件修改，也必须保留不可自动越过的敏感路径黑区。

至少包括：

- `.git/`
- `.awaken/`
- 凭据、证书、密钥目录
- shell rc / profile
- IDE 配置
- 系统配置目录
- Awaken 自身本地配置目录

这些路径不应被普通零审批路径覆盖。

### 九、用户体验重新定义

小白用户不应看到：

- `apply_patch`
- `shell`
- `destructive`
- `open_world`
- `ask permission`

小白用户应只看到：

- 本次任务允许访问哪些文件
- 本次任务可处理哪些文件类型
- 当前连接器能做查询还是写入
- 某个动作为什么系统不允许自动做
- 最终产物在哪里

也就是说，用户感知层的单位不再是工具，而是：

- 文件任务
- 外部系统任务
- 结果产物
- 风险说明

## 目标运行时结构

推荐新增或收敛为以下核心抽象：

### 1. TaskScope

负责表达任务边界，而不是只表达项目根目录。

### 2. CapabilityDescriptor

统一描述每个能力单元：

- 来源
- 能力类
- 文件桥权限
- 网络范围
- 鉴权要求
- 风险级别
- 可见性策略

### 3. CapabilityRouter

根据用户目标与输入资源，把请求路由到：

- 文件产物链
- 连接器链
- 本地受限计算链
- 系统级能力链

### 4. PolicyEngine

不再只做“allow/ask/deny”，而是结合：

- `TaskScope`
- `CapabilityClass`
- `trusted_origin`
- `ExecutionProfile`
- `sensitive paths`
- `network policy`
- `sandbox policy`

做最终 `allow/deny/transform`。

### 5. Artifact Executors

按文件类型拆分：

- `SpreadsheetExecutor`
- `DocxExecutor`
- `PdfExecutor`
- `SlidesExecutor`
- `ImageExecutor`
- `VideoExecutor`

### 6. Connector Runtime

支持：

- auth recovery
- cached capability snapshot
- file bridge
- trusted origin gate
- structured result normalization

## 分阶段落地路线

### 第一阶段：先改边界模型

目标：

- 从 `workspace_root` 升级到 `TaskScope`
- 支持 `additional writable roots`
- 建立 `protected_roots`
- 统一回收站删除语义

这一阶段解决的是“工作区外文件也能改，但不是全盘放开”。

### 第二阶段：先打通高频 work 文件

优先顺序：

1. `xlsx/xls/csv/tsv`
2. `pdf`
3. `docx`
4. `pptx`
5. 图片
6. 视频

目标：

- 不再依赖 `read_file` 读取二进制。
- 不再依赖模型自己拼 `shell + python`。
- work 任务第一次真正有正式主路径。

### 第三阶段：重构 MCP / Connector

目标：

- 引入 `CapabilityClass`
- 引入 `trusted_origin`
- 区分官方 connector 与第三方 MCP
- 引入 auth recovery 与 file bridge gate

这一阶段解决的是“Playwright / GitHub / 飞书 / 表格能力为什么老被误杀”。

### 第四阶段：建设本地受限计算链

目标：

- 提供本地转换执行器
- 让 Python / Node / ffmpeg / OCR 进入受限通道
- 从普通 shell 中剥离 work 计算

### 第五阶段：收缩 shell 到兜底角色

目标：

- shell 不再承担主 work 路径
- 只保留工程命令、系统命令、受限后端承载
- 强化 sandbox 与敏感路径保护

## 与现有规范的关系

本文档不推翻《零审批自动执行策略与落地路线》的大方向，但明确修正其中两点：

1. 文件链与 shell 的二分法还不够，需要扩展为“能力链四分法”。
2. `workspace_root` 还不够表达真实桌面 work 场景，需要升级为 `TaskScope`。

后续实现应以本文档作为面向小白用户与重 work 场景的目标方案，再把具体实现拆回：

- `03_运行时与本地服务`
- `04_工具权限与审批`
- `06_MCP运行时`
- `07_连接器`
- `08_技能`

对应的实施计划、接口设计、执行器设计与验收清单。

## 最终判断

Awaken 下一阶段不应继续围绕“怎么让 AUTO policy 更聪明”做局部修补。

真正更优的路线是：

- 上层用能力分层承接 work 场景。
- 中层用任务作用域表达真实文件边界。
- 下层用分层权限、sandbox、敏感路径与回收站语义做内建安全。

对小白用户，最佳体验不是“请你批准这一步”，而是：

**系统默认会做对的事，默认做不了危险的事。**
