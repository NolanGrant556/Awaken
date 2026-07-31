# AwakenProjectHoverCard

`AwakenProjectHoverCard` 是侧栏项目行的悬停卡片。指针停在项目行上时，它在行的右侧展开这个项目由哪些文件夹组成，并允许点击单个文件夹在系统文件管理器中定位。侧栏树本身不把文件夹展开成子行，文件夹的完整视图和写操作留在编辑项目弹窗里。

## 文件

- `AwakenProjectHoverCard.tsx` - 组件实现。
- `AwakenProjectHoverCard.css` - 组件局部样式入口。
- 状态演示在 `AwakenProjectTree.states.ts` 的「悬停展示文件夹组成」里，因为卡片只能通过项目行触发。

## 用法约定

组件用 render prop 而不是包裹元素交出锚点：

```tsx
<AwakenProjectHoverCard card={item.hoverCard}>
  {(anchor) => <div className="aw-project-row" {...anchor}>…</div>}
</AwakenProjectHoverCard>
```

这一点是硬约束。项目行的激活、拖拽、菜单展开样式都挂在 `.aw-project-node > .aw-project-row` 直接子选择器上，插一层容器会静默打断它们。

`useHover` 配 `safePolygon`，指针可以从行斜着移进卡片而不关闭，否则文件夹行根本点不到。

项目行已有这张受控悬停卡片时不再输出原生 `title`，卡片里的文件夹按钮也只用可见文件夹名组成辅助技术名称，不把绝对路径交给浏览器原生提示。这样持续悬停只出现一层卡片，完整路径仍仅作为定位回调的内部 identity。

`disabled` 表示「此刻不该出现这张卡片」，由 `AwakenProjectTree` 在两种情形传入：拖拽排序进行中，避免卡片跟着拖影跑；项目行 `⋯` 菜单展开时，避免左右两个浮层同时占屏。转入 `disabled` 会连内部 open 状态一起清掉——只用 `isOpen` 遮住的话，指针没离开行时 `disabled` 一解除卡片就立刻弹回来，还绕过了开启延迟。

## 数据

`ProjectHoverCard` 定义在 `src/types.ts`，通过 `ProjectItem.hoverCard` 挂到项目行上。`folders` 为空时展示 `emptyFoldersLabel`，用于没有文件夹的项目。Gallery 只模拟回调，不打开系统对话框、不触碰磁盘。
