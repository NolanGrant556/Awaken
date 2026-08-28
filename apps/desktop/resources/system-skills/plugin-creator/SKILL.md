---
name: plugin-creator
version: 1.2.0
description: 创建、校验或修改 Awaken Agent Plugins 1.0.0 完整目录包；显式规划每个内含 Skill 的 SKILL.md、references、scripts、assets 与其他资源，并通过 Core PluginStore 安全安装、替换或提交已授权的在线修改。
when_to_use: 用户要求创建、编写、验证、修复、更新或修改 Awaken Plugin，或询问候选 Plugin 及其安装路径、Skills、MCP 配置是否合规时。
user-invocable: true
context: inline
---

# Awaken Plugin Creator

Awaken Plugin 是一个包管理与来源容器，不是第二套 Skill 或 MCP runtime。当前只创建 Agent Plugins 1.0.0：根 `plugin.json`，可选直接子级 `skills/` 和根 `mcp.json`。内含 Skill 是完整目录包，不能把它误解成一份孤立的 `SKILL.md`。仅创建 Awaken 当前支持的 Agent Plugins 1.0.0 结构；不要生成其他宿主的私有清单，也不要创建或宣称支持本 Skill 未明确列出的组件。

## 安全边界

- 新建和默认修改都在任务工作区的普通候选目录完成，例如 `plugin-authoring/<plugin-name>/`。默认修改先用 `plugin action=inspect` 确认身份，再用 `export` 导出完整候选包；不要只复制 manifest。
- 只有用户在当前请求明确授权直接修改某个已安装 Plugin 时，才可以编辑 inspect 返回的准确 `root_dir`。普通的「修改/修复插件」请求不自动授权在线直改，仍走候选 export/replace。
- 在线直改必须在第一次写入前保留 inspect 返回的 recorded digest，写入只限该 `PLUGIN_ROOT`；TaskScope 已把 `<AWAKEN_HOME>/plugins` 作为文件工具可写例外，因此优先用 `Read` + `Edit` / `Write` 保留宿主快照与 diff，必要时才用当前用户 `Bash`。授权不包含 `PLUGIN_DATA`、`~/.awaken/config/plugins.json` 或其他插件。所有修改完成后必须调用 `plugin action=commit` 且传入该旧 digest 作为 `expected_digest`；直接写入本身不代表已生效。
- `plugin action=validate` 是同一 PluginStore/runtime parser 的无写入校验；安装前必须成功且没有 error diagnostics。
- 同名安装默认 abort。只有用户明确要求修改该插件、完整候选已复核时才可 `replace: true`；旧包由 Core 移入系统废纸篓，`PLUGIN_DATA` 跨替换保留。
- Plugin 中的 stdio MCP 子进程以当前用户身份运行，当前没有额外 Seatbelt/WorkspaceExecutionBroker 沙盒。安装、替换或签收修改过的 server 前，必须向用户列出 command、网络端点和这一权限事实。

## 完整包结构

```text
<plugin-name>/
├── plugin.json                    # 必需：Agent Plugins 1.0.0 manifest
├── skills/
│   └── <skill-name>/              # 可选；只发现 skills/ 的直接子目录
│       ├── SKILL.md               # 必需：该 Skill 的入口
│       ├── references/            # 可选：API、schema、业务规则、错误恢复
│       ├── scripts/               # 可选：确定性辅助逻辑
│       ├── assets/                # 可选：模板与静态资源
│       └── <other-resources>/     # 可选：有明确用途的其他包内资源
├── mcp.json                       # 可选：标准 MCP server 配置
├── bin/                           # 可选：mcp.json 引用的包内可执行文件
└── <reverse-domain-extension>/   # 可选：标准允许；Awaken 未实现时忽略
```

`scripts/`、`references/`、`assets/` 是常见约定，不是白名单；Agent Skills 允许包内携带其他必要文件。不要创建空目录或占位资源。完整规划规则见 [references/package-authoring.md](references/package-authoring.md)。本 Skill 已经自包含这些规则；不要假设文字提到 `skill-creator` 就会自动加载另一份 Skill 正文。

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
- `skills/` 中每个直接子目录必须是严格 Agent Skills 包：`name`/`description` 显式存在，`name` 与目录完全一致；可选 `license` 为字符串、`compatibility` 为非空的 1–500 字符字符串、`metadata` 为 string->string map、`allowed-tools` 为空格分隔字符串，不能以显式 `null` 冒充省略。按需携带 supporting resources；不要递归嵌套成另一层插件结构。

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
- `./` stdio command 必须指向包内已存在的普通文件；在 macOS/Linux 候选中必须带执行位。Core 安装会把普通文件规范化为 owner-only `0600`，把带执行意图的文件规范化为 owner-only `0700`，ZIP/Git/导出也遵守同一规则。

## 资源规划门

创建每个内含 Skill 前，分别确定：

- 入口中必须保留的共同流程、工具选择、安全边界；
- 应拆入 `references/` 的 API 字段、主数据要求、状态、错误码、长示例；
- 应拆入 `scripts/` 的重复、可测试、确定性辅助逻辑；
- 应拆入 `assets/` 的 payload 模板、文档模板或静态数据；
- `SKILL.md` 中何时列出或读取每个资源。

多阶段业务流程、API/MCP 工具编排、复杂 schema、错误恢复或可复用模板若最终只有 `SKILL.md`，必须在交付摘要中说明为什么 supporting resources 没有实际价值。MCP server 继续拥有认证、实时数据和受控动作；Skill 不复制第二套 API client。

## 确定性脚手架

新插件优先用包内脚手架建立标准候选。`--skill` 可重复，格式为 `<name>=<description>`；`--skill-resources` 可按 Skill 指定实际需要的标准资源目录：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/init_plugin.py" <plugin-name> \
  --path <candidate-parent> \
  --description "<插件用途>" \
  --skill 'order-flow=编排订单、采购、入仓和领料；当用户要求执行完整订单流程时使用。' \
  --skill-resources 'order-flow=references' \
  --with-mcp
```

生成后填充或删除空目录，补齐真实 MCP servers、脚本和资源。Core 校验前先运行无写入预检：

```bash
python3 "${AWAKEN_SKILL_DIR}/scripts/validate_plugin.py" <candidate-plugin-root>
```

预检会严格检查 manifest、直接子 Skill、名称/目录一致性、资源引用、symlink、包预算和 `mcp.json` 顶层结构。最终权威仍是 `plugin action=validate`；即使 Core 为标准故障隔离而允许安装含无效组件的包，Creator 也不得把本次意图中的 Skill/server 被跳过当成成功。

## 创建流程

1. 明确插件要承载哪些完整 Skills、哪些 MCP servers，以及为什么需要一个 Plugin 而不是单独 Skill。没有组件也可以形成包，但校验会给出 warning。
2. 对每个 Skill 完成资源规划；复杂流程只有入口文件时记录理由。
3. 使用 `init_plugin.py` 或普通文件工具创建候选，再填充真实 `references/`、`scripts/`、`assets/`、`bin/` 和 `mcp.json`。不要保留脚手架占位正文或空目录。
4. 运行零外部依赖的 `validate_plugin.py`；修复包结构、基础入口、manifest/MCP 与资源引用 error，并处置空资源目录、未引用资源等 warning。
5. 实际从候选目录执行每个新增或变化的 `./` stdio command，确认其可执行、能完成 MCP initialize/tools list，且没有依赖作者机器绝对路径或未声明秘密。
6. 调用 `plugin`：`{"action":"validate","source":{"kind":"local_path","path":"<candidate>"}}`。Core 以真实 YAML parser 按 Agent Skills 规范完整校验每个内含 Skill 的 `name`、`description`、`license`、`compatibility`、`metadata` 和 `allowed-tools`；脚本预检不能替代这道权威门禁。
7. 确认所有意图中的 Skill/server 都进入 loadable 计数；标准故障隔离产生的 component diagnostic 也必须修复，不能只看顶层 `valid`。记录 manifest identity、digest、完整文件树、Skill 数和 MCP server 数。
8. 安装前向用户总结来源、组件、stdio command、remote URL、数据目录保留和 MCP 权限边界。用户请求本身包含创建并安装时，以同一候选调用 `action=install` 且 `replace=false`；同名冲突时停止。

## 默认修改流程（候选替换）

1. 调用 `plugin action=inspect`，确认准确 name、版本、digest、组件与当前 root；不要根据用户口语名称猜包身份。
2. 调用 `plugin action=export`，把完整包导出到一个不存在的工作区候选目录。在线 package/data/state 均保持不变。
3. 用 `Read`/`Glob` 盘点完整候选树，再以 `Edit`/`Write` 做最小修改。默认保留 plugin name、未涉及字段、全部 Skills、脚本和资源；不得把多文件 Skill 压回单文件，也不要读取或打包 `PLUGIN_DATA` 用户数据。
4. 重新执行资源规划与 `validate_plugin.py`，确认资源引用完整、没有空目录或脚手架正文。
5. 实际运行新增/变化的可执行文件或脚本，并确认不依赖作者机器的绝对路径、秘密或未声明软件。
6. 对完整候选调用 `plugin action=validate`；若校验结果 identity 与原 name 不同，将其视为新插件而不是替换。
7. 只有用户明确要求修改原插件、候选 identity 相同、校验与全部意图组件均通过且安全摘要已复核时，调用 `plugin action=install` 并设置 `replace=true`。

## 授权在线修改流程（直改 + commit）

1. 确认用户在当前请求明确授权直接修改准确的已安装 Plugin；未明确授权就使用默认候选替换流程。
2. 调用 `plugin action=inspect`，确认准确 `name`、recorded `digest`、`root_dir`、版本、组件与诊断。基线已有 `plugin_digest_changed`、identity 异常或其他包级损坏时不得把它冒充本次修改的旧状态；停止并报告。
3. 在第一次写入前固定 inspect 返回的 recorded digest 为 `expected_digest`，盘点 `root_dir` 内完整文件树。只对用户请求的 Plugin 做最小修改，保留 name、未涉及字段、Skills、scripts、assets 和其他资源；优先先 `Read` 目标文件再以 `Edit` / `Write` 修改，文件工具对插件仓库的放行不构成修改其他 Plugin、`PLUGIN_DATA` 或 `plugins.json` 的授权。
4. 完成全部文件写入后，直接对 `root_dir` 运行 `validate_plugin.py`，并实际运行新增/变化的脚本或 stdio command。在 commit 完成前，该包会因 digest 未签收而 fail closed；不要在这个窗口宣称插件可用或用旧 Plugin 快照验收。
5. 确认 manifest identity 仍与 inspect 的 `name` 相同、所有意图 Skill/server 都完整且没有未处置诊断后，调用：

```json
{
  "action": "commit",
  "name": "<installed-plugin-name>",
  "expected_digest": "<recorded-digest-from-pre-edit-inspect>"
}
```

6. commit 会在 Store 锁内 CAS 对比旧 recorded digest，再对当前安装树完整校验。`plugin_recorded_digest_changed` 表示该插件状态已被其他 mutation 推进；不要换用漂移树的 digest 强行重试，应重新 inspect 并停止本轮以免覆盖并发变更。校验失败会保留旧 recorded digest 并继续 fail closed；只能修正当前树后以原 `expected_digest` 重试，不得直接修改状态文件。
7. 只在 commit 返回 `valid=true`、`installed=true`、新 `digest`、正确组件计数且没有未处置诊断后声明签收成功。成功会刷新 Skill/MCP/catalog，但已开始 run 的固定快照不变；停止当前 run，在新 run 验收。

## 生效与验收

- install/replace/commit 完成只证明包、状态与 live roots 已更新；已开始 run 的 Skill/工具快照不变。
- 安装后不得再修改候选并把旧安装包写成“最新”；候选若有任何变化，必须重新 validate 并显式 replace。
- 在线写入后、commit 成功前出现 `plugin_digest_changed` 是未签收漂移的预期 fail-closed 状态，不是成功。commit 成功后若仍有该诊断，或任一路径存在 `plugin_data_invalid`、stdio command 异常或意图组件缺失，立即停止并分类报告；不能手工改 `plugins.json` 或以 `chmod` 绕过 PluginStore。
- 当前 run 不会获得刚 install/replace/commit 的 Skill/MCP 快照。完成 mutation 摘要后停止本 run，在新 run 中通过结构化 `@ Plugin` 选择验证 Skill；先用 `skill_resource_list` 复核包内文件树，再按入口路由用 `skill_resource_read` 读取资源。MCP server 必须完成连接、工具发现和至少一次无害的真实 MCP tool call。
- 新 run 没有目标 MCP 工具时，报告安装/runtime readiness 问题；不得退回 Bash、直接运行包内二进制或调用 ERP HTTP 来冒充 Plugin 验收。
- 验收失败时区分包结构、组件配置、外部依赖/认证和业务行为，不通过改 manifest 隐藏错误。
- 交付时报告：使用候选替换还是授权在线 commit、候选路径（如有）、安装 root、旧/新 digest、组件清单、是否替换、旧包废纸篓 receipt（如有）、已执行验证和未执行项。

## 停止条件

- 不能完整导出原插件，或在线包在修改过程中变化；
- manifest/schema/name/path containment 不合规；
- 任一意图中的 Skill 或 MCP server 未被加载、被严格标准校验跳过，或仍有未处置 diagnostic；
- 同名插件存在但用户未授权替换；
- 包含未披露的 stdio 命令、远程端点、密钥、外部下载或生产写入；
- 用户未在当前请求明确授权在线直改，却要求跳过候选替换；
- 在线直改前无法 inspect 一个无 digest 漂移的旧基线，或 commit 返回 `plugin_recorded_digest_changed`；
- 用户要求编辑 `PLUGIN_DATA`、`plugins.json` 或未授权的其他插件。

候选路径保留候选包并报告阻塞；授权在线路径保留当前树和旧 recorded digest，使其继续 fail closed 并报告阻塞。不要手工覆盖状态、永久删除旧包或绕开 PluginStore。
