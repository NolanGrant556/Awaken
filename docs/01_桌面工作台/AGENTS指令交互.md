# AGENTS 指令交互

## 1. 用户入口

Settings 导航的“全局规则”栏目拥有全局 `AGENTS.md` 编辑入口。页面使用极简文档布局，顶部只用一行显示文件图标、文件名和启用状态，不向用户展示本机路径；其余只保留 Markdown 编辑器、UTF-8 字节用量和底部操作，不重复展示功能简介、路径卡片、覆盖关系卡片或独立安全说明。编辑器显示 16 KiB 模型注入上限，使用受控高度且不允许纵向拖大，长内容在编辑器内部使用与设置框架一致的轻量滚动条，常用窗口高度不因编辑器产生右侧整页滚动条。冲突时项目规则优先、仅影响新任务和不得写入密钥三项必要提示压缩为底部单行；保存/删除、revision 冲突恢复和成功状态复用同一状态位置。

项目文件继续由右侧文件工作区拥有。`.awaken/AGENTS.md` 随普通目录树自然展示并复用 Markdown 预览/编辑；主文件夹缺失该文件时显示“创建 AGENTS.md”。Main 必须重新查询 active project 和当前 primary folder，原子创建 `.awaken/AGENTS.md`，初始内容仅为 `# 项目指令`，绝不覆盖已有文件；成功后直接打开编辑标签。不在添加项目时自动创建。

次文件夹中的同名文件是普通 Markdown：文件行明确标记“普通 Markdown”，可显示、编辑，但不显示生效标识，也不计入当前运行来源。

## 2. 保存与冲突

全局页面使用 Core `expected_revision` CAS。收到 `INSTRUCTION_REVISION_CONFLICT` 时保留本地草稿，提示重新加载后人工合并，不静默覆盖外部内容。

项目 Markdown preview 返回 `contentDigest`。保存 `.awaken/AGENTS.md` 必须携带 `expectedDigest`；Main 在原子 replace 前复核当前 digest。外部内容已变化时返回 `FILE_CHANGED`、current digest 以及 current/submitted content，编辑器保留两个版本并允许重新加载或恢复本地草稿。其他 Markdown 保持现有保存语义。

## 3. 运行摘要与界面可见性

Composer 输入框下方不显示 AGENTS 来源数量，RunStage 也不提供本次快照来源面板。用户只通过 Settings 的“全局规则”和右侧项目文件工作区管理规则；保存、删除或切换主文件夹仍只影响下一次运行。

Renderer 仍接收并保留安全的 `instruction_snapshot` 摘要，供 SSE 重连、transcript replay 和运行恢复保持一致，但不展示正文、来源路径卡片或独立 Composer 状态。来源加载失败仍不会阻断 Composer 提交；旧 run 没有摘要时不补造。

## 4. Owner、恢复与验收

- Settings 产品状态：`apps/desktop/src/renderer/settings/GlobalAgentsSettingsPanel.tsx`。
- 项目创建和原子文件操作：`apps/desktop/src/main/index.ts`、`project-file-preview-manager.ts`。
- typed bridge：`apps/desktop/src/shared/runtime.ts`、`preload/index.ts`、`renderer/runtime/client.ts`。
- 运行摘要 replay：`renderer/runtime/reducer.ts`；`renderer/App.tsx` 不把摘要投影到 Composer 或 RunStage overlay。
- Design System 只拥有 Settings 导航 item 和通用 Composer/RunStage 容器，不读取文件或 Runtime。

真实 Desktop 必须覆盖：全局保存/删除/冲突、主文件夹项目创建不覆盖、次文件夹普通 Markdown、项目外部修改冲突、全局+项目同时存在、主文件夹切换、运行中编辑、SSE 重连与完整重启回放，并确认 Composer 下方不出现 AGENTS 来源入口或快照弹层。
