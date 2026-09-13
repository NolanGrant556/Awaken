# AwakenToolTraceGroup

此目录包含一个独立的 Awaken 设计系统组件、可观测演示状态，以及“工具轨迹”组件族的 Gallery 入口。

## 用途

`AwakenToolTraceGroup` 承载 Desktop 投影出的阶段活动。`actions` 支持工具行，`children` 可加入思考等异构条目；父级负责顺序、分类和执行事实。`entryCount=1` 时直接显示唯一条目，增加活动后使用同一容器升级为组。

组默认收起。`open/onOpenChange` 提供受控披露；未传时使用本地用户选择。`label/detail` 展示累计摘要与异常数，`currentActivity` 只在收起时显示当前活动，`attention` 保持失败或待处理信息可见。展开组后每项承担自身状态。普通完成项沿正文左边缘对齐，不显示固定图标列、底卡或状态胶囊。

披露高度为 180ms、透明度为 120ms，关闭过渡结束后卸载条目。历史一次呈现全部已有条目，不按行交错入场；减少动态效果时直接切换。Desktop 在外层使用独立整轮执行过程披露，状态归属其 disclosure store。

## 文件

- `AwakenToolTraceGroup.tsx` - 组件实现。
- `AwakenToolTraceGroup.css` - 组件局部样式入口。
- `AwakenToolTraceGroup.states.ts` - 可观测状态与模拟 props。
- `AwakenToolTraceGroup.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
