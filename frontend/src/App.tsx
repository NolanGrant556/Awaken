import { useState } from "react";
import { AppShell } from "./components/AppShell";
import { ChatComposer } from "./components/ChatComposer";
import { ChatHome } from "./components/ChatHome";
import { Sidebar } from "./components/Sidebar";
import { conversations, projects } from "./data/mockData";

type PreviewMode = "full" | "sidebar" | "composer";

const previewTabs: Array<{ id: PreviewMode; label: string }> = [
  { id: "full", label: "完整主界面" },
  { id: "sidebar", label: "侧边栏近景" },
  { id: "composer", label: "聊天框近景" },
];

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
    </div>
  );
}
