# Slice 8: Sidebar Runs 与 Thread 状态

## 目标

把左侧导航从通用 Conversation/Dashboard 语义收窄为 Run/Thread 列表，让用户能看到每个 thread 当前对应的 run 状态，并在切换 thread 时恢复中心 Run Narrative。

## 范围

- `packages/ui` 只扩展可复用导航 UI 的展示字段，不引入 runtime、Electron、AG-UI、A2UI、SSE 或 `window.awaken`。
- Desktop reducer 保存每个 thread 的状态快照，包括 run status、是否有 artifact、是否还有待处理 approval。
- Desktop App 把 reducer 的 thread 状态映射成左侧用户可读状态：`running`、`needs input`、`ready for review`、`done`、`error` 等。
- 切换 thread 时保留并恢复 prompt、answer、approval、artifact、narrative 与 inspector 相关状态。

## 验收标准

- 新 run 创建后出现在左侧列表，并显示 running。
- awaiting approval / approval request 会在左侧显示 needs input。
- 有 artifact 的 run 可显示 ready for review，终态无待审事项时显示 done 或 error/cancelled。
- 切换 thread 后中心 narrative 不丢失 answer、approval、artifact。
- 左侧只承担 Runs/Threads 导航职责，不恢复 Dashboard 或卡片墙入口。

## 暂不做

- 不做 scheduled runs。
- 不做云端历史。
- 不做跨 workspace thread 合并。
