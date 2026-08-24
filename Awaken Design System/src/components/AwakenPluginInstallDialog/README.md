# AwakenPluginInstallDialog

插件安装弹窗。提供本地目录/zip 与 HTTPS URL 两种输入，以及同名替换与“MCP 子进程无沙盒”确认。来源使用统一分段选择，风险说明与确认位于同一警示区；确认勾选前不能提交。

## Props

- `mode` / `localPath` / `url` / `refValue` / `subpath` / `replace` / `confirm`：受控表单。
- `onPickLocal`：可选本地浏览按钮。
- `onSubmit`：用户确认安装后触发；实际安装由 Desktop 拥有。

风险说明是固定文案，不因插件声明而改变。组件不读取文件系统或 runtime。
