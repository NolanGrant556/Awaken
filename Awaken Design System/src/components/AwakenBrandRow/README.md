# AwakenBrandRow

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenBrandRow` 用于展示 Awaken 品牌标识、名称和一行辅助元信息。它适合放在侧栏顶部、启动区域或组件库 Gallery 的品牌行；父级负责选择放置位置、传入自定义 logo/title/meta，本组件只负责品牌行排版，不承担导航、账号切换或窗口控制。

## 文件

- `AwakenBrandRow.tsx` - 组件实现。
- `AwakenBrandRow.css` - 组件局部样式入口。
- `AwakenBrandRow.states.ts` - 可观测状态与模拟 props。
- `AwakenBrandRow.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
