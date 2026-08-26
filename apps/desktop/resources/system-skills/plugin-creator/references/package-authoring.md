# Agent Plugin 完整包创作检查

只在创建、修改或审查 Plugin 候选目录时读取本文件。

## 便携核心与客户端边界

Agent Plugins 1.0.0 的便携核心只有：

- 根 `plugin.json`；
- `skills/` 的直接子 Skill 包；
- 根 `mcp.json`。

安装来源、升级、权限确认、UI、OAuth、marketplace 和客户端扩展行为由宿主拥有。Awaken 不把 `.codex-plugin`、`.claude-plugin` 或 `.cursor-plugin` 当作便携 manifest。

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

## 完成检查

- manifest 和 mcp schema 使用相同的 1.0.0 版本；
- 每个 Skill 的 `name` 与目录一致；
- 没有空资源目录、占位正文、绝对作者路径或未声明秘密；
- 每个意图组件都被 Core 计入 loadable Skill/server；
- 新/变脚本和 stdio MCP 已实际运行；
- 候选预检和 Core validate 均通过；
- 安装后新 run 能列出 Skill 文件树、读取资源并完成无害 MCP 调用。
