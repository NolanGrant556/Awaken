# AwakenSidebarSectionAction

此目录包含一个独立的 Awaken 设计系统组件和可观测演示状态；它集中在“「左侧栏」组件库” Gallery 入口中查看。

## 用途

`AwakenSidebarSectionAction` 用于侧栏分区标题和项目行中的紧凑图标动作，例如筛选项目、添加文件夹或新建会话。父级负责传入动作语义、图标和回调；组件负责命中区域、Tooltip、禁用态与可访问名称。

## 文件

- `AwakenSidebarSectionAction.tsx` - 组件实现。
- `AwakenSidebarSectionAction.css` - 组件局部样式入口。
- `AwakenSidebarSectionAction.states.ts` - 可观测状态与模拟 props。
- `AwakenSidebarSectionAction.demo.tsx` - 独立视觉 Demo，由“「左侧栏」组件库”集中承载入口。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- `label` 同时作为按钮可访问名称与 Tooltip 文案。
