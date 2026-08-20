# AwakenProductLogo

`AwakenProductLogo` 是 Awaken 官方产品标志的唯一 React 组件入口，Gallery 显示名为“产品 LOGO”。

## 三件套

1. **Mark**（默认 `variant="mark"`）— 透明底双柱 A，`product-mark.svg`。颜色跟随 `--aw-text-primary`。应用内 chrome、连接器与品牌组合使用它。
2. **App icon**（`variant="icon"`）— 已创作的 256 PNG（黑底 + 浅色 squircle + 标记）。只用于 Dock、开始菜单和安装包。
3. **Wordmark** — `logo-wordmark.svg`，只用于欢迎空态、About 与安装封面。欢迎区默认视觉宽度 220px，窄屏 200px。

三件套不得互相裁切或互相替代。安全区为标记高度的四分之一。最小应用内尺寸为 16px；组件公开 `sm / md / lg` 对应 32 / 40 / 64。

## 用法

- 默认 `variant` 是 `mark`。需要完整 App Icon 构图时才传 `icon`。
- `sm`、`md`、`lg` 分别对应 32px、40px、64px。
- 默认提供 `Awaken` 替代文本；已有等价文字上下文时使用 `decorative`。
- 不要把标记改成 `--aw-accent` 紫色，不要描边、投影、拉伸或旋转。
- consumer 不得再复制、覆盖或注入另一份 Awaken 方形图标。
- 完整导出矩阵（favicon、DMG、ICO、文档封面）当前不支持。
