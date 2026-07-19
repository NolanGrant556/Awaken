# 长期记忆与 Dream 设计契约

## 1. 状态与权威范围

本文是 Awaken 长期记忆与 Dream 的当前事实与目标设计权威。

**当前支持：** Core 已实现 user/project Markdown topic、`MEMORY.md` 索引、受控 `memory` 工具、host 注入来源 identity、revision/敏感内容/重复门禁、请求构建期索引注入、复用当前主模型 profile 的非阻塞 selector、最多 5 条有界主题召回和 compact 后预算复位。主动记忆默认开启，由主模型在正常任务推理中决定是否调用工具。Memory 调用与 recall 进入 SemanticJournal/AG-UI 前转为无正文 receipt；Workbench 已投影 recall/mutation 紧凑回执。Desktop Settings 已提供四个开关、搜索/筛选/正文与来源查看、revision 删除、手动 Dream 和取消；Auto Dream 已接入持久化活动计数、24 小时、5 分钟 idle、active-run gate 和新 run 取消；不提供人工创建、编辑或 supersede。Dream task/report 已原子持久化，Core 重启会把未完成任务恢复为 `interrupted`，Desktop 通过任务列表重放维护回执；scope lock 定期 heartbeat，批量 Apply 使用 staged transaction、活动 marker、失败关闭读取和 crash rollback，主题与 `MEMORY.md` 不会以半套状态参与召回。

**当前不支持：** 当前产品尚无正式 subagent/child Agent 能力，因此不实现 child recall 继承。待 subagent 域建立正式 parent/child identity、workspace 与权限边界后，本域必须实现 5.3 的 `MemoryRecallSnapshot` 继承门禁，不能让 child 自行扩大 user/project scope。

**当前验收边界：** Settings 的查看/筛选/详情清空、开关持久化和手动 Dream，以及未来持续称呼指令触发真实 selector、search/update、completion receipt 与简短收尾，已完成当前 macOS Desktop 验收；Workbench 回执、Dream 重启恢复、heartbeat 与 crash recovery 已有自动化，真实 Desktop 中断恢复和跨平台基线仍待验收。

本文未标注“当前支持”的 child 内容属于**已确认但依赖正式 subagent 能力的后置设计约束**。

本域拥有：

- 独立于 thread 的 user/project 长期记忆；
- `MEMORY.md` + 主题 Markdown 文件的存储合同；
- 新任务的有界相关性召回；
- 显式保存、读取、更新、替代、遗忘；
- 主 Agent 默认开启的主动记忆判断；
- 空闲期 Dream 整理；
- Memory 管理 UI、事件和恢复语义。

本域不拥有 thread 对话、上下文压缩、项目正式文档、Skill、权限真相或第二 Agent loop。长期记忆不能由 `context_compacted`、SemanticJournal 全文、审计日志或 renderer cache 代替。

## 2. 设计来源与采用范围

Awaken 采用 `sourcecode` 参考项目的核心 Memory 形态：

1. Markdown 文件是记忆内容 authority；
2. `MEMORY.md` 是小型索引，不承载完整正文；
3. 每个主题文件使用 YAML frontmatter 提供名称、描述和类型；
4. 召回先扫描文件头，再由 selector 从 manifest 中选择最多 5 个文件；Awaken 不配置独立小模型，selector 复用当前任务的主模型 profile；
5. 被选文件按行数和字节数有界注入；
6. 已读或已召回文件在当前上下文中去重；
7. Dream 低频整理主题文件和索引，合并重复、修正冲突并清理过时入口。

Awaken 不照搬以下边界：

- 主 Agent 和 Dream 不得通过普通 `Write`/`Edit`/`Bash` 自由修改 Memory；所有 mutation 经过 host-owned Memory service；
- Memory 文件位于 Awaken 明确拥有的 user/project 根，不复用 thread journal 或用户任意配置路径；
- 文件 frontmatter 增加稳定 identity、revision、source 与状态，以支持 UI、并发检查、替代和重放；
- Dream 先生成结构化 maintenance plan，再原子应用，不能边推理边留下半套整理结果。

## 3. 产品语义与作用域

### 3.1 记忆类型

封闭的类型集合为：

| type | 内容 | 典型作用域 |
| --- | --- | --- |
| `user` | 用户角色、目标、职责和知识背景 | user |
| `feedback` | 用户纠正或确认过的协作方式、偏好和非显然规则 | user；明确的项目共同规则可为 project |
| `project` | 无法从当前代码、Git 或正式文档直接推导的项目背景、决策、约束和原因 | project |
| `reference` | 外部系统、文档、频道、仪表盘或知识入口 | project；个人资源可为 user |

不得保存：

- 代码结构、文件路径、当前实现、Git 历史或正式文档已有事实；
- 当前任务计划、临时进度、一次性错误输出或完整对话；
- API key、密码、token、cookie、Authorization header、验证码或认证材料；
- system prompt、完整 tool input/result 或外部文件正文；
- 未经用户表达而推断出的敏感个人属性；
- 仅为未来一次当前 thread continuation 服务的信息。

用户明确说“记住”时立即走受控保存；“以后叫我……”“从今以后……”等语义明确、面向未来持续生效的指令即使没有“记住”二字，也属于显式持久化意图。用户明确说“忘记”时按 identity 删除或在目标确有歧义时用 `ask_user` 确认。默认开启主 Agent 主动记忆判断：主模型在正常任务推理中自行判断是否出现值得跨 thread 保存的信息，只有判断应保存时才调用受控 `memory` 工具；宿主不在每轮结束后再启动额外模型分析。

### 3.2 作用域

```text
user scope     ~/.awaken/memory/
project scope  <workspace>/.awaken/memory/
```

- user scope 可跨项目召回，保存用户背景和个人协作偏好；
- project scope 只在 canonical workspace/project identity 匹配时召回；
- project 路径位于工作区，可能被版本控制系统观察或提交，因此禁止写入 user profile、私密反馈和秘密；
- Awaken 不自动修改 `.gitignore`、`.git/info/exclude` 或 Git 状态；
- 移除项目只解除 Awaken 项目关系，不删除 workspace 内 Memory；项目整体移到废纸篓时 project Memory 随工作区一起移动；
- thread 删除不删除已经独立保存的 Memory，但其 source UI 应显示来源已不可用。

## 4. Markdown 存储合同

本节 4.1–4.3 的主题文件、最小 frontmatter、`MEMORY.md`、原子写入、revision、supersede/delete、敏感内容和重复检查当前已在 `src/memory` 与 `src/tools/memory` 实现。未知字段保留和跨平台原子替换仍只有当前 Go 自动化覆盖，尚无全平台实机结论。

### 4.1 目录布局

两个 scope 使用相同布局：

```text
memory/
├── MEMORY.md
├── user-role-7f31.md
├── feedback-testing-a82c.md
├── project-release-21bd.md
├── reference-observability-54ee.md
└── .maintenance-lock
```

主题文件是 Markdown；当前设计不使用数据库、SQLite、向量数据库或二进制 authority。`MEMORY.md` 和 frontmatter manifest 可以从主题文件重建，任何未来全文/向量索引只能是可删除、可重建 cache。

### 4.2 `MEMORY.md`

`MEMORY.md` 是导航和常驻小索引，不是记忆正文。每行格式：

```markdown
- [数据库测试约束](feedback-testing-a82c.md) — 集成测试必须连接真实数据库
```

每个 scope 的硬上限：

- 200 行；
- 25,000 bytes；
- 单条索引建议不超过 150 字符，超过 200 字符必须把详情移回主题文件；
- 超限时从自然换行处截断并向模型提供明确 warning；
- 索引损坏或缺失时从合法主题 frontmatter 重建，不从正文猜造新记忆；
- user/project 两份索引都可进入请求，但合计仍受上下文预算约束，project 索引优先。

### 4.3 主题文件

一个文件拥有一个稳定 `memory_id`，按语义主题组织而不是按时间组织。文件名可读且包含短 identity；rename 不改变 `memory_id`。

```markdown
---
schema_version: 1
memory_id: mem_01J...
name: 数据库测试约束
description: 集成测试必须连接真实数据库，避免 mock 与生产迁移不一致
type: feedback
status: active
revision: 3
updated_at: 2026-07-18T11:00:00Z
source_thread_id: thread_...
source_record_id: record_...
---

集成测试必须连接真实数据库，不使用数据库 mock。

**Why:** 过去发生过 mock 测试通过、生产迁移失败的问题。

**How to apply:** 修改 schema、migration 或 repository 时运行真实数据库测试。
```

约束：

- 必填字段只有 `schema_version`、`memory_id`、`name`、`description`、`type`、`status`、`revision`、`updated_at`、`source_thread_id` 和 `source_record_id`；
- `scope` 由文件所在根推导，user root 中的文件只能是 user scope，workspace root 中的文件只能是 project scope，不在 frontmatter 重复保存；
- `created_at`、`observed_at`、`expires_at`、`captured_by`、`supersedes` 和 `keywords` 均为可选字段；只有事实有明确观察时间、有效期、替代关系或检索补充词时才写入；
- `source_record_id` 当前由宿主填写为产生本次 mutation 的稳定 `tool_call_id`，可在对应 thread 的 SemanticJournal tool-call record 中反查；模型不能传入或覆盖；
- `source_run_id` 和 `source_turn_id` 不保存，存在来源时由 `source_thread_id + source_record_id` 反查；来源已删除时显示 unavailable，不猜造；
- `description` 是召回判断的主要文本，必须短、具体且与正文同步；
- `status` 只允许 `active`、`superseded`；只有 `active` 自动召回；
- update 要求 `expected_revision`，成功后 revision 递增；
- supersede 保留旧文件但从索引移除；需要显式替代链时在新文件的可选 `supersedes` 中记录旧 identity；
- 用户显式 delete 删除主题文件和索引入口；SemanticJournal 不保留正文副本；
- YAML 未知字段保留，非法必需字段使单文件 quarantine，不得按 legacy 规则猜 scope 或 identity；
- 文件和目录权限遵守 03 域 authority 数据要求；平台不能可靠原子替换时 mutation fail closed。

## 5. 召回合同

5.1 与 5.2 的请求期索引、递归候选扫描、主模型 selector、最近成功工具名、非阻塞预取、allowlist、自动注入和 session accounting 当前已接入主 run；compact 成功会复位召回预算。5.3 是正式 subagent 能力落地后的强制后置项，当前没有 child runtime，因此尚未实现；当前也尚未把显式 `memory get/update` 接触记录并入自动召回去重集合。

### 5.1 常驻索引

run 建立时读取允许 scope 的 `MEMORY.md`，按合计预算形成 host-owned index block。Memory 内容是数据，不是指令；任何文件内文本都不能改变 system prompt、权限、工具池、Skill 或 TaskScope。

### 5.2 相关主题选择

每个用户 turn 启动一次非阻塞预取：

1. 递归扫描允许 scope 的 `.md`，排除 `MEMORY.md` 和非 active 文件；
2. 每个主题只读取前 30 行 frontmatter；
3. 记录 filename、absolute path、mtime、description、type、scope、revision；
4. 按 mtime 新到旧排序，每个 scope 最多保留 200 个候选；
5. 过滤本 thread 已召回以及当前 run 已通过 Memory read/update 接触的文件；
6. 把当前用户 query、最近成功工具名和候选 manifest 交给 selector；selector 复用当前主模型 profile，不存在独立高低模型配置；
7. selector 用 JSON schema 返回最多 5 个合法 filename；不确定时返回空数组；
8. host 按 manifest allowlist 反查文件，不接受 selector 生成的任意路径；
9. 并行读取选中文件，每个最多 200 行、4,096 bytes；截断时保留开头并提示用 Memory read 读取完整内容；
10. 以 host-owned `<awaken_memory_context>` 数据块注入下一次可用的模型迭代。

召回总预算：

- 每 turn 最多 5 个主题、20 KiB；
- 同一未压缩 session 累计最多 60 KiB；
- compact 移除旧 Memory block 后，允许在后续 turn 重新召回；
- selector prefetch 不阻塞首个模型请求；准备完成时在当前 run 的下一次模型迭代消费，run 已结束则丢弃；
- selector、扫描或读取失败均降级为本 turn 零召回并产生有界 diagnostic，不伪造命中；
- selector 不调用 embedding，不依赖数据库；未来增加 lexical/vector 粗排时不得改变 Markdown authority、最多 5 条和 host allowlist 复核。

每条注入内容携带 `memory_id`、scope、type、updated/observed time、source identity 和陈旧提示。记忆描述代码、外部状态或日期时必须先验证当前事实；冲突时以当前可观察状态为准，并通过受控 update/supersede 修正 Memory。

### 5.3 Run 与 child Agent

本节当前不运行。正式 subagent 完成后，subagent 域必须先提供不可伪造的 parent/child identity、父 `MemoryRecallSnapshot`、canonical workspace 和权限快照，本域再按以下约束接入；不得以“child 自己再跑一次 selector”代替继承。

- 第一次成功召回形成 run-scoped `MemoryRecallSnapshot`；同一 run 不因磁盘变化替换已注入内容；
- run 内后续 turn 可追加新的 recall receipts，但不得无界重复同一文件；
- Memory mutation 的工具结果可供当前 Agent 使用，新内容从下一 run 才成为自动召回候选；
- child Agent 默认只继承父 run 已召回且与 child workspace scope 相容的 Memory identity；
- child 不单独扫描 user scope，不得扩大父边界；父显式允许时可在相同 project scope 发起新召回；
- child 的写入仍通过相同 Memory service 和 parent/child source identity，不能直接编辑文件。

## 6. 受控 Memory 能力

本节列出的单一 `memory` 工具与六种 action 当前已进入 run 固定工具快照。`supersede` 当前只将既有记录标为 superseded；“同时创建 replacement 并建立 supersedes 链”的复合事务尚未实现，需先 create/update 再显式 supersede。

模型只看到一个 `memory` 工具，明确 action：

```text
search(query, scope?, types?, limit?)
get(memory_id)
create(scope, type, name, description, content, keywords?)
update(memory_id, expected_revision, patch, reason)
supersede(memory_id, expected_revision, replacement, reason)
delete(memory_id, expected_revision, reason)
```

Memory service 是唯一 mutation owner：

- 从当前 run 填入 source identity，模型输入不能伪造；
- 校验 scope、canonical workspace、symlink、路径、frontmatter、UTF-8、大小和 revision；
- 在写入前执行 secret/sensitive-content gate；
- create 前扫描 description/keywords/正文近重复，存在重复时要求 update 或 supersede；
- 使用同目录临时文件、file fsync、原子 rename、directory fsync，再更新 `MEMORY.md`；
- 主题文件成功而索引更新失败时保留主题 authority，并把索引标记为需要重建；不能反过来只留下索引指针；
- delete 从主题文件与索引移除正文；thread receipt 只保存 identity/action/revision，不保存可复活正文；
- create/update/supersede/delete 给主模型的结果只包含完成状态、action、identity、scope/type、revision 和时间，不回灌刚保存的完整正文；search/get 仍返回完成当前任务所需的受控内容；
- 普通 `Read`/`Edit`/`Write` 对 Memory 根维持敏感/host-owned 限制，`Bash` 的当前用户权限事实不等于合法 Memory mutation，任何 Bash 改动在下次扫描时必须经过格式和 identity 校验。

Memory 工具进入 run 固定 ToolAvailabilitySnapshot。实现该工具意味着改变 04 域当前工具目录事实，必须同步更新 04 权威、测试和真实 Desktop 验收，不能只在 prompt 中宣称存在。

## 7. 主动记忆与 Dream

### 7.1 主 Agent 主动记忆

当前 Core 默认提供本节主动记忆指导和 `memory` 工具，由主模型在正常任务推理中自行决定是否调用；没有 terminal 后 extractor 或 extraction cursor。Settings 的 Memory、自动召回、主动记忆和 Auto Dream 四个开关均已持久化并接入现有行为；手动“立即整理”不依赖 Auto Dream 开关。

主动记忆默认开启，行为参考 `sourcecode` 主 Agent 始终获得 Memory 保存规则的路径：

- Memory 行为规则以独立的 host-owned policy block 提供；`MEMORY.md` 索引仍位于 `trust="data_only"` block，记忆数据不能伪装成行为规则；`memory` 工具随正常主 Agent 请求提供；
- 主模型在完成用户任务的同一推理过程中判断信息是否值得跨 thread 保存；只有判断应保存时才调用 create/update/supersede/delete；
- 不要求用户逐字说“记住”。“以后”“从今以后”“always”“call me”等未来持续表达是显式持久化信号；稳定姓名、称呼和双方命名属于 user scope 的协作偏好。例如“以后叫我博士哥，你叫小田”应在同一 turn 创建或更新一条 `feedback`，不能只因缺少“记住”二字而询问是否保存；
- Memory mutation 的本 turn 成功 receipt 是“已经持久化”的唯一 authority；纯文本确认不会写入磁盘。模型在没有成功 receipt 时不得声称“已记住、已保存或已更新”，明确未来持续指令必须先调用 Memory 再最终回复；
- 主动判断的正向信号包括：用户角色、目标、职责或知识背景；稳定协作偏好；用户纠正与对非显然做法的确认；无法从仓库推导的项目原因、负责人或期限；有持续用途的外部知识入口。信号必须由用户确认且对未来 thread 有实际价值；
- 意图清楚且内容非敏感时同轮执行 Memory，不先用“要不要帮你记住”把动作退回给用户。只有持久化意图确有歧义、内容可能敏感或与现有偏好冲突时才确认；
- 可能已有同主题时先 search，再 update；没有同主题才 create，避免把每次表述拆成重复 topic；
- mutation 成功后，工具向模型返回不含正文的明确 completion receipt、`write_completed`、`next_step` 和 finish next-action；成功意味着持久化已完成，模型停止额外 reasoning，不得对同一事实重复 mutation 或反复演练称呼，只输出一句确认后结束无剩余工作的 turn；
- 宿主不在每轮 terminal 后启动额外 extractor、不重复分析完整对话，也不维护 extraction cursor；
- 普通闲聊、临时任务状态、可从代码/文档/Git 推导的事实和不确定推断不触发写入；
- 用户明确要求记住或忘记时优先执行；隐式写入必须满足本域类型、敏感信息和重复检查，并在 Workbench 显示 receipt；
- Settings 可关闭“主动记忆”；关闭后模型不获得上述宽泛的主动信号指导，但逐字“记住/忘记”和语义明确的未来持续指令仍可显式调用 Memory；
- 主动记忆开关不控制召回。Memory 总开关和自动召回开关独立；关闭主动写入后，已有 active Memory 仍可正常召回。

这里不存在 Memory extraction checkpoint，也不复用 SSE `streamCursor` 或 SemanticJournal `seq`。未来若重新引入独立后台 extractor，必须另立设计和稳定 checkpoint 字段，不能把 transport cursor 当作提取进度。

### 7.2 Dream 目标

本节 7.2 的 plan/validate/apply、scope lock、手动启动/查询/取消当前已实现；7.3 的自动触发门槛、新 run 取消和每分钟 heartbeat 已实现；7.4 的跨进程 staged transaction、crash rollback、durable task/report 与重启恢复已实现。source journal 窄搜索仍未实现，Dream 当前只使用合法 Memory topic manifest。

Dream 是低频 Memory maintenance，不是第二个面向用户任务的 Agent loop。它负责：

- 合并同 scope、同类型、同事实的重复或近重复主题；
- 用新事实更新旧主题，并保留 Why/How to apply；
- 把相对日期改为绝对日期；
- 检测内容漂移、互相冲突和已被 source/current state 否定的条目；
- 将过时条目标记 superseded，从 `MEMORY.md` 移除；
- 修复悬空、重复或过长的索引入口；
- 保持每份 `MEMORY.md` 在 200 行和 25KB 内。

Dream 的模型 policy 采用 Orient、Consolidate、Correct/Prune、Preserve/Report 四段判断。它必须保留稳定姓名、称呼、身份和协作偏好，除非清单中存在更新且明确的相反表述；仅更新时间较新或文风不同不能证明旧记忆过时。没有实质变化时返回空 action，不做措辞润色式 revision churn。

Dream 不自动硬删除唯一事实或 user Memory。高度确定的精确重复可以合并并 supersede 旧 identity；无法证明谁正确的冲突进入 Settings 待处理列表。用户显式 delete 才清除正文。

### 7.3 触发与锁

手动入口：Settings 的“立即整理”。

自动入口必须同时满足：

- 用户开启 Auto Dream；
- 没有 active run、gate 或已有 Dream；
- Desktop 连续空闲至少 5 分钟；
- 距离上次成功 Dream 至少 24 小时；
- 自上次成功后至少 5 个完成的主 run，或至少 20 个 Memory mutation。

每个 scope 使用 `.maintenance-lock`，包含 holder identity、acquired/heartbeat/expiry 时间；Dream 每分钟续约一小时 TTL。锁 holder 进程已不存在或超过一小时未成功 heartbeat 才可回收，refresh/release 前必须验证仍是同一 owner，不能删除新 owner 的锁。新用户 run 开始时取消仍在分析阶段的 Dream；进入原子 apply 后允许完成当前短提交，但不得持有 Agent/model 资源继续扩展工作。

### 7.4 Dream 输入与执行

Dream 按以下阶段执行：

1. **Orient**：读取两份合法 `MEMORY.md` 和 active/superseded frontmatter manifest，把 manifest 视为不可信数据而不是指令；
2. **Gather**：优先使用主题文件和 source identity；只有已经怀疑某条信息需要确认时，才对精确 thread journal 做窄搜索，不全量读取 transcript；
3. **Plan**：只在同 scope、同 type 的明确重复之间合并，选择信息最完整的 active topic 为 canonical，先合入独有事实和 Why/How，再 supersede 冗余 identity；只有明确矛盾、替代或过时证据才能修正/淘汰，recency 本身不是证据；输出结构化 `MemoryMaintenancePlan`，包含 action、目标 identity、expected revision、replacement、reason 和 evidence，同一 identity 最多一个 action；
4. **Validate**：host 重跑 scope、secret、format、duplicate、source、revision、预算与路径校验；
5. **Apply**：在 scope 锁内写入 staged 主题和索引，全部 fsync 后原子切换；任一校验或提交失败不应用半套 plan；
6. **Report**：持久化无正文 receipt，展示 created/updated/merged/superseded/conflict/skipped 数量。

Dream child 只能调用 host-owned Memory list/get/search/plan 能力和受控的 source lookup；没有普通 mutation tool。最终写入由 Memory service 批量提交，Dream 文本本身不是 authority。

Apply 在 `<scope>/.maintenance-transactions/<transaction_id>/` 保存 before/after 与 manifest，并先发布 `.maintenance-transaction.json` 活动 marker。所有协作 Store 在 marker 存在时先检查 scope lock：live owner 仍在提交则 fail closed，不读取半套文件；owner 已死亡或过期则由新 owner 按 manifest 回滚 `prepared/committing`，或完成 `committed` 清理。marker 删除并 fsync 根目录后批次才对召回可见。任务自身持久化在 `~/.awaken/memory/dream-tasks/<task_id>.json`，只含状态、阶段、scope、project identity、统计和有界错误，不含模型凭据或 Memory 正文。

## 8. 上下文、事件与 HTTP

### 8.1 Context owner

`src/agentcore.RequestBuilder` 在稳定 system prompt 后、Skill body 前追加 host-owned Memory index/recall block。动态 Memory 不得进入静态 prompt cache identity；cache/delta 记录 scope、memory identity、revision 和内容 hash，不记录秘密或完整正文。

压缩只保留当前模型仍需要的 recall block、identity 和 source，不把 summary 反向保存为长期记忆。Dream 不从 `context_compacted` 自动生成 Memory。

### 8.2 SemanticJournal 与 wire

当前 SemanticKind：

```text
memory_recalled
memory_mutated
```

Dream 不伪装成 thread 内的 SemanticJournal record。维护任务的 durable authority 是 Dream task JSON；Desktop live 收到同一 task terminal receipt，重启或 thread replay 时从 `GET /v1/memory/dream` 按 task identity 重新投影。普通 recall/mutation 仍由 thread SemanticJournal 重放。

耐久 payload 只保存 memory IDs、scope、type、action、revision、status、source identity、counts 和 error code，不保存主题正文或 daily-log 原文。live 与 replay 通过 09 域唯一 projector 产生：

```text
AWAKEN_MEMORY_RECALLED
AWAKEN_MEMORY_CREATED
AWAKEN_MEMORY_UPDATED
AWAKEN_MEMORY_SUPERSEDED
AWAKEN_MEMORY_DELETED
AWAKEN_MEMORY_REJECTED
AWAKEN_MEMORY_DREAM_COMPLETED
AWAKEN_MEMORY_DREAM_FAILED
AWAKEN_MEMORY_DREAM_CANCELLED
AWAKEN_MEMORY_DREAM_INTERRUPTED
```

重放按 identity hydrate 当前 Memory：记录仍存在时显示当前 name/scope；已经删除时只显示“该记忆已删除”，不能从 journal 恢复正文。未知/旧 Memory event 按 09 域 unknown 容错。

### 8.3 HTTP 与 Desktop bridge

当前 route：

```text
GET    /v1/memories
GET    /v1/memories/{memory_id}
DELETE /v1/memories/{memory_id}
GET    /v1/memory/settings
PATCH  /v1/memory/settings
POST   /v1/memory/dream
GET    /v1/memory/dream/{task_id}
GET    /v1/memory/dream
POST   /v1/memory/dream/{task_id}/cancel
```

Desktop renderer 只能通过 typed preload/Main bridge 调用 loopback route；不直接读写 Memory 文件。project route 必须用 Core 已确认的 project/workspace identity，不能接受 renderer 任意绝对路径作为授权。

## 9. Desktop 产品行为

Settings 新增“记忆”页：

- Memory 总开关、自动召回、主动记忆和 Auto Dream；
- user/project、type、status、更新时间使用 Desktop 自绘且等宽的下拉筛选，不使用平台原生 `select` 菜单；项目范围只读取当前明确选择的一个 active project，没有当前项目时显示“（项目待选择）”，不跨全部项目聚合；项目菜单限定高度并滚动；
- 搜索或筛选条件变化时立即清空已选详情和旧列表，丢弃晚到的旧列表/详情请求；查看正文和来源后允许用户按 revision 删除已有记忆；Settings 不提供创建、编辑或 supersede；
- 最近 Dream 时间、统计、冲突和“立即整理/取消”；
- Dream 排队、运行、完成、失败和取消状态均使用中文产品文案；
- store/索引损坏、来源缺失和陈旧提示。

Workbench：

- recall 在执行过程显示“已召回 N 条记忆”，进入 execution-process 折叠，不平铺正文；
- create/update/supersede/delete 显示明确 receipt；
- 主动记忆和 Dream 完成显示紧凑通知，不创建通用 approval 状态；
- Dream 不占用用户当前 turn，不向 final answer 注入维护推理；
- 右侧 Inspector 当前不承载 Memory 详情；
- 页面切换、重连和重启按 durable identity 保持一致。

## 10. 异常、恢复与不变量

| 场景 | 目标行为 |
| --- | --- |
| 单个主题 frontmatter 损坏 | quarantine 该文件，其他合法 Memory 可召回；Settings 显示诊断，不覆盖损坏文件 |
| `MEMORY.md` 缺失/损坏 | 从合法 active frontmatter 重建；重建失败则本 scope recall fail closed |
| selector 失败/超时 | 本 turn 零召回，主 run 继续；记录 bounded diagnostic |
| 主题读取在选择后变化 | revision/hash 不一致则跳过，不注入选择时未审核的新内容 |
| create/update 写入失败 | Memory 工具失败，不发成功 receipt，不更新模型内 Memory truth |
| 索引写入失败 | 主题 authority 保留并标记 index dirty；重建前 Settings 明示，不能只留下悬空索引 |
| Dream 被新 run 取消 | staged 内容清理，锁释放；无 live authority 变化 |
| Dream plan revision 冲突 | 冲突项跳过或整批失败，不能覆盖用户刚编辑的 Memory |
| Core/Desktop 重启 | 锁按 holder/heartbeat 恢复；未提交 staged 数据不参与召回 |
| 用户删除来源 thread | Memory 保留，source 标记 unavailable；不从其他 thread 猜来源 |
| 用户删除 Memory | 主题正文与索引入口移除；journal receipt 不保留正文副本 |

硬不变量：

- Markdown 主题文件是 Memory 内容唯一 authority；数据库、embedding、manifest、renderer state 都不是；
- `MEMORY.md` 是索引，不是主题正文容器；
- 最多 5 条相关主题进入一次召回，失败可以零命中；
- 当前代码、正式文档和外部实时状态优先于旧 Memory；
- Memory 不改变工具、权限、Skill、TaskScope 或 provider snapshot；
- Memory mutation 只能经过 host-owned service；
- Secret、认证材料、完整 prompt、完整 transcript 不进入 Memory；
- Dream 不建立第二 Agent loop，不自动硬删除不确定事实，不绕过 revision 和原子提交；
- 依赖正式 subagent 的 child 继承与尚未实现的 source journal 窄搜索必须明确标为缺口，不能由主 Agent 召回或手动 Dream 入口推断为已支持。

## 11. 代码 owner

| 层 | owner |
| --- | --- |
| Markdown parser/store/index/lock | `src/memory` |
| recall selector、prefetch、snapshot | `src/memory`、`src/runtime` |
| request context injection | `src/agentcore/request_builder.go`、context envelope |
| model-visible Memory tool | `src/tools/memory`、`src/tool` snapshot/policy |
| 主动记忆指导、Dream plan/apply | `src/memory`、`src/runtime`；child orchestration 复用 `src/agentcore` |
| SemanticRecord 与 replay | `src/agentcore`、`src/runtime` |
| HTTP/SSE | `src/server`、`src/protocol/agui` |
| preload/Main bridge、Settings、Workbench | `apps/desktop` |
| UI-only primitives | `Awaken Design System`，不得读取 Core/文件/store |

实现不得把 store、召回、安全判断或 Dream scheduler 放进 renderer，也不得让 Design System 持有业务状态。
