# Awaken 后续开发待办清单

## 第一阶段：优先追功能

- [ ] **通用多 Agent 编排**
  - 主 Agent 自动拆分任务、创建多个 Agent。
  - 支持并行执行、暂停、取消、结果汇总和任务交接。
  - 展示 Agent 角色、当前任务、状态、进度和父子关系。
  - 后续加入虚拟办公室、人物走动等可视化表现。

- [ ] **苹果“灵动岛”式前端交互研究**
  - 提醒：开发前先研究 Awaken 前端哪些位置适合融入“灵动岛”式的收缩、展开、状态切换和轻量通知效果，再决定正式产品入口，不直接照搬外观。
  - 优先评估运行状态、当前 Agent/多 Agent 切换、等待用户操作、扫码授权、模型额度提醒和桌宠通知等短时状态场景。
  - 明确它属于顶栏状态入口、`RunStage` overlay、composer stack 还是桌宠通知；不得绕过现有 Workbench ownership 新增全局悬浮容器。
  - 默认保持轻量收起，只在状态变化或需要用户处理时展开；不得长期遮挡任务叙事、输入框或最终交付物。
  - 设计 loading、成功、失败、等待、展开详情和自动收起动效，并支持键盘操作、减少动态效果和不同窗口尺寸。
  - 先在 Design System Gallery 制作可复用 UI 原型，再选定一个真实场景接入 Awaken Desktop 验证；Gallery 效果不能替代真实 Desktop 验收。
  - macOS 可以采用更接近灵动岛的视觉语言；Windows 需提供一致语义的自适应样式，不能依赖 Mac 屏幕硬件形态。

- [ ] **Skill 详情与 `SKILL.md` 预览**
  - 点击 Skill 卡片打开详情。
  - 渲染 `SKILL.md` 正文和 frontmatter。
  - 展示 Skill 来源、资源、脚本、诊断及 MCP 依赖。
  - 交互形式与 MCP 详情页保持一致。

- [ ] **任务执行过程中的 Skill 调用展示**
  - Agent 调用 Skill 时，在 Workbench 中间区域显示具体 Skill 的 `displayName`/名称及 `description`，不再只显示通用工具名 `skill`。
  - 不在中心叙事中直接展示 `contract_id`、`content_sha256`、`invocation_source` 等 activation receipt 原始 JSON；这些内部字段继续保留在运行时记录中。
  - 为 Skill 激活建立稳定的展示字段和语义投影，包括 Skill ID、显示名称、description、inline/fork 模式和激活状态；Renderer 不从工具输出字符串临时解析业务信息。
  - description 使用调用当时 Registry 中的快照并随激活记录耐久保存，避免 Skill 后续修改导致历史展示漂移。
  - 激活中、成功、失败和 fork child 执行使用一致但可区分的状态文案；实时执行、线程切换、上下文压缩和重启后的历史重放保持一致。
  - 旧历史缺少名称或 description 快照时只回退显示 `skill_id`，不得扫描当前磁盘猜造旧记录。

- [ ] **跨任务长期记忆（Memory）**
  - 建立独立于 thread 历史、上下文压缩、SemanticJournal 和审计日志的长期记忆能力，让 Agent 可在后续任务中复用用户确认过的偏好、事实、约定和项目知识。
  - 支持用户级与项目级作用域；Awaken 自有存储分别位于 `~/.awaken/memory/` 和 `<workspace>/.awaken/memory/`，不得把某个 thread 的完整记录直接当作长期记忆。
  - 提供明确的 Memory 写入、读取、更新、替代和删除工具；每条记忆保存来源、作用域、创建/更新时间和可追溯 identity。
  - Agent 只能通过受控 Memory 工具写入候选记忆，并在中间区域明确展示写入或更新结果；不得静默保存 API Key、认证信息、验证码、完整系统提示或大段原始聊天记录。
  - 新任务开始和运行中按相关性检索，在独立上下文预算内注入必要记忆，并携带来源；不能把全部 Memory 无差别塞入模型上下文。
  - 处理重复、冲突、过期和 superseded 关系；新事实不得静默覆盖仍可能有效的旧事实。
  - 在 Settings 中提供 Memory 管理页面，支持搜索、按用户/项目筛选、查看来源、编辑、删除和关闭自动检索。
  - Memory 命中、未命中、写入、更新和删除在实时执行、重启及多 Agent handoff 中保持一致；child Agent 只能继承父任务明确允许的 Memory 范围。

- [ ] **生命周期 Hook**
  - 将当前仅解析和保留的 Skill `hooks` frontmatter 接入真实运行时，并扩展为可管理的用户级、项目级和 Skill 级 Hook 能力。
  - 第一批事件至少覆盖 run 开始/结束、工具调用前/后、Skill 激活前/后、Memory 写入后和任务失败；每个事件拥有稳定 payload schema 和版本。
  - 明确同步门禁 Hook 与异步通知 Hook：同步 Hook 可以显式允许、拒绝或变换受支持输入，异步 Hook 只能观察，不能暗中修改运行状态。
  - Hook 执行继续经过 Awaken 的工具、权限、TaskScope 和 Outcome 边界；Skill 声明 Hook 不得自动扩大 Shell、网络、文件或凭据权限。
  - 支持启用/停用、优先级、超时、取消、失败策略和去重；禁止无限递归、Hook 触发自身和失败后无界重试。
  - Hook 配置与执行结果需要耐久记录，重启后可恢复启用状态；敏感输入必须脱敏，不能把 API Key、认证信息或完整系统提示传给普通 Hook。
  - 在 Settings 中提供 Hook 列表、来源、触发事件、最近结果、错误诊断和手动测试入口，并覆盖 macOS、Windows 的真实 Desktop 验收。

- [ ] **Cron 与定时任务**
  - 支持一次性、固定间隔和标准 Cron 表达式，并为每个任务保存时区、workspace、Agent/Skill、模型 Profile、输入和启用状态。
  - 建立本地耐久 scheduler；Awaken 重启后恢复任务，不把 renderer timer 或单个 thread 的内存状态当作调度真相。
  - 明确应用未运行、系统休眠、离线和错过触发时间时的策略，支持跳过、启动后补跑或仅补跑一次，不默认批量追赶全部历史任务。
  - 为同一任务配置并发策略：跳过、排队或替换；默认禁止重复重叠执行，并为每次触发生成独立 run identity。
  - 定时任务执行必须使用正常 Agent、工具、权限、Skill、Connector 和 Memory 边界，不能因为后台触发而获得额外权限或绕过需要用户处理的 gate。
  - 在 Desktop 提供定时任务列表、下一次运行时间、暂停/恢复、立即运行、编辑、删除、运行历史和失败恢复入口。
  - 通知用户后台任务的开始、成功、失败和等待操作状态；涉及扫码、授权、主动提问或专用审核时进入等待状态，不伪装成无人值守完成。
  - 覆盖时区/DST、重启恢复、睡眠唤醒、取消、并发、任务修改和 macOS/Windows 实机验收。

- [ ] **兼容用户级 `.agents/skills`**
  - macOS：自动发现 `~/.agents/skills/`，实际路径为 `/Users/<用户名>/.agents/skills/`。
  - Windows：自动发现 `%USERPROFILE%\.agents\skills\`，实际路径为 `C:\Users\<用户名>\.agents\skills\`。
  - 作为用户级兼容 Skill 来源接入现有发现、去重、诊断和调用链。
  - `~/.awaken/skills` 继续作为 Awaken 自有的全局 Skill 安装目录；不自动迁移、覆盖或删除 `.agents/skills` 中的包。

- [ ] **钉钉、飞书官方 CLI 一键安装与扫码授权**
  - 在连接器页面检测钉钉、飞书官方 CLI 是否已安装，并提供点击即安装入口，不要求用户手动打开终端执行命令。
  - 安装完成后直接进入对应 CLI 的登录、应用配置和授权流程，并持续展示当前状态与恢复动作。
  - 需要扫码时，将二维码生成或保存为 Awaken 管理的临时图片文件，例如 `~/.awaken/runtime/auth/<provider>/<session>/qr.png`；不得写入用户项目目录或作为最终交付物发布。
  - 在 Workbench 中间区域以专用授权叙事卡片直接展示二维码、配置链接、说明文字和重新获取入口，用户无需打开文件夹寻找二维码。
  - 支持等待扫码、已确认、已过期、授权失败、取消和重试；扫码成功后自动继续连接并刷新 Connector readiness。
  - 二维码、临时授权信息到期后自动清理；CLI 凭据沿用官方 CLI 的安全存储，不进入模型上下文、SemanticJournal 或普通日志。
  - 同时覆盖 macOS 与 Windows 的安装、登录、扫码、重启恢复和卸载/断开验收。

- [ ] **原生 OS Computer Use**
  - 参考项目：[trycua/cua](https://github.com/trycua/cua)；用于评估跨平台 Computer Use driver、sandbox、MCP/CLI 接入和轨迹评测能力，不代表 Awaken 已完成技术选型或直接采用其 Agent loop。
  - 观察桌面、识别窗口和界面元素。
  - 执行鼠标、键盘、滚动和拖拽操作。
  - 支持截图验证、人工接管和操作轨迹记录。
  - 接入安全策略，避免不受控地操作系统。

- [ ] **WebSearch 联网供应商与 API Key 前端配置闭环**
  - 当前只有 Core 认证链路：内置 Brave adapter 从 `AWAKEN_WEB_SEARCH_ENDPOINT` 和 `AWAKEN_WEB_SEARCH_API_KEY` 读取配置并发送 `X-Subscription-Token`；Desktop 尚无联网搜索配置页、凭据持久化和连接测试，模型供应商 API Key 与 Context7 Connector 都不能代替该链路。
  - 在 Settings 增加独立的联网搜索配置；第一期只展示当前真实支持的 Brave provider，提供可选 Endpoint 高级项、蒙版 API Key 输入、已配置状态、更换和删除入口，不提前展示尚未实现的供应商。
  - 凭据由 Electron Main 的安全存储 owner 管理，与模型 Profile、MCP 和 Connector 凭据分开命名；Renderer 不持久、不回读明文，Key 不进入模型上下文、SemanticJournal、AG-UI、audit 或普通日志。
  - Runtime Supervisor 启动 Core 前清除登录 Shell 继承的 `AWAKEN_WEB_SEARCH_ENDPOINT` / `AWAKEN_WEB_SEARCH_API_KEY`，再注入 Desktop 持久化的产品配置，避免 Shell 配置静默改变产品身份。
  - 支持不落盘的连接测试，分别呈现未配置、认证失败、限流、网络错误、上游格式错误和取消；错误文案不回显 Endpoint 中的 credential 或 API Key。
  - 保存、更换或删除配置后必须通过可观测的 Core restart/reload 生效；失败时回滚到上一份可用配置，不能出现页面显示已配置但 Core 仍使用旧值。
  - 自动化覆盖凭据不进入 Renderer/日志、重启恢复、Shell 环境不能覆盖、错误分类和配置回滚；真实 Desktop 验收要求在无预设环境变量的新环境中从页面完成配置，重启后执行一次真实 `WebSearch`，再删除 Key 确认工具恢复为明确的未配置状态。

- [ ] **模型生态与能力闭环**
  - 扩充模型供应商和模型目录。
  - 建立统一的工具调用能力判断。
  - 打通图片、视频和 Prompt Cache 能力。
  - 完善不同供应商、不同 Profile 的额度和余额查询。

- [ ] **macOS/Windows 正式发布闭环**
  - 建立各平台独立的干净构建流程。
  - 完成 macOS 签名、公证和 Gatekeeper 验证。
  - 完成 Windows 代码签名、安装和卸载验证。
  - 正式发布稳定后，再决定是否开发自动更新。

## 最后阶段

- [ ] **独立任务日志与审计系统**
  - 在 `.awaken` 中保存独立于模型上下文的任务日志。
  - 上下文压缩不删除日志，也不自动把日志重新送入模型。
  - 支持按任务查询、导出和保留周期管理。
  - 过滤 API Key、认证数据、系统提示等敏感内容。
