# AwakenProductLogo

`AwakenProductLogo` 是 Awaken 官方方形产品标志的唯一 React 组件入口，Gallery 显示名为“产品 LOGO”。组件直接拥有 `src/assets/product-logo.png`，consumer 不再复制、覆盖或注入另一份 Awaken 方形图标。

## 用途

- 产品身份、连接关系和品牌组合中的方形 Awaken 标志。
- `sm`、`md`、`lg` 分别对应 32px、40px、64px。
- 默认提供 `Awaken` 替代文本；已有等价文字上下文时使用 `decorative`。

横向 `AWAKEN` 字标仍由 `logo-wordmark.svg` 负责，不得用横向字标替代本组件。
