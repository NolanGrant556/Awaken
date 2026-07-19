# 前端设计系统与 Workbench 优化进展复核

> 复核日期：2026-07-19
>
> 复核范围：`f72ab9fd`、`910376a3`、`9b3bc07e`、`0135d7d6` 四个已落地提交
>
> 文件定位：这是「前端设计系统与 Workbench 优化」的阶段交接文档，只说明已完成的第一步和下一步执行方向，不替代 `docs/` 中的产品、架构与验收权威。

## 一、阶段结论

第一步已经完成。这一步不是重做 Workbench，而是先建立可继续迭代的视觉基础和交互稳定性基线：

- Awaken 已拥有清晰的 skin / skeleton token 边界，视觉改动与会影响测量、滚动和定位的结构改动不再混在一起。
- 浅色与深色的中性表面、文字、边框、阴影和状态色已完成第一轮语义收敛，关键小字、状态和控件的可读性也已纳入设计系统约束。
- Composer、项目树、新建任务、用户 prompt、滚动条和图表等现有组件已开始统一消费语义 token，不再依靠各自的临时色值和阴影。
- Workbench 的 streaming follow、被动布局变化和 native tail 处理已在 scroll controller / 纯决策 owner 内稳定，为后续视觉与几何优化提供了可信的交互底座。
- Mermaid 的 inline、展开、缩放、拖动、复位、重开与多实例渲染已有组件测试和独立的真实 Electron 原生指针回归。

因此，当前任务不再是「继续收掉第一步的修复尾项」，而是正式进入第二步：**从基础一致性转向 Workbench 核心场景的整体体验优化。**

## 二、第一步完成了什么

### 2.1 设计系统的迭代边界已经建立

[Awaken Design System Guide](<Awaken Design System/DESIGN_SYSTEM_GUIDE.md>) 现在明确区分：

- skin token 负责颜色和阴影色值，不参与布局测量，但仍必须通过主题、真实视觉和无障碍验收；
- skeleton token 负责间距、字体度量、圆角、控件尺寸和布局宽度；修改它们等同于交互变更，必须进入 Workbench 中间区域复核与真实 Desktop 验收；
- `Awaken Design System` 继续保持 UI-only，产品状态、runtime、Electron 和滚动决策仍由 Desktop 拥有。

这些边界让后续优化可以清楚地回答「改的是视觉语言、组件结构，还是 Workbench 交互几何」。

### 2.2 第一轮视觉语言已经统一

当前已形成一套可继续迭代的 Awaken 视觉基底：

- 以暖调中性表面建立页面层级，减少无差别纯白、冷灰和临时透明色的混用；
- success / warning / error / running 各自回归单一基色家族，文字、背景、边框和 diff 状态不再互相脱节；
- 菜单、弹层、composer、控件边框和滚动条开始共享语义 token；
- light 是主设计目标，dark 保持对等语义和可读性，不另建一套无关的色板。

这一步完成的是「视觉语言的底座」，不代表所有界面已经完成最终视觉设计。

### 2.3 Workbench 已具备继续优化的稳定基础

本轮没有把 Workbench 当作普通聊天容器重排，而是保留了 [架构契约](docs/01_桌面工作台/中间交互区域/架构契约.md) 规定的：

- `RunStage` 唯一滚动、定位、overlay 和 composer ownership；
- `ExecutionFeed` 只承载 narrative / turn；
- composer 只保留 `center` / `dock` 两种形态；
- prompt anchor、follow / paused、return-to-latest 和 composer 测量共用同一坐标与滚动体系。

在这些不变量内，streaming 内容增长、被动高度变化和恢复 follow 的行为已有更确定的 controller 与测试基线。后续才能安全地处理信息层级、密度、间距和状态表达。

### 2.4 高风险图表交互已转为可持续回归

Mermaid 不再依赖临时个人会话或一次性脚本验证。当前仓库已同时保留：

- 组件与 jsdom 回归，覆盖多实例渲染、缓存身份和 pan 状态计算；
- 使用隔离 userData 的 Electron Gallery fixture；
- 通过原生鼠标事件验证 inline 滚动边界、100% / 400% 拖动、reset、close / reopen 的仓库内回归入口。

这部分现在是设计系统的稳定能力，不再是下一阶段的主任务。

### 2.5 四个提交的阶段分工

| 提交 | 阶段作用 |
| --- | --- |
| `f72ab9fd` `feat(frontend): refine visuals and stabilize workbench` | 建立 skin / skeleton 边界，完成第一轮语义视觉收敛，稳定 Workbench follow 与 Mermaid 交互 |
| `910376a3` `fix(design-system): close visual accessibility gaps` | 收敛对比度、可访问名称、主题与作者规则 |
| `9b3bc07e` `fix(mermaid): isolate cached svg instances` | 建立 Mermaid 多实例唯一身份与对应回归 |
| `0135d7d6` `test(mermaid): add real Electron interaction regression` | 把 Electron 原生事件链验收收敛成隔离、可重复的仓库测试资产 |