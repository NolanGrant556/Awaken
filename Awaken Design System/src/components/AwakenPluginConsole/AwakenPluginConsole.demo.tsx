import { useState } from "react";
import {
  AwakenPluginConsole,
  type AwakenPluginConsoleComponent,
  type AwakenPluginConsoleDetail,
} from "./AwakenPluginConsole";

const components: AwakenPluginConsoleComponent[] = [
  { logicalId: "skill/review", logicalIdEncoded: "c2tpbGwvcmV2aWV3", kind: "skill", name: "代码评审", description: "原生 Skill", sourcePath: "skills/review/SKILL.md", sourceDialect: "cursor", mappingState: "native", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "ready", integrityState: "verified", switchable: true },
  { logicalId: "mcp/browser", logicalIdEncoded: "bWNwL2Jyb3dzZXI", kind: "mcp_server", name: "browser", description: "提供浏览器自动化工具", sourcePath: ".mcp.json", sourceDialect: "cursor", mappingState: "adapted", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "ready", integrityState: "verified", switchable: true, mcp: { shortName: "browser", transport: "stdio", connectionState: "ready", toolsLoaded: true, toolCount: 8 } },
  { logicalId: "rule/typescript", logicalIdEncoded: "cnVsZS90eXBlc2NyaXB0", kind: "rule", name: "TypeScript 约束", description: "按文件匹配", sourcePath: ".cursor/rules/typescript.mdc", sourceDialect: "cursor", mappingState: "adapted", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "ready", integrityState: "verified", switchable: true, rule: { activation: "file_globs", globs: ["**/*.ts", "**/*.tsx"] } },
  { logicalId: "variable/api-token", logicalIdEncoded: "dmFyaWFibGUvYXBpLXRva2Vu", kind: "variable", name: "API_TOKEN", description: "缺少必填配置", sourcePath: ".cursor-plugin/plugin.json", sourceDialect: "cursor", mappingState: "adapted", validation: "valid", desiredState: "enabled", configurationState: "needs_configuration", runtimeState: "not_loaded", integrityState: "verified", switchable: false, variable: { key: "API_TOKEN", type: "string", required: true, secret: true } },
  { logicalId: "app/dashboard", logicalIdEncoded: "YXBwL2Rhc2hib2FyZA", kind: "app", name: "Dashboard", description: "宿主暂不运行 App UI", sourcePath: "apps/dashboard.json", sourceDialect: "codex", mappingState: "recognized_unsupported", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "not_loaded", integrityState: "verified", switchable: false },
  { logicalId: "command/legacy", logicalIdEncoded: "Y29tbWFuZC9sZWdhY3k", kind: "command", name: "legacy", description: "当前方言不加载", sourcePath: "commands/legacy.md", sourceDialect: "agent_plugins", mappingState: "ignored_by_dialect", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "not_loaded", integrityState: "verified", switchable: false, command: { invocationName: "/legacy", argumentHint: "[path]" } },
  { logicalId: "hook/broken", logicalIdEncoded: "aG9vay9icm9rZW4", kind: "hook", name: "broken", description: "事件映射无效", sourcePath: "hooks/hooks.json", sourceDialect: "claude_code", mappingState: "invalid", validation: "invalid", desiredState: "enabled", configurationState: "ready", runtimeState: "error", integrityState: "verified", switchable: false, diagnosticCount: 1, hook: { sourceEvent: "postCompact", handlerType: "command" } },
  { logicalId: "agent/research", logicalIdEncoded: "YWdlbnQvcmVzZWFyY2g", kind: "agent", name: "research", description: "安装树已被外部修改", sourcePath: ".cursor/agents/research.md", sourceDialect: "cursor", mappingState: "adapted", validation: "valid", desiredState: "enabled", configurationState: "ready", runtimeState: "blocked", integrityState: "tampered", switchable: false, agent: { model: "inherit", readOnly: true } },
];

export function AwakenPluginConsoleDemo() {
  const [activeTab, setActiveTab] = useState<"overview" | "preview" | "source" | "diagnostics">("overview");
  const [detail, setDetail] = useState<AwakenPluginConsoleDetail | null>(null);
  return (
    <div style={{ height: 720 }} aria-label="AwakenPluginConsole demo">
      <AwakenPluginConsole
        name="sample-plugin"
        displayName="Sample Plugin"
        version="1.0.0"
        enabled
        components={components}
        selectedDetail={detail}
        activeDetailTab={activeTab}
        onBack={() => undefined}
        onEnabledChange={() => undefined}
        onUninstall={() => undefined}
        onComponentOpen={(component) => setDetail({ component, preview: component.description, source: component.sourcePath, diagnostics: component.mappingState === "invalid" ? [{ severity: "error", code: "component_invalid", message: "示例诊断" }] : [] })}
        onComponentEnabledChange={() => undefined}
        onDetailTabChange={setActiveTab}
        onDetailClose={() => setDetail(null)}
      />
    </div>
  );
}
