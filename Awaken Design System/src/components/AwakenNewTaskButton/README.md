# AwakenNewTaskButton

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它集中在“「左侧栏」组件库” Gallery 入口中查看。

## 用途

`AwakenNewTaskButton` 用于在侧边栏、任务列表或欢迎态中提供“新建任务”的主入口，并可展示平台相关快捷键。父级负责传入点击回调、文案、平台和禁用状态；组件只负责按钮外观、悬停/按下反馈、图标和快捷键展示，不创建任务实体。

## 文件

- `AwakenNewTaskButton.tsx` - 组件实现。
- `AwakenNewTaskButton.css` - 组件局部样式入口。
- `AwakenNewTaskButton.states.ts` - 可观测状态与模拟 props。
- `AwakenNewTaskButton.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
