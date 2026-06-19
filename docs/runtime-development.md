# Runtime Development

本文档只面向 `awaken-core` / Go runtime 开发调试，不是 Desktop 使用流程。Desktop 开发与使用入口始终是：

```bash
npm run dev:desktop
```

## 直接运行 awaken-core

需要单独调试 HTTP route、SSE、runtime state、tool policy 或 provider transport 时，可以直接运行：

```bash
go run ./cmd/awaken-core
```

默认监听地址由 `AWAKEN_CORE_ADDR` 控制，未设置时为 `127.0.0.1:8787`：

```bash
AWAKEN_CORE_ADDR=127.0.0.1:9999 go run ./cmd/awaken-core
```

## 配置

- `AWAKEN_CONFIG_DIR`：指定 `.env` 与 `config.toml` 所在目录。
- `AWAKEN_DEFAULT_WORKSPACE`：指定 `/v1/runtime/info` 返回的默认 workspace。
- 未设置这些环境变量时，runtime 保持 Go 侧默认行为。

## Smoke

```bash
curl --noproxy 127.0.0.1 http://127.0.0.1:8787/health
```

本机有代理环境变量时，localhost smoke 优先使用 `--noproxy 127.0.0.1`。
