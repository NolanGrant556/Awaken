# AwakenExecutionFeed

此目录包含一个独立的 Awaken 设计系统组件及可观测演示状态；Gallery 观测合并在“运行阶段”组件族入口中。

## 用途

`AwakenExecutionFeed` 用于承载 Agent 执行叙事中的一组可见事件、步骤或消息，并在没有子项时提供空态 class。父级负责生成和排序具体执行内容；组件只负责 section 容器、可访问标签和过滤空子项，不订阅 runtime 事件，也不生成活动记录。

## 文件

- `AwakenExecutionFeed.tsx` - 组件实现。
- `AwakenExecutionFeed.css` - 组件局部样式入口。
- `AwakenExecutionFeed.states.ts` - 可观测状态与模拟 props。
- `AwakenExecutionFeed.demo.tsx` - 保留给 Gallery render smoke 的组件级演示。
- `index.ts` - 组件导出。

## 规则

- 仅包含 UI：不产生 runtime、Electron、AG-UI、A2UI 或文件系统副作用。
- 通过 props 输入，通过回调输出。
- 公共 API 变化时同步更新可观测状态。
- Gallery 在 `#/components/awaken-run-stage` 的真实运行缩略场景中突出显示执行叙事边界；旧 `awaken-execution-feed` URL 跳转到该组件族入口。
