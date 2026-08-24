---
name: plugin-creator
version: 1.0.0
description: 创建、校验或修改 Awaken Agent Plugins 1.0.0 包；支持内含 Skills 与 MCP 配置，始终在任务工作区维护完整候选包并通过 Core PluginStore 安全安装或替换。
when_to_use: 用户要求创建、编写、验证、修复、更新或修改 Awaken Plugin，或询问候选 Plugin 及其安装路径、Skills、MCP 配置是否合规时。
user-invocable: true
context: inline
---

# Awaken Plugin Creator

Awaken Plugin 是一个包管理与来源容器，不是第二套 Skill 或 MCP runtime。当前只创建 Agent Plugins 1.0.0：根 `plugin.json`，可选直接子级 `skills/` 和根 `mcp.json`。不要生成 Codex/Claude/Cursor 的 legacy manifest，也不要声称支持 marketplace、commands、agents、apps、LSP 或自动 Hook。

## 安全边界

- 候选包放在任务工作区的普通目录，例如 `plugin-authoring/<plugin-name>/`；不要在 `~/.awaken/plugins` 内创作。
- 修改已安装 Plugin 时，先用 `plugin` 的 `inspect` 确认身份，再用 `export` 导出完整候选包。不要只复制 manifest 或直接编辑 `root_dir`。
- `plugin action=validate` 是同一 PluginStore/runtime parser 的无写入校验；安装前必须成功且没有 error diagnostics。
- 同名安装默认 abort。只有用户明确要求修改该插件、完整候选已复核时才可 `replace: true`；旧包由 Core 移入系统废纸篓，`PLUGIN_DATA` 跨替换保留。
- Plugin 中的 stdio MCP 子进程以当前用户身份运行，当前没有额外 Seatbelt/WorkspaceExecutionBroker 沙盒。安装前必须向用户列出 command、网络端点和这一权限事实。

## 当前包结构

```text
<plugin-name>/
├── plugin.json
├── skills/
│   └── <skill-name>/SKILL.md   # 可选；只发现直接子目录
└── mcp.json                    # 可选
```

最小 manifest：

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "acme-tools",
  "version": "1.0.0",
  "description": "Reusable Acme workflows"
}
```

- `name` 为 1–64 字符，只用 `a-z`、`0-9`、`.`、`-`，首尾必须是字母或数字，禁止 `--` 和 `..`；它同时决定安装目录与身份。
- 可选字段仅按当前合同使用：`version`、`description`、`author`、`homepage`、`repository`、`license`、`keywords`、`extensions`。未知字段会被报告并忽略，不要依赖它们。
- `skills/` 中每个直接子目录按 Awaken Skill 合同提供普通文件 `SKILL.md`；不要递归嵌套成另一层插件结构。

`mcp.json` 必须是 closed schema：

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "local-helper": {
      "type": "stdio",
      "command": "./bin/helper",
      "args": ["--data", "${PLUGIN_DATA}/cache"],
      "cwd": "${PLUGIN_ROOT}"
    },
    "remote-api": {
      "type": "streamable-http",
      "url": "https://example.com/mcp"
    }
  }
}
```

- transport 只用 `stdio`、`streamable-http`、`sse`。
- 包内路径必须以 `./` 开始，不能含 `..`、绝对路径或盘符，解析后必须留在 plugin root。
- `${PLUGIN_ROOT}` / `${PLUGIN_DATA}` 仅用于 `args`、`env` 值和 `cwd`；不在 `command`、`url`、headers、env 键中展开。
- `cwd` 只接受 `./...`、`${PLUGIN_ROOT}...`、`${PLUGIN_DATA}...`。不要在 env 中覆盖 `PLUGIN_ROOT` 或 `PLUGIN_DATA`。
- HTTPS remote MCP 不内嵌密钥；需要认证时说明宿主侧配置要求，不把 token 写进候选包。

## 创建流程

1. 明确插件要承载哪些 Skills、哪些 MCP servers，以及为什么需要一个 Plugin 而不是单独 Skill。没有组件也可以形成包，但校验会给出 warning。
2. 选择合规 name，在普通候选目录写最小 `plugin.json`；只创建真实需要的 `skills/`、`mcp.json`、脚本或资源。
3. 对内含 Skill 使用 `skill-creator` 的包编写原则：入口说明有辨识度、资源按需、脚本实际测试。Plugin 级 `validate` 会用当前 Skill parser 复核这些直接子包。
4. 调用 `plugin`：`{"action":"validate","source":{"kind":"local_path","path":"<candidate>"}}`。
5. 修复所有 error diagnostics，并逐条判断 warning 是否符合用户意图。记录 manifest identity、digest、Skill 数和 MCP server 数。
6. 安装前向用户总结来源、组件、stdio command、remote URL、数据目录保留和 MCP 权限边界。用户请求本身包含创建并安装时，以同一候选调用 `action=install` 且 `replace=false`；同名冲突时停止。

## 修改流程

1. 调用 `plugin action=inspect`，确认准确 name、版本、digest、组件与当前 root；不要根据用户口语名称猜包身份。
2. 调用 `plugin action=export`，把完整包导出到一个不存在的工作区候选目录。在线 package/data/state 均保持不变。
3. 用 `Read`/`Glob` 盘点候选，再以 `Edit`/`Write` 做最小修改。默认保留 plugin name、未涉及字段、全部 Skills、脚本和资源；不要读取或打包 `PLUGIN_DATA` 用户数据。
4. 实际运行新增/变化的可执行文件或脚本，并确认不依赖作者机器的绝对路径、秘密或未声明软件。
5. 对完整候选调用 `plugin action=validate`；若校验结果 identity 与原 name 不同，将其视为新插件而不是替换。
6. 只有用户明确要求修改原插件、候选 identity 相同、校验无 error 且安全摘要已复核时，调用 `plugin action=install` 并设置 `replace=true`。

## 生效与验收

- install/replace 完成只证明包、状态与 live roots 已更新；已开始 run 的 Skill/工具快照不变。
- 在新 run 中验证新增或修改的 Plugin Skill 能被选择；MCP server 需要验证连接、工具发现和一次无害调用。
- 验收失败时区分包结构、组件配置、外部依赖/认证和业务行为，不通过改 manifest 隐藏错误。
- 交付时报告：候选路径、安装 root、digest、组件清单、是否替换、旧包废纸篓 receipt、已执行验证和未执行项。

## 停止条件

- 不能完整导出原插件，或在线包在修改过程中变化；
- manifest/schema/name/path containment 不合规；
- 任一内含 Skill 或 MCP server 有 error diagnostic；
- 同名插件存在但用户未授权替换；
- 包含未披露的 stdio 命令、远程端点、密钥、外部下载或生产写入；
- 用户要求直接编辑 `~/.awaken/plugins/packages`、`data` 或 `plugins.json`。

保留候选包并报告阻塞；不要手工覆盖在线目录、永久删除旧包或绕开 PluginStore。
