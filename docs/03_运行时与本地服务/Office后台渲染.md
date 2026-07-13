# Office 后台渲染

## 1. 当前支持

Office worker 复用当前用户已安装的 LibreOffice。每次转换前从 `AWAKEN_SOFFICE`、当前 PATH 和平台标准应用位置解析 `soffice`，记录绝对路径、大小与修改时间指纹，并在执行前再次解析，避免调用过程中可执行身份漂移。Awaken 不复制、修改、重签或版本化 LibreOffice。

每次转换使用独立 LibreOffice user profile、明确输入/输出目录、timeout 和输出校验。取消/超时终止 process group；成功后原子发布目标，失败保留此前有效文件，不以半写 PDF/预览覆盖成功产物。

## 2. 与 Agent/Shell 的关系

LibreOffice 缺失时 typed 工具返回明确依赖错误，不形成权限 gate或后台安装状态。Agent可用当前用户 Shell 按正常系统方式安装，再重试；worker在下一次调用重新解析环境。直接 Shell 调用 `soffice` 仍是合法本机路径，但不自动获得 worker 的独立 profile、原子发布、Artifact schema 和渲染验证。

## 3. 平台边界

- macOS：发现 PATH、`/Applications/LibreOffice.app` 与用户 Applications；真实 Desktop 仍需验证 headless 行为及安装版签名上下文。
- Windows：发现 PATH、Program Files、LocalAppData；必须实机验证 `soffice.exe`、进程树和原子替换。
- Linux：发现 PATH 与常见系统位置，仍需对应发布环境验证。

## 4. 不变量

- 找到文件不等于可用；执行前必须 probe并复核具体可执行身份。
- render 失败不得生成成功 Artifact 或伪装完成。
- 不在启动、构建、打包阶段安装或复制 LibreOffice。

## 5. 代码 owner

- `src/runtime/officerender/host_runtime.go`：当前用户 LibreOffice 发现与指纹。
- `src/runtime/officerender/worker.go`：隔离 profile、进程、timeout、验证和原子发布。
- `src/tools/artifact_runtime`：typed tool 依赖发现、Outcome 与 Artifact 接入。
