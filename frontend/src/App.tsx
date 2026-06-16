import { useState } from "react";
import { AppShell } from "./components/AppShell";
import { ChatComposer } from "./components/ChatComposer";
import { ChatHome } from "./components/ChatHome";
import { Sidebar } from "./components/Sidebar";
import { Icon, IconButton, Button, Pill, StatusPill, Avatar, Divider, Kbd } from "./components";
import { Composer, SendButton, ModelSelector, ModeToggle, QuickActionChip, QuickActionGrid } from "./components";
import type { ModelOption, ComposerMode } from "./types";
import { conversations, projects } from "./data/mockData";
import { Wand2, Code, FileText, Search, Zap } from "lucide-react";

type PreviewMode = "full" | "sidebar" | "composer" | "primitives" | "phase2";

const previewTabs: Array<{ id: PreviewMode; label: string }> = [
  { id: "full", label: "完整主界面" },
  { id: "sidebar", label: "侧边栏近景" },
  { id: "composer", label: "聊天框近景" },
  { id: "primitives", label: "Phase 1 基础组件" },
  { id: "phase2", label: "Phase 2 输入组件" },
];

const mockModels: ModelOption[] = [
  { id: "qwen3.7-max", label: "Qwen3.7-Max" },
  { id: "gpt-4o", label: "GPT-4o" },
  { id: "claude-3.7", label: "Claude 3.7 Sonnet" },
  { id: "deepseek-v3", label: "DeepSeek-V3" },
];

const quickActions = [
  { id: "1", label: "写代码", prompt: "帮我写一个 React 组件，包含输入框和提交按钮", icon: <Code size={14} strokeWidth={1.85} /> },
  { id: "2", label: "查文档", prompt: "解释一下 React 的 useEffect 钩子", icon: <FileText size={14} strokeWidth={1.85} /> },
  { id: "3", label: "Debug", prompt: "帮我调试这个错误：TypeError: Cannot read property", icon: <Search size={14} strokeWidth={1.85} /> },
  { id: "4", label: "优化", prompt: "优化这段代码的性能", icon: <Zap size={14} strokeWidth={1.85} /> },
  { id: "5", label: "生成测试", prompt: "为这段代码生成单元测试", icon: <Wand2 size={14} strokeWidth={1.85} /> },
];

function PrimitivePreview() {
  return (
    <div className="zoom-stage" style={{ justifyContent: "flex-start", paddingTop: 96 }}>
      <div
        style={{
          width: "min(860px, calc(100vw - 80px))",
          display: "grid",
          gap: 24,
          padding: 24,
          border: "1px solid var(--aw-border-main)",
          borderRadius: "var(--aw-radius-lg)",
          background: "var(--aw-bg-white)",
          color: "var(--aw-text-primary)",
        }}
      >
        <h2 style={{ margin: 0, fontSize: "var(--aw-font-size-xl)", lineHeight: "var(--aw-line-height-xl)" }}>
          Phase 1 Primitives
        </h2>

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>Icon</h3>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Icon>
              <path d="M12 4v16" />
              <path d="M12 8c-4.2 0-6.4 2.8-7 6.6 4.2.2 6.8-1.7 7-6.6z" />
              <path d="M12 8c4.2 0 6.4 2.8 7 6.6-4.2.2-6.8-1.7-7-6.6z" />
            </Icon>
            <Icon size={24} strokeWidth={2}>
              <path d="M5 12h14" />
              <path d="M12 5v14" />
            </Icon>
            <span role="img" aria-label="语义图标" title="语义图标">
              <Icon ariaHidden={false}>
                <circle cx="12" cy="12" r="9" />
                <path d="M12 7v6l4 2" />
              </Icon>
            </span>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>IconButton</h3>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <IconButton label="Square icon">
              <Icon>
                <path d="M5 12h14" />
                <path d="M12 5v14" />
              </Icon>
            </IconButton>
            <IconButton label="Circle icon" variant="circle">
              <Icon>
                <path d="M5 12h14" />
                <path d="M12 5v14" />
              </Icon>
            </IconButton>
            <IconButton label="Disabled" disabled>
              <Icon>
                <path d="M5 12h14" />
              </Icon>
            </IconButton>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>Button</h3>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
            <Button>Default</Button>
            <Button variant="primary">Primary</Button>
            <Button variant="ghost">Ghost</Button>
            <Button loading>Loading</Button>
            <Button disabled>Disabled</Button>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>Pill</h3>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
            <Pill>Default</Pill>
            <Pill variant="bordered">Bordered</Pill>
            <Pill active>Active</Pill>
            <Pill disabled>Disabled</Pill>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>StatusPill</h3>
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
            <StatusPill tone="neutral">Neutral</StatusPill>
            <StatusPill tone="local">Local</StatusPill>
            <StatusPill tone="permission">Permission</StatusPill>
            <StatusPill tone="success">Success</StatusPill>
            <StatusPill tone="warning">Warning</StatusPill>
            <StatusPill tone="error">Error</StatusPill>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>Avatar</h3>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <Avatar fallback="NG" />
            <Avatar fallback="ab" size="sm" />
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>Kbd</h3>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <Kbd>N</Kbd>
            <Kbd platform="windows">N</Kbd>
            <Kbd>Shift+P</Kbd>
          </div>
        </section>
      </div>
    </div>
  );
}

function Phase2Preview() {
  const [value, setValue] = useState("");
  const [mode, setMode] = useState<ComposerMode>("Plan");
  const [model, setModel] = useState<ModelOption>(mockModels[0]);
  const [submitCount, setSubmitCount] = useState(0);
  const [lastSubmitted, setLastSubmitted] = useState("");

  const isReady = value.length > 0;

  const handleSubmit = () => {
    if (isReady) {
      setSubmitCount((c) => c + 1);
      setLastSubmitted(value);
    }
  };

  const handleQuickAction = (prompt: string) => {
    setValue(prompt);
  };

  return (
    <div className="zoom-stage" style={{ justifyContent: "flex-start", paddingTop: 48 }}>
      <div
        style={{
          width: "min(860px, calc(100vw - 80px))",
          display: "grid",
          gap: 24,
          padding: 24,
          border: "1px solid var(--aw-border-main)",
          borderRadius: "var(--aw-radius-lg)",
          background: "var(--aw-bg-white)",
          color: "var(--aw-text-primary)",
        }}
      >
        <h2 style={{ margin: 0, fontSize: "var(--aw-font-size-xl)", lineHeight: "var(--aw-line-height-xl)" }}>
          Phase 2 输入组件
        </h2>

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            Composer（完整组合）
          </h3>
          <Composer
            value={value}
            onChange={setValue}
            onSubmit={handleSubmit}
            mode={mode}
            onModeChange={setMode}
            model={model}
            models={mockModels}
            onModelChange={setModel}
            isReady={isReady}
            placeholder="输入任务，按 Cmd/Ctrl+Enter 提交..."
          />
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            提交状态
          </h3>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              fontSize: "var(--aw-font-size-sm)",
              color: "var(--aw-text-secondary)",
            }}
          >
            <span>提交次数: <strong style={{ color: "var(--aw-text-primary)" }}>{submitCount}</strong></span>
            {lastSubmitted && (
              <span className="aw-truncate" style={{ maxWidth: 400 }}>
                上次提交: <strong style={{ color: "var(--aw-text-primary)" }}>{lastSubmitted}</strong>
              </span>
            )}
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            SendButton（独立）
          </h3>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>未就绪:</span>
            <SendButton ready={false} />
            <span style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>就绪:</span>
            <SendButton ready={true} />
            <span style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>禁用:</span>
            <SendButton ready={true} disabled />
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            ModelSelector（独立）
          </h3>
          <ModelSelector
            value={model}
            options={mockModels}
            onChange={setModel}
          />
          <div style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            当前选中: <strong style={{ color: "var(--aw-text-primary)" }}>{model.label}</strong>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            ModeToggle（独立）
          </h3>
          <ModeToggle mode={mode} onChange={setMode} />
          <div style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            当前模式: <strong style={{ color: "var(--aw-text-primary)" }}>{mode}</strong>
          </div>
        </section>

        <Divider />

        <section style={{ display: "grid", gap: 8 }}>
          <h3 style={{ margin: 0, fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            QuickActionChip / QuickActionGrid
          </h3>
          <QuickActionGrid>
            {quickActions.map((action) => (
              <QuickActionChip
                key={action.id}
                icon={action.icon}
                label={action.label}
                prompt={action.prompt}
                onClick={handleQuickAction}
              />
            ))}
          </QuickActionGrid>
          <div style={{ fontSize: "var(--aw-font-size-sm)", color: "var(--aw-text-tertiary)" }}>
            点击 chip 会将 prompt 填入上方 Composer
          </div>
        </section>
      </div>
    </div>
  );
}

export default function App() {
  const [activeItemId, setActiveItemId] = useState("design-components");
  const [previewMode, setPreviewMode] = useState<PreviewMode>("full");

  return (
    <div className={`preview-root preview-${previewMode}`}>
      <div className="preview-switcher" aria-label="预览视图">
        {previewTabs.map((tab) => (
          <button
            key={tab.id}
            className={previewMode === tab.id ? "is-active" : ""}
            onClick={() => setPreviewMode(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {previewMode === "full" ? (
        <AppShell
          sidebar={
            <Sidebar
              conversations={conversations}
              projects={projects}
              activeItemId={activeItemId}
              onSelectItem={setActiveItemId}
            />
          }
          main={<ChatHome />}
        />
      ) : null}

      {previewMode === "sidebar" ? (
        <div className="zoom-stage sidebar-zoom">
          <Sidebar
            conversations={conversations}
            projects={projects}
            activeItemId={activeItemId}
            onSelectItem={setActiveItemId}
          />
        </div>
      ) : null}

      {previewMode === "composer" ? (
        <div className="zoom-stage composer-zoom">
          <div className="composer-demo-heading">
            <span>聊天输入组件</span>
            <strong>主界面就是聊天框</strong>
          </div>
          <ChatComposer
            placeholder="告诉 Awaken 你要完成的任务"
            modelName="Qwen3.7-Max"
            workspaceName="Awaken"
            permissionLabel="完全访问"
          />
        </div>
      ) : null}

      {previewMode === "primitives" ? <PrimitivePreview /> : null}
      {previewMode === "phase2" ? <Phase2Preview /> : null}
    </div>
  );
}
