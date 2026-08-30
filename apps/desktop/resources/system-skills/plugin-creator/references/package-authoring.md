# Agent Plugin 完整包创作检查

在创建、修改或审查 Plugin 候选目录，或用户在当前请求已明确授权直接修改已安装 Plugin 时读取本文件。

## 便携核心与客户端边界

Agent Plugins 1.0.0 的便携核心只有：

- 根 `plugin.json`；
- `skills/` 的直接子 Skill 包；
- 根 `mcp.json`。

安装来源、升级、权限确认、UI、OAuth 和客户端扩展行为由宿主拥有。Awaken 不把 `.codex-plugin`、`.claude-plugin` 或 `.cursor-plugin` 当作便携 manifest。

## 内含 Skill 不是单文件

每个 `skills/<name>/` 都是独立 Agent Skills 包。入口只放共同流程、路由和硬边界；条件性详情放入 supporting resources：

```text
skills/<name>/
├── SKILL.md
├── references/
├── scripts/
├── assets/
└── ...
```

API/MCP 插件优先考虑：

- 工具/用户意图映射；
- 输入字段与主数据来源；
- 多阶段顺序、确认、幂等与验证；
- 错误分类、重试、补偿、回滚和停止条件；
- 脱敏请求/返回示例；
- 只有 MCP 未提供且确需复用的辅助脚本。

`SKILL.md` 必须说明何时读取哪个文件。未被入口或其他已路由资源引用的文件需要删除、补路由或说明由脚本/交付物直接使用。

## MCP 边界

- `command` 是一个 executable token，不是 shell command；
- 包内 command 使用 `./` 路径并保持执行意图；
- `${PLUGIN_ROOT}` / `${PLUGIN_DATA}` 只在标准允许的字段展开；
- headers 是可见包数据，不存凭据；
- MCP server 拥有认证、实时数据与受控动作，Skill 只拥有选择、编排与验证；
- stdio 子进程以当前用户身份运行，安装前必须披露。

## 已安装包的在线修改边界

- Host 在技术上允许文件工具写整个 `<AWAKEN_HOME>/plugins` 仓库，但这只是能力边界，不是用户意图授权；Creator 仍只处理本次明确授权且经 inspect 固定 identity 的准确 `PLUGIN_ROOT`。
- 候选 export/validate/replace 是默认修改路径；只有用户在当前请求明确授权时，才直接修改 inspect 返回的准确 `PLUGIN_ROOT`。
- 第一次写入前固定 inspect 返回的 recorded digest；不修改 `PLUGIN_DATA`、`plugins.json` 或其他插件。
- 全部写入与预检完成后，用 `plugin action=commit` 和旧 `expected_digest` 让 Core 完整校验并 CAS 签收新 digest。直接写入到 commit 成功之间，该包应因 digest 未签收而 fail closed。
- commit 失败不更新 recorded digest；`plugin_recorded_digest_changed` 必须重新 inspect 并停止，不能用安装树的当前 digest 强行签收。
- 只有 commit 返回新 digest、正确组件计数且无未处置诊断后，才把在线修改写成已生效；真实 Skill/MCP 验收仍在新 run 执行。

## 完成检查

- manifest 和 mcp schema 使用相同的 1.0.0 版本；
- 每个 Skill 的 `name` 与目录一致；
- 没有空资源目录、占位正文、绝对作者路径或未声明秘密；
- 每个意图组件都被 Core 计入 loadable Skill/server；
- 新/变脚本和 stdio MCP 已实际运行；
- 候选路径的预检和 Core validate 均通过，或授权在线路径的预检和 Core commit 均通过；
- 安装后新 run 能列出 Skill 文件树、读取资源并完成无害 MCP 调用。
