# AwakenToolActionRow

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenToolActionRow` 用于展示探索阶段中的一条工具动作摘要，包括标题、细节和 active/done/error 状态。它适合被 `AwakenExploringGroup` 这类父级列表包裹，用来表示“正在读取文件”“已完成检查”“命令失败”等动作；父级负责提供动作数据和选择行为，本组件只负责单行动作的按钮外观与状态图标。

## 文件

- `AwakenToolActionRow.tsx` - 组件实现。
- `AwakenToolActionRow.css` - 组件局部样式入口。
- `AwakenToolActionRow.states.ts` - 可观测状态与模拟 props。
- `AwakenToolActionRow.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
