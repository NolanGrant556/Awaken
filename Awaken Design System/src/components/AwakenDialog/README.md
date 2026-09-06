# AwakenDialog

此目录包含 Awaken 的统一模态基础组件及其可观测状态。它不再单独占用 Gallery 目录入口；真实弹窗随“左侧栏”“设置”“技能”和“连接器”组件族一起观测。

## 用途

`AwakenDialog` 统一提供遮罩、Portal、标题与底部操作区、初始焦点、焦点恢复、Tab 焦点循环和 Escape 关闭。Escape 在当前弹窗消费后停止继续冒泡，避免同时关闭承载弹窗的设置页等外层界面。父级负责弹窗内容、业务状态与确认动作；组件不读取 Desktop runtime，也不执行项目、Skill、设置或连接器操作。

## 文件

- `AwakenDialog.tsx` - 组件实现。
- `AwakenDialog.css` - 组件局部样式入口；全局模态层必须高于 full-screen Settings 和页面级浮层。
- `AwakenDialog.states.ts` - 可观测状态与模拟 props。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- Portal 遮罩使用全局模态层级，不能被 full-screen Settings 外壳或其页面级浮层覆盖。
- 通过 props 输入，通过回调输出。
- 遮罩可用 `backdropClassName` 叠加状态。`aw-dialog-backdrop--hidden` 在卸载前先停止覆盖窗口，避免全屏 Portal 卸掉时闪白。
- 公共 API 变化时同步更新可观测状态。
- 原 `awaken-dialog` URL 跳转到“「左侧栏」组件库”；组件源码和公开导出继续供 Desktop 与其他设计系统组件使用。
