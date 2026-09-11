# 模型供应商 Logo

这里保存模型配置界面和 Composer 使用的本地产品 Logo，避免 Renderer 在运行时加载第三方图片或远程资源。优先使用 SVG；用户确认的原图和本机 App Icon 保留为高分辨率 PNG。Kimi App Icon 原图自身带安全留白，因此由 `AwakenProviderLogo` 单独提高容器占比，不裁切或重绘官方资源。

| 文件 | 产品 | 入库与核对来源 |
|---|---|---|
| `kimi.png` | Kimi | 从本机 `/Applications/Kimi.app/Contents/Resources/icon.icns` 提取的 1024 × 1024 App Icon |
| `minimax.svg` | MiniMax | [Lobe Icons MiniMax](https://lobehub.com/icons/minimax)，并与 MiniMax 产品标识核对 |
| `glm.png` | GLM / Z.ai | 用户确认的 640 × 640 原图，并与 [Z.ai 当前标识](https://z-cdn.chatglm.cn/z-ai/static/logo.svg) 核对 |
| `qwen.svg` | Qwen | [Lobe Icons Qwen](https://lobehub.com/icons/qwen)，并与 Qwen 产品标识核对 |
| `doubao.png` | Doubao | 用户确认的 640 × 640 彩色原图，并与 [Lobe Icons Doubao](https://lobehub.com/icons/doubao) 核对 |
| `deepseek.svg` | DeepSeek | [Lobe Icons DeepSeek](https://lobehub.com/icons/deepseek)，并与 DeepSeek 产品标识核对 |
| `volcengine.svg` | 火山引擎 | [theSVG Volcengine](https://thesvg.org/icon/volcengine)，并与[火山引擎官方图标](https://portal.volccdn.com/obj/volcfe/misc/favicon.png)核对 |
| `openai.svg` | OpenAI | [Lobe Icons OpenAI](https://lobehub.com/icons/openai) |
| `anthropic.svg` | Anthropic | [Lobe Icons Anthropic](https://lobehub.com/icons/anthropic) |
| `gemini.svg` | Google Gemini | [Lobe Icons Gemini](https://lobehub.com/icons/gemini) |

SVG 来源为 MIT 授权的品牌图标资源；Kimi 图标来自用户本机安装的官方 App，品牌标识本身仍归各自权利人所有。Kimi API 与 Kimi Coding Plan 共用 Kimi Logo，Qwen API 与 Qwen Token Plan 共用 Qwen Logo，火山方舟、火山 Coding Plan 与火山 Agent Plan 共用火山引擎 Logo。

新增 LLM provider 时，需要同时补齐本目录品牌资源、`AwakenProviderBrand` / `PROVIDER_LOGOS` 和 Desktop 的 `model-provider-brand.ts` 映射。模型设置、Composer 与用量窗口共同消费这套映射；未知 provider 只显示首字母兜底，不在任一 consumer 内另建 Logo 规则。
