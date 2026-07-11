# Office 后台渲染

## 权威范围

本文唯一定义 DOCX/XLSX/PPTX 等 Office 文件通过受管 LibreOffice 后台转换的 worker 架构、隔离、并发、原子发布和平台打包约束。04 拥有 `office_export_pdf` 等工具输入/输出；01 拥有最终交付物显示和安全打开。

## 当前架构

[`src/runtime/officerender`](../../src/runtime/officerender) 提供进程级共享 worker：

```text
typed Office tool
  -> RenderManaged(request)
  -> resolve active primary-runtime revision
  -> attest LibreOffice + dependencies
  -> shared worker queue / identical-job singleflight
  -> private staging + isolated profile
  -> process group
  -> validate output
  -> atomic replace destination
```

默认并发为 1，避免 LibreOffice profile/字体/cache 争用。同一输入、目标和 revision 的同时请求可 singleflight，共享一次实际转换结果；不同 job 保持独立 staging。

调用者不能向 `RenderManaged` 传任意 LibreOffice 路径。worker 必须从 primary runtime 的 active pointer 解析 binary；active revision 或 fingerprint 改变时，旧 attestation 失效并重新检查。

## 作业隔离与发布

每个 job 拥有：

- 私有 staging/input/output；
- 独立 LibreOffice user profile；
- 最小化环境变量；
- 独立 process group；
- 默认 90 秒 timeout。

取消或超时终止整个 process group，不能只杀父进程留下 soffice 子进程。转换输出完成并校验后才原子替换目标；失败时保留此前有效文件，不能留下半写 PDF 覆盖成功产物。

所有 Office 转换必须经过 worker。04 的 Shell 硬门禁拒绝直接 `soffice`/`libreoffice` 以及已知 wrapper，防止绕开 resolver、隔离、timeout 和原子发布。

## 平台打包边界

- macOS：随 primary runtime 提供完整 LibreOffice.app；所有依赖和 helper 进入签名/公证范围，后台启动不得弹出窗口或 Dock 图标。
- Windows：随 archive 提供完整 program/DLL 集；process group 与替换使用 Windows 实现，不假设 Unix signal/rename 行为。
- 字体和布局属于目标平台验收的一部分；“转换命令退出 0”不足以证明页面内容完整。

## 异常分类与恢复

| 异常 | 当前处理 |
| --- | --- |
| runtime/LibreOffice 缺失或 attestation 失败 | `needs_runtime` 或明确失败，不用系统 binary 兜底 |
| queue 等待时取消 | job 不启动子进程，返回 cancelled |
| 转换超时 | 终止 process group，保留旧目标 |
| LibreOffice 非零退出/无输出 | failed，保留 stderr 摘要和诊断，拒绝发布 |
| 输出校验失败 | failed，staging 清理，旧目标保留 |
| active revision 变化 | 重新 attestation；进行中的 job 继续绑定原 immutable revision |

## 当前不支持与限制

- 不支持并行复用同一个 LibreOffice profile。
- 不支持调用者指定系统 LibreOffice 绕过主运行时。
- 集成测试需要显式提供受管 runtime，默认单测不会证明所有目标平台字体/渲染一致。
- 页面视觉正确性仍需真实 DOCX/XLSX/PPTX 样本检查；PDF 存在不等于内容完整。

## 测试与真实验收

自动化 owner：

- [`src/runtime/officerender/worker_test.go`](../../src/runtime/officerender/worker_test.go)
- [`src/runtime/officerender/shared_test.go`](../../src/runtime/officerender/shared_test.go)
- [`src/runtime/officerender/managed_runtime_test.go`](../../src/runtime/officerender/managed_runtime_test.go)
- [`src/runtime/officerender/process_unix_test.go`](../../src/runtime/officerender/process_unix_test.go)
- [`src/runtime/officerender/integration_test.go`](../../src/runtime/officerender/integration_test.go)

测试覆盖 50 次顺序转换、相同 job singleflight、timeout/cancel process-group、revision 变化和旧目标保全。真实 Desktop 验收使用含中文字体、表格、图片、多页/多 sheet 的 DOCX/XLSX/PPTX，走 typed 工具导出 PDF，逐页核对内容完整、无前台 Office 窗口，并确认重启后 artifact 仍可重放和安全打开。
