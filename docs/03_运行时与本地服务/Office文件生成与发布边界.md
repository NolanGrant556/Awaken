# Office 文件生成与发布边界

## 1. 当前事实

Core 当前没有 Office 后台 worker，也不注册 typed Office/PDF 生成或渲染工具。Awaken 不复制、维护或托管 LibreOffice、Python、Poppler、FFmpeg 等完整 Work runtime。Agent Plan 的 `image_generate` / `video_generate` 是独立媒体 provider，不属于 Office/PDF Work runtime 或 `artifact_publish` 文档发布链。

Agent 可按 Skill 使用当前用户环境中的 Python/Node/LibreOffice/Pandoc 等工具，通过 `Bash` 或 `Write` 处理和生成 Office/PDF 文档。bundled Office helper 支持 Python 3.9 及以上，并由各格式 doctor 按 route 返回本轮应复用的 Python/Node/LibreOffice/Poppler 绝对执行计划；这只是 Skill 内的依赖探测，不是 Core typed preflight。当前 Word 阅读由纯 Python 标准库 `read_docx.py` 解析 DOCX/DOTX OOXML，并把未完整覆盖的视觉或复杂结构写入 warning；接受修订、Excel 重算和 Word/PPT 视觉 QA 脚本各自拥有独立 LibreOffice profile、超时和结果合同。这些能力只在 Agent 显式执行对应 Skill route 时成立，也不意味着 Desktop 启动时已经准备 Python、LibreOffice 或 Poppler。其他缺失依赖仍按目标 route 的真实 doctor/执行错误恢复；没有执行对应脚本和逐页检查时，不获得版式验证或视觉 QA 证明。

受支持文档生成完成后必须调用 `artifact_publish`，由 04 域验证普通文件、symlink、扩展名和基础签名，并生成 deliverable Artifact；文件目录不是发布边界。发布证明文件身份和交付状态，不证明 Office 视觉质量。图片/视频不走这条链：专用媒体工具成功时自行产生 ready 媒体 Artifact，Bash/Skill 单独生成的任意媒体文件当前没有通用手动发布入口。

## 2. 不变量

- Desktop/Core 启动不以 LibreOffice、Poppler 或 Office Python 包存在为前提。
- `Write`/`Edit` 不自动发布 Artifact。
- Bash 进程成功或文件存在不等于格式、版式或业务内容通过。
- Office `--json` helper 不得连接会改变退出码或截断结果的 shell 管道；Bash outcome 与完整 JSON `status/issues` 必须同时通过。helper 失败后不得用裸 `soffice`/`pdftoppm` 绕过独立 profile、超时、原子替换和页数一致性合同。
- 旧 typed Office Artifact 记录继续可重放；兼容 Renderer 不代表旧 producer 仍可调用。
- 新增生成 runtime 前必须重新建立依赖、取消、原子发布、格式验证和真实 Desktop 视觉契约，不能复用历史文案冒充当前支持。

## 3. Owner 与验收

- `src/tools/artifact_publish` 与 04 域 Artifact 契约：当前发布链。
- `src/tool/environment_snapshot.go`、`src/tools/bash`：当前用户依赖发现与执行。
- `src/runtime/journal_store.go`、Desktop reducer：历史 Artifact 回放。

验收必须区分“生成命令成功”“基础文件校验通过”“Artifact 发布成功”“逐页视觉通过”四层事实。逐页视觉通过还必须能列出本轮实际读取的最终页图；未读取页、旧预览或只打开部分页面不能计入覆盖。没有真实应用打开或渲染证据时，只能声明前三层实际通过项。
