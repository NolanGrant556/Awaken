---
name: plugin-creator
version: 1.3.0
description: 创建、修改或校验 Awaken 的 Agent Plugins 1.0.0 包时使用。
user-invocable: true
context: inline
---

# Awaken Plugin Creator

创建便携 Agent Plugins 1.0.0：根 plugin.json、可选 skills/ 与 mcp.json。插件是包管理与来源容器，Skill/MCP 执行仍由宿主已有 runtime 拥有；本 Skill 的创作范围不等于其他方言的全部产品能力。

## 按需路线

- 设计完整 Skill 与 MCP 边界：读 [包设计](references/package-authoring.md)。Skill 入口只保留目标、路由与关键约束；条件性内容进入 references/，实际重复逻辑进入 scripts/，交付底稿进入 assets/。
- 新建或修改 manifest/schema：读 [包结构与脚手架](references/包结构与脚手架.md)，使用 `scripts/init_plugin.py` 与 `scripts/validate_plugin.py`。
- 安装、候选替换或已授权在线修改：读 [安装与修改](references/安装与修改.md)，只执行匹配分支。
- 简单 Skill 无需额外目录；窄修改保留未涉及资源，不重新初始化完整插件。

## 安全与身份

默认在任务工作区普通候选目录新建／修改；修改先 inspect，再 export 完整包。只在用户已经明确授权直接修改准确已安装 Plugin 时写 inspect 返回的 root_dir；首次写入前固定 recorded digest，最后以该值调用 commit。不得改 PLUGIN_DATA、plugins.json 或其他插件。

候选校验与安装经 Core PluginStore；同名默认 abort，用户已明确要求修改该插件且完整候选验证通过时可 replace。旧包由 Core Trash 处理，保留 PLUGIN_DATA；不得绕过 digest/CAS、组件诊断或安装器。

stdio MCP 以当前用户权限运行。安装／替换／commit 前说明实际 command、端点和权限事实，不能把结构验证当作运行隔离。密钥不写入包、Renderer 或日志。

## 完成条件

通过 `${AWAKEN_SKILL_DIR}` 绝对路径执行预检和变更脚本；Core validate／commit 是最终包校验，必须核对所有意图组件，不把被跳过的 Skill/server 当成功。MCP 改动验证 initialize/tools list，范围外外部动作不由包创作授权。

授权安装完成后报告精确 identity、旧／新 digest、组件与已验证项。新 run 才取得新工具／Skill 快照：用 `skill_resource_list` / `skill_resource_read` 复核实际所需资源，再执行无害的真实 MCP 调用。需要下一 run 是宿主快照边界，不以直接 Bash／HTTP 冒充新版本验收。

来源漂移、并发 CAS、无效组件或新增副作用超出授权时保留候选并报告具体阻塞；可恢复的候选错误应修正后继续。只要求候选或校验时不擅自安装，结果明确限制在实际完成范围。
