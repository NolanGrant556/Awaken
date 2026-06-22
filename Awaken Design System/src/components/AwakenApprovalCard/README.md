# AwakenApprovalCard

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenApprovalCard` 用于在文件修改、命令执行或权限请求前展示审批信息，包括风险等级、变更摘要、受影响路径和允许/拒绝按钮。它适合以内联卡片或遮罩对话框形式出现在运行流中；父级负责决定是否打开、风险语义、审批结果和回调副作用，本组件只负责可访问的审批 UI、禁用态和关闭/拒绝交互。

## 文件

- `AwakenApprovalCard.tsx` - 组件实现。
- `AwakenApprovalCard.css` - 组件局部样式入口。
- `AwakenApprovalCard.states.ts` - 可观测状态与模拟 props。
- `AwakenApprovalCard.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
