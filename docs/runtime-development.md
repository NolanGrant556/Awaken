# Runtime Internal Verification

本文档只面向 `awaken-core` / Go runtime 的内部测试与验证，不是 Awaken 使用流程，也不是独立启动入口。Awaken 的本地使用入口始终是：

```bash
npm run dev:desktop
```

## 验证 runtime/server

```bash
go test ./src/runtime ./src/server ./src/agentcore
```

## 配置

- `AWAKEN_CONFIG_DIR`：指定 `.env` 与 `config.toml` 所在目录。
- `AWAKEN_DEFAULT_WORKSPACE`：指定 `/v1/runtime/info` 返回的默认 workspace。
- 未设置这些环境变量时，runtime 保持 Go 侧默认行为。

## Smoke

```bash
go test ./src/server -run 'TestPlainQASmoke|TestReadOnlyFileSmoke|TestFileMutation(Approval|Reject|Artifact|PatchAndTurnDiff)Smoke' -count=1 -v
```

这些 smoke 会在测试进程内启动本地 server，不形成 Awaken 的第三个使用入口。
