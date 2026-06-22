# AwakenAvatar

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及对应的 Gallery 入口。

## 用途

`AwakenAvatar` 用于显示用户、代理或工作区身份的圆形头像；有 `src` 时展示图片，没有图片时从 `fallback` 提取两个字符作为缩写。它适合会话行、用户入口和协作者标识；父级负责提供身份数据和外围可访问标签，本组件只负责头像图形和尺寸，不决定身份名称的展示位置。

## 文件

- `AwakenAvatar.tsx` - 组件实现。
- `AwakenAvatar.css` - 组件局部样式入口。
- `AwakenAvatar.states.ts` - 可观测状态与模拟 props。
- `AwakenAvatar.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
