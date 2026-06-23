# AGENTS.md

## 测试与验收

测试、验收可以调用真实 LLM。对 Awaken 的 runtime、agent loop、工具执行、approval、SSE 等真实产品路径，缺少 LLM 参与的验证通常不能代表实际可用性；需要使用真实 provider 做 smoke 或 acceptance 验证。

## 打开 / 启动 Awaken

### 默认目标

当用户说“打开 Awaken”时，默认指打开 Awaken Desktop，而不是设计系统 Gallery。

除非用户明确说“打开设计系统”或“打开组件库”，不要启动 `npm run open:ui` / `npm run dev:ui`。

### 正式入口

正式入口只有仓库根目录的：

```bash
cd /Users/nolangrant/Awaken
npm run dev:desktop
```

不要把 `cmd/awaken-core`、Vite renderer、Electron 子脚本当成独立使用入口；Desktop 会自己启动和管理 `awaken-core`。

### 启动前检查

启动前先确认是否已经在运行，避免重复启动：

```bash
screen -ls
pgrep -afil "awaken-core|electron . --dev|vite --host 127.0.0.1 --port 5174|awaken-dev-desktop"
lsof -nP -iTCP:5174 -sTCP:LISTEN || true
```

如果已经有 Awaken Desktop 在运行，只需要把窗口切到前台，不要重启：

```bash
osascript -e 'tell application "Electron" to activate'
```

### 持续运行

在 Codex 这类一次性 shell 环境里，需要让 Awaken 持续运行时，优先用 `screen` 承载一次启动：

```bash
screen -dmS awaken-dev-desktop bash -lc 'cd /Users/nolangrant/Awaken && npm run dev:desktop'
```

不要先前台启动成功后再 `Ctrl-C`，也不要依赖普通 `nohup ... &` 来承载长期 dev server；本机环境可能回收这类后台子进程，导致看起来像“启动失败”或需要重复启动。

### 成功确认

启动后用这些信号确认成功：

- `screen -ls` 里只有一个 `awaken-dev-desktop` 会话。
- `lsof -nP -iTCP:5174 -sTCP:LISTEN` 能看到 renderer 监听。
- `pgrep -afil "awaken-core|electron . --dev"` 能看到 Electron 和 `awaken-core`。
- 系统窗口标题能看到 `Awaken`。
