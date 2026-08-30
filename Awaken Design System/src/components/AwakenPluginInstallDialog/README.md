# AwakenPluginInstallDialog

插件安装弹窗说明兼容 Codex、Cursor、Claude Code、Agent Plugins 格式，并只提供一个 ZIP 拖放区。用户可以点击该区域打开系统 ZIP 选择器，也可以直接拖入文件；Desktop 在来源确定后自动校验，组件只根据受控状态呈现校验中、格式错误和浅绿色有效状态。

## Props

- `localPath`：当前 ZIP 来源，仅用于展示文件名。
- `validation`：Desktop 自动校验得到的受控结果；`valid=true` 且 digest 非空时显示浅绿色状态并允许安装。
- `validating` / `busy`：自动校验与安装状态，期间锁定拖放区。
- `errorMessage`：格式或来源错误；拖放区本身通过 live status 呈现通用格式错误，存在更具体的安全错误时才在下方追加 `role="alert"` 说明。
- `onPickLocal`：点击拖放区时触发，由 Desktop 打开系统 ZIP 选择器。
- `onDropFile`：拖入文件时触发；文件路径解析和 ZIP 限制由 Desktop Main/Preload bridge 负责。
- `onSubmit`：用户点击“安装”时触发；实际安装由 Desktop 拥有。

组件不展示人工检查按钮、包详情、同名替换或权限确认。Desktop 始终使用 digest 锁定的自动校验结果安装；同名替换和当前用户权限属于 Awaken 插件运行的默认产品策略。
