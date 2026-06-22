# AwakenIcon

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenIcon` 用于承载 Awaken 自定义 SVG 图形，统一尺寸、描边、颜色继承和 `aria-hidden` 默认规则。父级负责传入具体的 SVG 子节点并决定图标语义；组件只提供 SVG 容器和绘制规范，不内置图标名称，也不替代 lucide 等图标库。

## 文件

- `AwakenIcon.tsx` - 组件实现。
- `AwakenIcon.css` - 组件局部样式入口。
- `AwakenIcon.states.ts` - 可观测状态与模拟 props。
- `AwakenIcon.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
