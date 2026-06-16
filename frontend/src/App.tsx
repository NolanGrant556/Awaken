import { useState } from "react";
import { AppShell } from "./components/AppShell";
import { ChatComposer } from "./components/ChatComposer";
import { ChatHome } from "./components/ChatHome";
import { Sidebar } from "./components/Sidebar";
import { Icon, IconButton, Button, Pill, StatusPill, Avatar, Divider, Kbd } from "./components";
import { conversations, projects } from "./data/mockData";

type PreviewMode = "full" | "sidebar" | "composer" | "primitives";

const previewTabs: Array<{ id: PreviewMode; label: string }> = [
  { id: "full", label: "完整主界面" },
  { id: "sidebar", label: "侧边栏近景" },
  { id: "composer", label: "聊天框近景" },
  { id: "primitives", label: "Phase 1 基础组件" },
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
    </div>
  );
}
