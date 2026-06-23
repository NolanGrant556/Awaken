# AwakenTaskWelcome

`AwakenTaskWelcome` 是新建任务空闲态的品牌欢迎区，用于在中心 Composer 上方展示 Awaken 字标和一句说明文案。

## 用途

该组件表达“当前可以开始交给 Awaken 执行任务”的入口状态。它只负责静态视觉展示，不拥有输入、提交、运行状态、runtime 或 Electron 行为。

## 文件

- `AwakenTaskWelcome.tsx` - 组件实现。
- `AwakenTaskWelcome.css` - 组件局部样式。
- `AwakenTaskWelcome.states.ts` - 可观测状态与模拟 props。
- `AwakenTaskWelcome.demo.tsx` - 独立 Gallery 演示。
- `index.ts` - 组件导出。

## 规则

- 默认使用设计系统 `logo-wordmark.png` 透明字标，呈现无底色的黑色几何 wordmark。
- 默认文案是 `把任务交给 Awaken，自主执行，唤醒自由。`。
- 保持 UI-only，不访问 runtime、Electron、AG-UI、A2UI、SSE 或文件系统。
- 父级负责决定组件何时出现；运行开始后不应继续保留在执行叙事里。
