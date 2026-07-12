# Office 后台渲染

## 1. 当前支持

Office worker 是 Awaken typed artifact provider 的受管转换路径。它从 primary runtime active pointer 解析并校验 LibreOffice，而不是信任调用者传入任意 binary；revision/fingerprint 改变后重新 attestation。

每次转换使用独立 LibreOffice user profile、明确输入/输出目录、timeout 和输出校验。取消/超时终止 process group；成功校验后原子发布目标，失败保留此前有效文件，不以半写 PDF/预览覆盖成功产物。

## 2. 与工具目录的关系

worker 通过 provider manifest 声明 presentation/document render capability、managed-runtime dependencies 和 readiness。run 工具快照固定本次调用使用的 provider/runtime；active revision 在运行中变化只影响后续 run。

Office worker 是已定义的 typed provider，不是所有 Office/Artifact 任务的唯一实现。默认 Shell 可以当前登录用户的环境和操作系统权限直接调用用户已安装的 `soffice`、网络或 workspace 外资源，但该路径不自动获得 Office worker 的 attestation、独立 profile、原子发布、渲染验证或 artifact schema。因此直接 `soffice` 是可用的本机候选，但不是 typed worker 的质量/恢复合同等价替代；需要 verified artifact 时仍应调用独立 inspect/render 并完成真实视觉验收。

## 3. 平台边界

- macOS：managed LibreOffice.app 及 helper 进入签名/公证和后台无窗口约束；仍需本轮真实 Desktop 渲染复验。
- Windows：代码/manifest 必须解析 `soffice.exe`、处理进程树和原子替换；本轮无 Windows 实机渲染证据。
- Linux：没有可从现有 macOS pipeline 推导的发布承诺。

## 4. 不变量

- PATH 中存在 LibreOffice 不等于 managed provider attested/ready。
- render 失败不得生成成功 artifact 或让最终回合伪装完成。
- worker readiness、tool outcome 与 UI artifact 展示必须使用同一 run snapshot 和 journal 事实。

## 5. 代码 owner

- `src/runtime/officerender`：managed revision 解析、进程隔离、timeout、验证和原子发布。
- `src/tools/artifact_runtime`：tool/provider 接入与 runtime resolver。
