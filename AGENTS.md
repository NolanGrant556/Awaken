# AGENTS.md

## 测试与验收

测试、验收可以调用真实 LLM。对 Awaken 的 runtime、agent loop、工具执行、approval、SSE 等真实产品路径，缺少 LLM 参与的验证通常不能代表实际可用性；需要使用真实 provider 做 smoke 或 acceptance 验证。

## 打开 / 启动 Awaken

启动规则以 `/Users/nolangrant/Awaken/README.md` 的“入口契约”和“本地运行”为唯一来源。Coding Agent 需要打开、检查、诊断 Awaken 启动状态时，先读根 README，不要在 `AGENTS.md` 里维护第二套命令。

当用户说“打开 Awaken”时，默认含义、正式入口、状态检查、持续运行方式和成功确认标准，都按根 README 执行。除非用户明确说“打开设计系统”或“打开组件库”，不要切到组件 Gallery 入口。
