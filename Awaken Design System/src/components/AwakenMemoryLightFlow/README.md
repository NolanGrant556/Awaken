# AwakenMemoryLightFlow

`AwakenMemoryLightFlow` 是一个独立、纯装饰性的横向光流 primitive。它以 Canvas 绘制细丝、短颗粒、薄雾和少量锐利高光，不展示业务文字、数据、状态或可操作控件；记忆页头是它的主要消费方，Composer 推理强度轨道只复用其紧凑绘制变体。`AwakenMemoryLightFlowLayout` 是记忆页正式的 UI-only 组合布局，统一承载光流页头、标题和现有 `AwakenMemorySettings` 内容。

## 边界

- 组件只接收 `theme`、`tone`、`variant`、`className` 和可选无障碍标签；默认 `memory + ambient` 保持记忆页视觉，`compact` 为小尺寸容器降低细丝与颗粒数量，并提高线宽、亮度和局部辉光以保持可读动势。
- 不读取 Memory、Electron、runtime、store、文件系统或网络。
- 不触发回调，不改变记忆设置状态，也不代表运行、焦点或选中状态。
- Gallery 与真实 Desktop 都通过 `AwakenMemoryLightFlowLayout` 组合现有 `AwakenMemorySettings`，两端共用相同的脑形图标标题、标题位置、页头高度、开关清单与记忆管理布局；光流页头从外层 `AwakenSettings` 当前分区继承标题与 `settings-memory` concept，因此与左侧“记忆”导航统一消费 `settings-memory -> brain`。Design System 拥有带来源与许可记录的 authored 几何，图标只作装饰且不进入可访问名称。
- 光流页头横向贴满消费容器，不再套独立卡片外框；标题下方直接进入开关清单，不额外显示“记忆控制”说明行。
- 记忆组合根节点承接完整纵向滚动并隐藏会占用额外 gutter 的原生滚动条；光流页头属于同一滚动内容并随页面移动。无轨道细滑块只绘制在页头下方的白色记忆视口右缘，高度限制在 44–72px，并按完整 `scrollHeight` 映射位置，不会缩短滚轮、触控板或触摸滚动的真实范围。Gallery 与 Desktop 使用同一实现。
- 真实消费只传入当前生效的 `light` / `dark` 主题，不增加记忆开关、外观偏好或业务回调。
- `AwakenMemoryLightFlowLayout` 只组合 children，不持有 Memory 数据或回调；真实 `MemorySettingsPanel` 仍拥有全部数据加载和副作用。
- Composer 推理强度轨道仅复用同一 Canvas 生命周期与光流绘制器：普通档使用 `effort` 色调，最右 `max` 使用 `danger` 红色色调；档位状态和交互仍由 `AwakenComposerBar` 持有。
- Gallery 通过“记忆设置”页面 Demo 观测该组合，不为光流 primitive 注册重复的顶层入口。
- Gallery 场景使用随窗口变化的固定高度视口；超出的现有记忆内容由场景内部滚轮浏览，不会继续撑长整个 Gallery 页面，也不会显示滚动条轨道。
- Gallery 借鉴纵向设置页的排版方法，把 Awaken 现有四个开关组织成单列清单；新增演示项只会向下追加，不会重新排列已有项目。
- 开关清单与记忆管理区共用同一居中内容宽度；下方使用“记忆库 / 最近动态”双视图、全宽主题行和按需详情弹窗，不再保留固定主从面板。
- Gallery 的搜索、四个现有筛选、手动整理、记忆列表和正文详情保持原有功能；最近动态只投影当前 topic 快照与既有 Dream task/report，不引入趋势统计、完整版本历史、文件管理、导入导出或其他对标产品功能。

## 动效

- 细丝铺满整个顶部，每根线拥有独立的波速、相位和细节起伏，并统一从左向右推进。
- 不绘制雾带或独立粗主线，避免整束线条读成一条彩带。
- 默认高度压缩为正常页头尺度，Gallery 不再用大面积顶部留白制造展示舞台。
- `prefers-reduced-motion: reduce` 下停止循环并保留静态画面。
- 绘制频率限制为约 20 FPS，Canvas 像素倍率上限为 1.25；线条和颗粒的随机参数在模块初始化时预计算，逐帧不再重复生成。
- 离开视口、切到后台标签页或卸载记忆分区后停止绘制，避免不可见时持续占用渲染资源。

## 文件

- `AwakenMemoryLightFlow.tsx`：Canvas 生命周期、可见性和 reduced-motion 处理。
- `AwakenMemoryLightFlow.model.ts`：确定性光流建模与绘制。
- `AwakenMemoryLightFlow.css`：组件表面和主题融合。
- `AwakenMemoryLightFlowLayout.tsx` / `.css`：Gallery 与 Desktop 共用的光流记忆页组合布局。
- `AwakenMemoryLightFlow.states.ts`：浅色与深色纯 UI 状态。
- 组件在 `AwakenMemorySettings.demo.tsx` 中随完整记忆设置页观测；旧 `awaken-memory-light-flow` URL 重定向到该入口。
