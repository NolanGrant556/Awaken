# Desktop Packaging Readiness

`electron-builder` 当前可以完成本地 Desktop 打包，但会给出几个正式分发前需要补齐的提醒。

## 当前提醒

- `description` / `author` 缺失：`apps/desktop/package.json` 还没有应用描述和作者信息。
- 使用默认 Electron 图标：还没有配置 Awaken 自己的 `.icns` / app icon。
- macOS code signing skipped：本机没有有效的 `Developer ID Application` 证书，打包产物未签名、未公证。

## 影响

这些提醒不影响本机开发测试、统一启动流程、真实 LLM run 或 runtime supervisor 验收。

正式分发给其他 macOS 用户时会有影响：未签名/未公证的 app 可能触发 Gatekeeper，出现“无法验证开发者”等安装或启动提示。

## 正式发布前补齐

- 在 `apps/desktop/package.json` 补充 `description` 和 `author`。
- 为 Electron 配置 Awaken app icon。
- 配置 Apple Developer ID 证书，完成 macOS signing 和 notarization。

当前结论：这是 release packaging readiness 问题，不是 Desktop v1 本地可用性 blocker。
