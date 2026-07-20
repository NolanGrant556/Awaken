# AwakenSidebarThreadSpinner

左侧栏项目树中的 UI-only 线程运行指示器。它集中在“「左侧栏」组件库” Gallery 入口中查看，不单独占用组件目录入口。

## 用途

`AwakenSidebarThreadSpinner` 只渲染侧栏线程行末尾的 14px 加载圈。Desktop 根据线程是否存在 `running` 或 `cancelling` run 决定是否显示；组件本身不读取 runtime 状态，也不推断线程生命周期。

## 文件

- `AwakenSidebarThreadSpinner.tsx` - 组件实现与可访问名称。
- `AwakenSidebarThreadSpinner.css` - 与原项目树加载圈一致的尺寸、边框和动效。
- `AwakenSidebarThreadSpinner.states.ts` - running / cancelling 可观测状态。
- `AwakenSidebarThreadSpinner.demo.tsx` - 独立视觉 Demo，由“「左侧栏」组件库”集中承载入口。
- `index.ts` - 组件导出。

## 规则

- 只接收展示文案，不读取线程或 run。
- 默认可访问名称为“加载中”。
- `prefers-reduced-motion` 下延长旋转周期，不改变 14px 行内几何。
