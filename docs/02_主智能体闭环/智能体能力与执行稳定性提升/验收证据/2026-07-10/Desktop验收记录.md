# Phase 8 真实 Desktop 验收记录

验收日期：2026-07-10（Asia/Shanghai）

验收入口：根 README 规定的 `npm run close:desktop` / `npm run open:desktop`

真实窗口：`Awaken Desktop`，1440×900；未使用浏览器 Vite 页面、组件 Gallery 或其他 Agent 产品代替。

隔离工作区：`/tmp/awaken-acceptance-workspace`，只包含本次构造的无敏感数据样例与项目 Skill。

当前状态：macOS 真实 Desktop 验收通过；跨平台发布门禁未通过，原因是尚无 Windows 真实 Desktop / Windows CI 验收证据。

## 1. 既有历史 LLM 文本恢复

目标会话：旧版 `外加工成本风险分析` transcript。

结果：重启后完整中文 assistant 最终回答可见，未再显示 `Streaming response...`。旧 transcript 的正文没有丢失；问题根因是同一 SemanticRecord 投影出的 START/CONTENT/END 帧共用 seq，被 Desktop 去重错误丢弃 CONTENT/END。

证据：`既有历史文本恢复截图.png`。

## 2. 显式 Skill 与工具语义任务

Thread：`thread_1783666774508595000_2`

Run：`run_1783666774508587000_1`

显式选择项目 Skill：`phase8-acceptance`。

Skill 契约：

- required resource：`references/验收要求.md`，mode=`preload`
- allow：`read_file`、`text_edit`、`shell`
- deny：`apply_patch`
- required runtime：`python`

耐久激活结果：

- `skill_activated` 1 条
- `resource_receipt` 1 条
- receipt SHA256：`cced72e01b17398c6de7b2bf156b554df37a3ad0f1b7e10c6b46e74a5801d3b5`
- receipt size：685 bytes
- effective tools：`read_file`、`shell`、`text_edit`

真实工具结果：

| 动作 | status | semantic | process / exit | error |
|---|---|---|---|---|
| read_file | succeeded | success | not_applicable | — |
| text_edit 唯一替换 | succeeded | success | not_applicable | — |
| `printf` | succeeded | success | exited / 0 | — |
| `sh -c 'exit 7'` | failed | failure | exited / 7 | `process_exit_nonzero` |
| `rg` 无匹配 | succeeded | neutral | exited / 1 | diagnostic=`no_match` |

最终 journal：35 条 semantic records；user=1、assistant=6、ToolCall=5、ToolResult=5、Artifact=1、file_change=1、terminal=1。

证据：`工具活动可见性截图.png`、`工具失败状态截图.png`、`重启前历史截图.png`、`重启后历史截图.png`。

## 3. required resource 缺失 fail-closed

Thread：`thread_1783666963135726000_4`

显式选择 `phase8-missing-resource`，其 required resource 故意指向不存在的 `references/不存在.md`。

结果：约 2 秒内返回 `skill_resource_missing`；journal 只有 thread、run_started、failed skill activation、run_terminal 四条记录。assistant_message=0、tool_call=0，没有进入首次 LLM，也没有副作用。

证据：`Skill缺资源阻断截图.png`。

## 4. 中文 PPT typed runtime

首轮 Thread：`thread_1783667002920164000_6`。

约束：禁止 shell、Python、生成脚本、pip、手写 OOXML/ZIP；使用 typed presentation tools 创建并渲染 5 页中文 PPT。

已确认的正向事实：

- 没有 shell / pip / 动态 Python。
- `presentation_create` 真实调用 Awaken-owned Python、LibreOffice 与 Poppler 完成创建、PDF/PNG 渲染和验证；渲染链是该 typed tool 的内部受控步骤。
- 5 个 PNG 均为 1920×1080；PPTX ZIP 完整；5 个 slide XML；`a:ea typeface="PingFang SC"` 存在；PDF 为 5 页、16:9。
- 初版第 4 页 overflow 没有被当成最终成功；重复的完全相同 edit 被 FailureSupervisor 阻断，Agent 随后切换到重建策略，warnings 清零。

首轮同时暴露发布阻断：schema 接受 comparison 三列，但 engine 只渲染前两列，第三列被静默丢弃；部分 layout 接受 body 却不渲染。该轮证据保留为 `PPT首次验收暴露静默丢列截图.png` 与 `PPT首次验收第3页缺列.png`，不作为最终通过证据。

最终复验：

- Thread：`thread_1783671075115597000_2`
- Run：`run_1783671075115586000_1`
- 输出：`/tmp/awaken-acceptance-workspace/outputs/Phase8中文运行时验收.pptx`
- PPTX SHA256：`2c1f05a3c90f009cc3ef4dfc02382421dd68d00f21e6e8b00c5d190087831453`
- PDF SHA256：`265dd255ffef7b9be60db92bdc1bfe3c4856719633d82a12bc5102c33974b0e4`
- `slide_count=5`、`pdf_page_count=5`、`png_count=5`
- `verified=true`、`render_check=libreoffice_pdf_poppler_png`、`warnings=[]`
- 第 3 页真实渲染三列，每列 4 条内容；第 4 页真实渲染 4 步时间线；逐页目视检查无空白、裁切、重叠或缺列。
- journal 中实际只有 1 次 `presentation_create` ToolCall；LibreOffice、Poppler、PDF/PNG 渲染是该工具内部受控验证链，并非第二次独立 `presentation_render` ToolCall。最终回答把内部链路写成 `presentation_render` 工具调用属于模型表述不精确，验收以 journal 为准。
- create/edit 已改为同目录 staging，只有 package、LibreOffice、Poppler 与视觉规则全部通过后才 fsync + atomic publish；失败不遗留新 final，原地编辑失败保持原 SHA，preview swap 支持回滚。已有 preview 目录只允许本 deck 的 PDF/pagecount/page-N.png 普通文件；无关文件、symlink 或特殊节点会在任何 rename 前 fail-closed，避免删除用户文件。

最终证据：`PPT产物与渲染验证截图.png`、`PPT最终验收_重启后历史重放.png`、`PPT渲染第1页.png`、`PPT渲染第3页.png`、`PPT渲染第4页.png`、`PPT渲染第5页.png`、`Phase8中文运行时验收.pptx`、`Phase8中文运行时验收.pdf`。

## 5. live/replay 时长

首轮工具任务直播显示 48s，重启重放显示 43s。根因是直播使用客户端提交/SSE 到达时间，重放使用耐久 RUN_STARTED/RUN_FINISHED 时间。

修复后真实 smoke：

- Thread：`thread_1783667891585834000_2`
- durable RUN_STARTED：`2026-07-10T07:18:11.869266Z`
- durable run_terminal：`2026-07-10T07:18:26.540768Z`
- 直播 UI：15s
- 重启重放 UI：15s

证据：`时长重放修复_重启前.png`、`时长重放修复_重启后.png`。

## 6. RuntimeManager

`node scripts/check-primary-runtime.mjs --json`：ready=true、missing=[]、provenance.ok=true、issues=[]。

Active revision：`0.2.0/darwin-arm64/build-mrektar3-1dce472ed7`。

Python、Node、LibreOffice、Poppler、ffmpeg、helper-packages 全部 probe 通过；Go preflight 与 hostless 隔离 preflight 通过。

证据：`Runtime状态.md`、`Runtime状态截图.png`。

## 7. 重启重放对账

完成态 journal 的核心耐久事实对账：

| Thread | user | assistant | call/result | artifact | recovery | terminal |
|---|---:|---:|---:|---:|---:|---:|
| 工具/Skill | 1 | 6 | 5 / 5 | 1 | 0 | 1 |
| PPT 首轮 | 1 | 9 | 15 / 15 | 20 | 2 | 1 |
| PPT 最终复验 | 1 | 1 | 1 / 1 | 7 | 0 | 1 |

工具名、参数、status、error code、exit code、duration、Artifact 路径、terminal result/reason 在 durable replay 中保留。最终 PPT 直播与重启重放均显示 `执行时间 47s`；重启后完整中文 assistant_message、ToolCall/ToolResult、7 个 Artifact 均可见，没有 `Streaming response...`、重复卡片或状态漂移。

运行中 transcript→SSE 的 `streamCursor` / `semanticSeq` 分域、`after_cursor` 与旧 `after_seq` 兼容通过 server 集成测试；本轮没有通过人工断网制造真实 SSE 断线，因此只把它记为自动化续接通过，不伪称手工断线验收。

## 8. 失败工具不产生假 Artifact，neutral diagnostic 可见

在最终 build 后再次按正式入口关闭并打开 Desktop，执行一条严格限制为两次工具调用的真实 smoke：

- Thread：`thread_1783673503498909000_2`
- Run：`run_1783673503498904000_1`
- `text_edit` 尝试替换不存在的 `PHASE8_TEXT_NOT_FOUND_SENTINEL`：`status=failed`、`semantic_status=failure`、`error_category=domain`、`error_code=text_not_found`、`match_count=0`，文件 SHA 保持不变。
- `rg 'PHASE8_DIAGNOSTIC_NOT_PRESENT' 验收样例.txt`：`exit_code=1`、`status=succeeded`、`semantic_status=neutral`、`diagnostic.reason=no_match`，不是系统故障。
- journal 共 15 条 semantic records：ToolCall=2、ToolResult=2、assistant_message=2、terminal=1、Artifact=0、file_change=0；没有重试，也没有假文件变更卡。
- Desktop UI 显示完整模型正文、`text_not_found` 和 `diagnostic: no_match`，执行时间 39s。

证据：`失败工具无假Artifact与中性诊断.png`；截图 SHA256：`5b80f0506b9ed832d267deb79729d3dee638f957fb2e768472efae5bd1f00ca8`。

### 8.1 最终 HEAD 复验

在 Phase 8 验收修复全部拆分提交并进入 `origin/main` 后，又以 `0c4f0646`（`feat: tighten failure-supervisor class budget and runner integration`）为输入 HEAD 完成一次独立复验，确认先前证据不是停留在修复前工作树上的偶然结果。

- 复验时间：2026-07-10 21:05–21:10（Asia/Shanghai）。
- 正式入口：先执行 `npm run close:desktop`，再执行 `npm run open:desktop`；启动检查确认 screen、renderer、Electron、awaken-core 和 `Awaken Desktop` 1440×900 真实窗口均就绪。
- 隔离工作区：`/tmp/awaken-acceptance-workspace`。
- Thread：`thread_1783688891557345000_2`。
- Run：`run_1783688891557339000_1`。
- `text_edit` 只调用一次，得到 `semantic_status=failure`、`error_code=text_not_found`、`match_count=0`；`验收样例.txt` 在调用后及重启后 SHA256 均为 `106b537c0c0147e0cdc678d7b89623eca692f9370891737e202dcdce695c081a`。
- `shell` 只调用一次，命令为 `rg 'PHASE8_DIAGNOSTIC_NOT_PRESENT' 验收样例.txt`；进程退出码为 1，但投影为 `semantic_status=neutral`、`diagnostic.reason=no_match`。
- transcript API 在直播与重启后都只有 2 个 ToolCall/ToolResult，Artifact 事件 0、文件事件 0，终态为 `RUN_FINISHED / completed / success`。
- Desktop UI 直播与重启重放均显示完整中文最终回答、`text_not_found`、`neutral`、`no_match`、无文件修改、无 Artifact，执行时间均为 27s；没有 `Streaming response...`、重复工具卡或状态漂移。

复验证据：`最终HEAD复验_重启前.png`（SHA256 `9a4c71126e4da4090c543bd39099f6e176df7d644d0c49401637e0c835e93d69`）、`最终HEAD复验_重启后.png`（SHA256 `2c2eed1732f4eae09f345222b5a1ba60fc8dc9f86af97136937c4213c161f966`）。

## 9. 最终结论

macOS 结论：通过。正式入口、旧历史文本、工具/Skill 过程、受控失败、中性结果、required resource fail-closed、中文 PPT、Awaken-owned runtime 与重启重放均有真实 Desktop 或真实 runtime 证据。

跨平台结论：不通过。Windows 已完成 `GOOS=windows GOARCH=amd64` 构建，以及通用 APPDATA、platform-relative-path 和 shell PATH fixture；Windows 实机/CI、盘符/反斜杠/空格/非 ASCII 路径、`current.json` 原子发布的 Windows CI、`soffice.exe` / `pdftoppm.exe` 实际 probe，以及 `npm run dev:desktop` 真实窗口验收仍未完成。Owner：Desktop/Runtime Windows 平台验收；目标日期：2026-07-17。
