# Office 文件生成与发布边界

## 1. 当前事实

Core 当前没有 Office 后台 worker，也不注册 typed Office/PDF/image/media 生成或渲染工具。Awaken 不复制、维护或托管 LibreOffice、Python、Poppler、FFmpeg 等完整 Work runtime。

Agent 可按 Skill 使用当前用户环境中的 Python/Node/LibreOffice/Pandoc 等工具，通过 `Bash` 或 `Write` 生成文件。缺依赖时使用真实安装/执行错误恢复；该路径不获得内建版式验证、独立 LibreOffice profile 或逐页 preview 保证。

生成完成后必须调用 `artifact_publish`，由 04 域验证 output root、普通文件、symlink、扩展名和基础签名，并生成 deliverable Artifact。发布证明文件身份和交付状态，不证明 Office 视觉质量。

## 2. 不变量

- Desktop/Core 启动不以 LibreOffice、Poppler 或 Office Python 包存在为前提。
- `Write`/`Edit` 不自动发布 Artifact。
- Bash 进程成功或文件存在不等于格式、版式或业务内容通过。
- 旧 typed Office Artifact 记录继续可重放；兼容 Renderer 不代表旧 producer 仍可调用。
- 新增生成 runtime 前必须重新建立依赖、取消、原子发布、格式验证和真实 Desktop 视觉契约，不能复用历史文案冒充当前支持。

## 3. Owner 与验收

- `src/tools/artifact_publish` 与 04 域 Artifact 契约：当前发布链。
- `src/tool/environment_snapshot.go`、`src/tools/bash`：当前用户依赖发现与执行。
- `src/runtime/journal_store.go`、Desktop reducer：历史 Artifact 回放。

验收必须区分“生成命令成功”“基础文件校验通过”“Artifact 发布成功”“逐页视觉通过”四层事实。没有真实应用打开或渲染证据时，只能声明前三层实际通过项。
