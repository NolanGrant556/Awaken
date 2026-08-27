# AwakenPluginInstallDialog

插件安装弹窗。提供本地目录/zip 与 HTTPS URL 两种输入，以及同名替换与“MCP 子进程无沙盒”确认。安装采用受控的两阶段交互：来源非空后先“检查插件”，再展示包 identity、可加载组件数量、digest 与有界诊断；只有检查结果 `valid=true`、digest 非空且用户完成完全访问风险确认后，才可“确认安装”。

## Props

- `mode` / `localPath` / `url` / `refValue` / `subpath` / `replace` / `confirm`：受控表单。
- `validation`：可选受控检查结果；类型为 `AwakenPluginValidationSummary`，包含 `valid`、`name`、`version`、`digest`、Skill/MCP server 数量和 `AwakenPluginValidationDiagnostic[]`。来源、Git ref、子目录或替换策略变化后，consumer 应清空旧结果。
- `validating` / `busy`：分别表示检查与安装进行中；任一状态都会锁定来源、可选项与风险确认，避免把旧检查结论用于变化中的输入。
- `onPickLocal`：可选本地浏览按钮。
- `onValidate`：用户点击“检查插件”后触发；实际 dry-run 与错误处理由 Desktop 拥有。
- `onSubmit`：用户确认安装后触发；实际安装由 Desktop 拥有。

检查通过仅表示插件包可安装，不代表每个意图组件都有效；结果卡会提示用户核对可加载数量。诊断最多展示前 4 条，每条正文最多显示 180 字符，并明确剩余数量；完整内容保留在原生提示中。

风险说明是固定文案，不因插件声明或检查结果而改变：插件 MCP 子进程仍以当前用户身份运行，可以读写本机文件并联网，不受额外沙盒约束。组件不读取文件系统或 runtime，也不实现受限权限模型。
