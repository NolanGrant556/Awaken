import { useState } from "react";
import { Icon, IconButton, Button, Pill, StatusPill, Avatar, Divider, Kbd } from "./components";
import { Composer, SendButton, ModelSelector, ModeToggle, QuickActionChip, QuickActionGrid } from "./components";
import {
  ProjectTree as NavProjectTree,
  Sidebar as AwakenSidebar,
  RunStep,
  RunTimeline,
  ArtifactGrid,
  ApprovalCard,
  Toast,
  PlatformProvider,
  usePlatform,
  AppShell,
  TopBar,
  WindowTitleBar,
  CenterStage,
} from "./components";
import type {
  ModelOption,
  ComposerMode,
  NavItem,
  ConversationItem,
  ProjectItem,
  RunStepItem,
  ArtifactItem,
  ApprovalRiskTone,
  PlatformKind,
} from "./types";

import { Wand2, Code, FileText, Search, Zap, MessageSquare, Puzzle, Clock3 } from "lucide-react";

type PreviewMode = "full" | "primitives" | "phase2" | "phase3" | "phase4" | "phase5";

const previewTabs: Array<{ id: PreviewMode; label: string }> = [
  { id: "full", label: "完整主界面" },
  { id: "primitives", label: "Phase 1 基础组件" },
  { id: "phase2", label: "Phase 2 输入组件" },
  { id: "phase3", label: "Phase 3 导航/侧栏" },
  { id: "phase4", label: "Phase 4 反馈组件" },
  { id: "phase5", label: "Phase 5 布局/平台" },
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

const phase3NavItems: NavItem[] = [
  { id: "nav-workflow", label: "WorkFlow", icon: <MessageSquare size={16} strokeWidth={1.85} /> },
  { id: "nav-plugins", label: "插件 & Skills", icon: <Puzzle size={16} strokeWidth={1.85} /> },
  { id: "nav-timer", label: "定时任务", icon: <Clock3 size={16} strokeWidth={1.85} /> },
];

const phase3LongText =
  "这是一个用于验证侧边栏长文本截断能力的超长标题当文本超过容器宽度时应当显示省略号而不是换行或撑破布局此处刻意写入超过一百个字符以确保截断逻辑被触发并可见Awaken组件库Phase3导航与侧边栏验收长文本场景测试用例结束";

const phase3Conversations: ConversationItem[] = [
  { id: "conv-1", label: "设计前端设计组件", timeLabel: "now" },
  { id: "conv-2", label: "组件库成熟度审查", timeLabel: "1h" },
  { id: "conv-long", label: phase3LongText, timeLabel: "2h" },
];

const phase3Projects: ProjectItem[] = [
  {
    id: "proj-product",
    label: "产品设计",
    timeLabel: "当前",
    children: [
      { id: "proj-product-1", label: "整理新工作台信息架构" },
      { id: "proj-product-2", label: "优化任务输入框交互" },
      { id: "proj-product-3", label: "定义组件状态规范" },
    ],
  },
  {
    id: "proj-research",
    label: "市场调研",
    timeLabel: "3",
    children: [
      { id: "proj-research-1", label: "收集竞品定价与定位" },
      { id: "proj-research-2", label: "生成用户访谈提纲" },
    ],
  },
  {
    id: "proj-loading",
    label: "数据同步中",
    timeLabel: "加载",
    isLoading: true,
    children: [{ id: "proj-loading-1", label: "正在拉取远程会话..." }],
  },
  {
    id: "proj-long",
    label: phase3LongText,
    timeLabel: "3",
    children: [{ id: "proj-long-1", label: "长名称项目下的子任务" }],
  },
];

const phase3CardStyle = {
  padding: 16,
  border: "1px solid var(--aw-border-main)",
  borderRadius: "var(--aw-radius-md)",
  background: "var(--aw-bg-white)",
  color: "var(--aw-text-primary)",
} as const;

const phase3H3Style = {
  margin: "0 0 10px",
  fontSize: "var(--aw-font-size-sm)",
  color: "var(--aw-text-tertiary)",
  fontWeight: "var(--aw-font-weight-bold)",
} as const;

const phase3CtrlBtnStyle = {
  height: 30,
  padding: "0 12px",
  border: "1px solid var(--aw-border-main)",
  borderRadius: "var(--aw-radius-sm)",
  background: "var(--aw-bg-white)",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-sm)",
  cursor: "pointer",
} as const;

function Phase3Preview() {
  const [activeNav, setActiveNav] = useState("nav-workflow");
  const [activeConv, setActiveConv] = useState("conv-1");
  const [activeProj, setActiveProj] = useState("proj-product");
  const [expandedIds, setExpandedIds] = useState<string[]>(["proj-product"]);
  const [convs, setConvs] = useState<ConversationItem[]>(phase3Conversations);
  const [projs, setProjs] = useState<ProjectItem[]>(phase3Projects);
  const [newTaskCount, setNewTaskCount] = useState(0);
  const [profileCount, setProfileCount] = useState(0);
  const [lastEvent, setLastEvent] = useState("就绪");

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "auto minmax(0, 1fr)",
        gap: 24,
        padding: "48px 32px",
        alignItems: "start",
        minHeight: "100vh",
      }}
    >
      <div
        style={{
          height: "min(760px, calc(100vh - 140px))",
          border: "1px solid var(--aw-border-main)",
          borderRadius: "var(--aw-radius-lg)",
          overflow: "hidden",
          boxShadow: "var(--aw-shadow-menu)",
        }}
      >
        <AwakenSidebar
          title="Awaken"
          navItems={phase3NavItems}
          activeNavId={activeNav}
          conversations={convs}
          activeConversationId={activeConv}
          projects={projs}
          activeProjectId={activeProj}
          expandedIds={expandedIds}
          onExpandedChange={setExpandedIds}
          profileName="Nolan"
          profileAvatarFallback="NG"
          newTaskShortcut="N"
          platform="macos"
          onSelectNav={(id) => {
            setActiveNav(id);
            setLastEvent(`导航选中: ${id}`);
          }}
          onSelectConversation={(id) => {
            setActiveConv(id);
            setLastEvent(`对话选中: ${id}`);
          }}
          onSelectProject={(id) => {
            setActiveProj(id);
            setLastEvent(`项目选中: ${id}`);
          }}
          onNewTask={() => {
            setNewTaskCount((c) => c + 1);
            setLastEvent("新建任务已触发");
          }}
          onProfileClick={() => {
            setProfileCount((c) => c + 1);
            setLastEvent("Profile 已点击");
          }}
        />
      </div>

      <div style={{ display: "grid", gap: 16, alignContent: "start" }}>
        <div style={phase3CardStyle}>
          <h3 style={phase3H3Style}>Smoke 状态</h3>
          <div
            style={{
              display: "grid",
              gap: 6,
              fontSize: "var(--aw-font-size-sm)",
              color: "var(--aw-text-secondary)",
            }}
          >
            <span>
              新建任务点击:{" "}
              <strong style={{ color: "var(--aw-text-primary)" }}>{newTaskCount}</strong>
            </span>
            <span>
              Profile 点击:{" "}
              <strong style={{ color: "var(--aw-text-primary)" }}>{profileCount}</strong>
            </span>
            <span>
              当前事件:{" "}
              <strong style={{ color: "var(--aw-text-primary)" }}>{lastEvent}</strong>
            </span>
            <span className="aw-truncate" style={{ maxWidth: 420 }}>
              展开节点:{" "}
              <strong style={{ color: "var(--aw-text-primary)" }}>
                {expandedIds.join(", ") || "(无)"}
              </strong>
            </span>
          </div>
        </div>

        <div style={phase3CardStyle}>
          <h3 style={phase3H3Style}>空状态 / 长文本 控制台</h3>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button
              style={phase3CtrlBtnStyle}
              onClick={() => setConvs(convs.length ? [] : phase3Conversations)}
            >
              {convs.length ? "清空对话" : "恢复对话"}
            </button>
            <button
              style={phase3CtrlBtnStyle}
              onClick={() => setProjs(projs.length ? [] : phase3Projects)}
            >
              {projs.length ? "清空项目" : "恢复项目"}
            </button>
          </div>
          <p
            style={{
              fontSize: "var(--aw-font-size-xs)",
              color: "var(--aw-text-tertiary)",
              margin: "8px 0 0",
            }}
          >
            清空后可在侧栏查看 ConversationList / ProjectTree 的 empty 状态。长文本节点已包含在对话与项目列表中。
          </p>
        </div>

        <div style={phase3CardStyle}>
          <h3 style={phase3H3Style}>非受控 ProjectTree（defaultExpandedIds）</h3>
          <div
            style={{
              width: 280,
              border: "1px solid var(--aw-border-light)",
              borderRadius: "var(--aw-radius-md)",
              padding: 8,
              background: "var(--aw-bg-nav)",
            }}
          >
            <NavProjectTree
              items={phase3Projects}
              activeId={activeProj}
              defaultExpandedIds={["proj-product", "proj-loading"]}
              onSelect={(id) => {
                setActiveProj(id);
                setLastEvent(`(非受控) 项目选中: ${id}`);
              }}
            />
          </div>
          <p
            style={{
              fontSize: "var(--aw-font-size-xs)",
              color: "var(--aw-text-tertiary)",
              margin: "8px 0 0",
            }}
          >
            此树使用 defaultExpandedIds 非受控模式，内部自管展开状态。
          </p>
        </div>
      </div>
    </div>
  );
}

const phase4CardStyle = {
  padding: 16,
  border: "1px solid var(--aw-border-main)",
  borderRadius: "var(--aw-radius-md)",
  background: "var(--aw-bg-white)",
  color: "var(--aw-text-primary)",
} as const;

const phase4H3Style = {
  margin: "0 0 10px",
  fontSize: "var(--aw-font-size-sm)",
  color: "var(--aw-text-tertiary)",
  fontWeight: "var(--aw-font-weight-bold)",
} as const;

const phase4BtnStyle = {
  height: 30,
  padding: "0 12px",
  border: "1px solid var(--aw-border-main)",
  borderColor: "var(--aw-border-main)",
  borderRadius: "var(--aw-radius-sm)",
  background: "var(--aw-bg-white)",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-sm)",
  cursor: "pointer",
} as const;

const phase4LongArtifactTitle =
  "这是一个超长标题用于验证 ArtifactCard 和 ArtifactGrid 的长文本截断能力，不应撑破卡片布局，Awaken 组件库 Phase 4 验收用例";

const phase4LongArtifactDesc =
  "这是一个超长的描述文本，用于验证描述在两行截断后的显示效果。当文本长度超过两行时应该被截断并显示省略号，而不是无限扩展或撑破网格布局，这是 Phase 4 的关键验收场景之一。";

const phase4ArtifactSets: Record<string, ArtifactItem[]> = {
  normal: [
    { id: "a1", title: "需求文档.md", description: "整理后的产品需求文档" },
    { id: "a2", title: "接口设计.json", description: "后端 API 接口定义" },
    { id: "a3", title: "测试用例.csv", description: "核心流程覆盖用例" },
  ],
  empty: [],
  long: [
    { id: "a-long-1", title: phase4LongArtifactTitle, description: phase4LongArtifactDesc },
    { id: "a-long-2", title: phase4LongArtifactTitle, description: phase4LongArtifactDesc },
    { id: "a-long-3", title: phase4LongArtifactTitle, description: phase4LongArtifactDesc },
  ],
};

const phase4ApprovalTitles: Record<ApprovalRiskTone, string> = {
  safe: "读取本地文件",
  warning: "执行 shell 命令",
  danger: "删除项目文件",
};

const phase4ApprovalDescs: Record<ApprovalRiskTone, string> = {
  safe: "将读取 /Users/nolan/project/README.md",
  warning: "即将执行：npm run build",
  danger: "即将执行：rm -rf /tmp/awaken-staging",
};

const phase4ToneLabels: Record<ApprovalRiskTone, string> = {
  safe: "安全",
  warning: "警告",
  danger: "危险",
};

function Phase4Preview() {
  const [timelineStatus, setTimelineStatus] = useState<
    "ready" | "running" | "done" | "error"
  >("running");
  const [artifactSet, setArtifactSet] = useState<"normal" | "empty" | "long">(
    "normal"
  );
  const [approvalOpen, setApprovalOpen] = useState(false);
  const [approvalTone, setApprovalTone] = useState<ApprovalRiskTone>("safe");
  const [approvalSmoke, setApprovalSmoke] = useState("无");
  const [toastVisible, setToastVisible] = useState(false);
  const [toastMsg, setToastMsg] = useState("");

  const timelineSteps: RunStepItem[] = (() => {
    switch (timelineStatus) {
      case "ready":
        return [];
      case "running":
        return [
          { id: "rs-1", index: 1, title: "解析任务", status: "done" },
          {
            id: "rs-2",
            index: 2,
            title: "检索上下文",
            description: "加载项目文件...",
            status: "active",
          },
          { id: "rs-3", index: 3, title: "生成计划", status: "pending" },
          { id: "rs-4", index: 4, title: "执行计划", status: "pending" },
        ];
      case "done":
        return [
          { id: "rs-1", index: 1, title: "解析任务", status: "done" },
          { id: "rs-2", index: 2, title: "检索上下文", status: "done" },
          { id: "rs-3", index: 3, title: "生成计划", status: "done" },
          { id: "rs-4", index: 4, title: "执行计划", status: "done" },
        ];
      case "error":
        return [
          { id: "rs-1", index: 1, title: "解析任务", status: "done" },
          {
            id: "rs-2",
            index: 2,
            title: "检索上下文",
            description: "网络超时",
            status: "error",
          },
          { id: "rs-3", index: 3, title: "生成计划", status: "pending" },
          { id: "rs-4", index: 4, title: "执行计划", status: "pending" },
        ];
    }
  })();

  const statusButton = (status: typeof timelineStatus, label: string) => {
    const active = timelineStatus === status;
    return (
      <button
        key={status}
        style={{
          ...phase4BtnStyle,
          background: active ? "var(--aw-button-black)" : phase4BtnStyle.background,
          color: active ? "var(--aw-text-onblack)" : phase4BtnStyle.color,
          borderColor: active ? "var(--aw-button-black)" : phase4BtnStyle.borderColor,
        }}
        onClick={() => setTimelineStatus(status)}
      >
        {label}
      </button>
    );
  };

  const artifactButton = (set: typeof artifactSet, label: string) => {
    const active = artifactSet === set;
    return (
      <button
        key={set}
        style={{
          ...phase4BtnStyle,
          background: active ? "var(--aw-button-black)" : phase4BtnStyle.background,
          color: active ? "var(--aw-text-onblack)" : phase4BtnStyle.color,
          borderColor: active ? "var(--aw-button-black)" : phase4BtnStyle.borderColor,
        }}
        onClick={() => setArtifactSet(set)}
      >
        {label}
      </button>
    );
  };

  return (
    <div className="zoom-stage" style={{ justifyContent: "flex-start", padding: "48px 24px" }}>
      <div
        style={{
          width: "min(900px, calc(100vw - 80px))",
          display: "grid",
          gap: 24,
          padding: 24,
          border: "1px solid var(--aw-border-main)",
          borderRadius: "var(--aw-radius-lg)",
          background: "var(--aw-bg-main)",
          color: "var(--aw-text-primary)",
        }}
      >
        <h2
          style={{
            margin: 0,
            fontSize: "var(--aw-font-size-xl)",
            lineHeight: "var(--aw-line-height-xl)",
          }}
        >
          Phase 4 反馈组件
        </h2>

        <section style={phase4CardStyle}>
          <h3 style={phase4H3Style}>RunTimeline 状态切换</h3>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
            {statusButton("ready", "就绪")}
            {statusButton("running", "运行中")}
            {statusButton("done", "已完成")}
            {statusButton("error", "出错")}
          </div>
          <RunTimeline title="Agent 运行" status={timelineStatus} steps={timelineSteps}>
            {timelineStatus !== "ready" ? (
              <ArtifactGrid items={phase4ArtifactSets[artifactSet]} />
            ) : null}
          </RunTimeline>
        </section>

        <section style={phase4CardStyle}>
          <h3 style={phase4H3Style}>RunStep 状态矩阵</h3>
          <div style={{ display: "grid", gap: 4 }}>
            <RunStep
              index={1}
              title="待处理步骤"
              description="等待前置步骤完成"
              tag="pending"
              status="pending"
            />
            <RunStep
              index={2}
              title="进行中步骤"
              description="正在处理数据"
              tag="active"
              status="active"
            />
            <RunStep
              index={3}
              title="已完成步骤"
              description="成功返回结果"
              tag="done"
              status="done"
            />
            <RunStep
              index={4}
              title="出错步骤"
              description="发生超时"
              tag="error"
              status="error"
            />
          </div>
        </section>

        <section style={phase4CardStyle}>
          <h3 style={phase4H3Style}>ArtifactGrid 状态切换</h3>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
            {artifactButton("normal", "正常")}
            {artifactButton("empty", "Empty")}
            {artifactButton("long", "长文本")}
          </div>
          <ArtifactGrid items={phase4ArtifactSets[artifactSet]} />
        </section>

        <section style={phase4CardStyle}>
          <h3 style={phase4H3Style}>ApprovalCard riskTone / callback</h3>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
            {(["safe", "warning", "danger"] as const).map((tone) => (
              <button
                key={tone}
                style={phase4BtnStyle}
                onClick={() => {
                  setApprovalTone(tone);
                  setApprovalOpen(true);
                }}
              >
                {phase4ToneLabels[tone]}
              </button>
            ))}
          </div>
          <div
            style={{
              fontSize: "var(--aw-font-size-sm)",
              color: "var(--aw-text-secondary)",
            }}
          >
            Smoke 状态:{" "}
            <strong style={{ color: "var(--aw-text-primary)" }}>
              {approvalSmoke}
            </strong>
          </div>
          <ApprovalCard
            open={approvalOpen}
            onOpenChange={setApprovalOpen}
            title={phase4ApprovalTitles[approvalTone]}
            description={phase4ApprovalDescs[approvalTone]}
            riskTone={approvalTone}
            onApprove={() => setApprovalSmoke(`approve: ${approvalTone}`)}
            onReject={() => setApprovalSmoke(`reject: ${approvalTone}`)}
          />
        </section>

        <section style={phase4CardStyle}>
          <h3 style={phase4H3Style}>Toast 自动关闭 / 手动触发</h3>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button
              style={phase4BtnStyle}
              onClick={() => {
                setToastMsg("Toast 已显示");
                setToastVisible(true);
              }}
            >
              显示 Toast
            </button>
            <button
              style={phase4BtnStyle}
              onClick={() => setToastVisible(false)}
            >
              手动关闭
            </button>
          </div>
          <Toast
            message={toastMsg}
            visible={toastVisible}
            onClose={() => setToastVisible(false)}
          />
        </section>
      </div>
    </div>
  );
}

function Phase5Preview() {
  const [platform, setPlatform] = useState<PlatformKind>("macos");
  const [windowEvent, setWindowEvent] = useState("无");

  return (
    <PlatformProvider platform={platform}>
      <Phase5Demo
        platform={platform}
        onPlatformChange={setPlatform}
        onWindowEvent={setWindowEvent}
        windowEvent={windowEvent}
      />
    </PlatformProvider>
  );
}

function Phase5Demo({
  platform,
  onPlatformChange,
  onWindowEvent,
  windowEvent,
}: {
  platform: PlatformKind;
  onPlatformChange: (p: PlatformKind) => void;
  onWindowEvent: (e: string) => void;
  windowEvent: string;
}) {
  const { isMac, isWindows, modifierKey } = usePlatform();

  const platformButton = (p: PlatformKind, label: string) => {
    const active = platform === p;
    return (
      <button
        key={p}
        style={{
          ...phase4BtnStyle,
          background: active
            ? "var(--aw-button-black)"
            : phase4BtnStyle.background,
          color: active ? "var(--aw-text-onblack)" : phase4BtnStyle.color,
          borderColor: active
            ? "var(--aw-button-black)"
            : phase4BtnStyle.borderColor,
        }}
        onClick={() => onPlatformChange(p)}
      >
        {label}
      </button>
    );
  };

  return (
    <div
      style={{
        height: "100vh",
        display: "grid",
        gridTemplateRows: "auto 1fr",
      }}
    >
      <WindowTitleBar
        platform={platform}
        onMinimize={() => onWindowEvent("minimize")}
        onMaximize={() => onWindowEvent("maximize")}
        onClose={() => onWindowEvent("close")}
      />
      <AppShell
        platform={platform}
        sidebar={
          <AwakenSidebar
            conversations={phase3Conversations}
            projects={phase3Projects}
            navItems={phase3NavItems}
            activeConversationId="conv-1"
            platform={platform}
            onSelectConversation={() => {}}
            onSelectNav={() => {}}
            onSelectProject={() => {}}
          />
        }
        main={
          <>
            <TopBar>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: "12px",
                  minWidth: 0,
                }}
              >
                <span
                  style={{
                    fontSize: "var(--aw-font-size-sm)",
                    color: "var(--aw-text-secondary)",
                  }}
                >
                  modifierKey:{" "}
                  <strong style={{ color: "var(--aw-text-primary)" }}>
                    {modifierKey}
                  </strong>
                </span>
                <span
                  style={{
                    fontSize: "var(--aw-font-size-xs)",
                    color: "var(--aw-text-tertiary)",
                    fontFamily: "var(--aw-font-mono)",
                  }}
                >
                  {isMac ? "macOS" : isWindows ? "Windows" : "other"}
                </span>
                {platformButton("macos", "macOS")}
                {platformButton("windows", "Windows")}
              </div>
            </TopBar>
            <CenterStage>
              <div
                style={{
                  display: "grid",
                  gap: "16px",
                  padding: "24px",
                  border: "1px solid var(--aw-border-main)",
                  borderRadius: "var(--aw-radius-lg)",
                  background: "var(--aw-bg-white)",
                  color: "var(--aw-text-primary)",
                }}
              >
                <h2
                  style={{
                    margin: 0,
                    fontSize: "var(--aw-font-size-xl)",
                    lineHeight: "var(--aw-line-height-xl)",
                  }}
                >
                  Phase 5 布局与平台适配
                </h2>
                <p
                  style={{
                    margin: 0,
                    fontSize: "var(--aw-font-size-sm)",
                    color: "var(--aw-text-secondary)",
                  }}
                >
                  当前 platform:{" "}
                  <strong style={{ color: "var(--aw-text-primary)" }}>
                    {platform}
                  </strong>
                  ；modifierKey:{" "}
                  <strong style={{ color: "var(--aw-text-primary)" }}>
                    {modifierKey}
                  </strong>
                </p>
                <p
                  style={{
                    margin: 0,
                    fontSize: "var(--aw-font-size-sm)",
                    color: "var(--aw-text-secondary)",
                  }}
                >
                  WindowTitleBar callback:{" "}
                  <strong style={{ color: "var(--aw-text-primary)" }}>
                    {windowEvent}
                  </strong>
                </p>
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  <Kbd platform={platform}>K</Kbd>
                  <Kbd platform={platform}>Enter</Kbd>
                </div>
              </div>
            </CenterStage>
          </>
        }
      />
    </div>
  );
}

function FullPage() {
  const [platform, setPlatform] = useState<PlatformKind>("macos");
  const [activeNavId, setActiveNavId] = useState("nav-workflow");
  const [activeConversationId, setActiveConversationId] = useState("conv-1");
  const [activeProjectId, setActiveProjectId] = useState("proj-product");
  const [expandedIds, setExpandedIds] = useState<string[]>(["proj-product"]);
  const [composerValue, setComposerValue] = useState("");
  const [mode, setMode] = useState<ComposerMode>("Act");
  const [selectedModel, setSelectedModel] = useState<ModelOption>(mockModels[0]);
  const [runStatus, setRunStatus] = useState<"ready" | "running" | "done" | "error">("ready");
  const [toastVisible, setToastVisible] = useState(false);

  const handleSubmit = () => {
    if (!composerValue.trim()) return;
    setToastVisible(true);
    setRunStatus("running");
    setTimeout(() => setRunStatus("done"), 2500);
  };

  const runSteps: RunStepItem[] = [
    { id: "s1", index: 1, title: "确认边界", description: "只更新独立 HTML demo，不侵入现有 React 前端", tag: "scope", status: "done" },
    { id: "s2", index: 2, title: "对齐参考", description: "300px 侧栏，768px 主轴，32px 工具按钮", tag: "design", status: runStatus === "running" ? "active" : runStatus === "done" ? "done" : "pending" },
    { id: "s3", index: 3, title: "生成产物", description: "纯前端 UI 原型，仅展示视觉与交互状态", tag: "html", status: runStatus === "done" ? "done" : "pending" },
  ];

  const artifacts: ArtifactItem[] = [
    { id: "a1", title: "HTML Demo", description: "single file prototype" },
    { id: "a2", title: "Design Tokens", description: "measured desktop spacing" },
    { id: "a3", title: "Electron Path", description: "renderer-ready shell" },
  ];

  return (
    <PlatformProvider platform={platform}>
      <div style={{ height: "100vh", display: "grid", gridTemplateRows: "auto 1fr" }}>
        <WindowTitleBar
          platform={platform}
          onMinimize={() => setToastVisible(false)}
          onMaximize={() => setToastVisible(false)}
          onClose={() => setToastVisible(false)}
        />
        <AppShell
          platform={platform}
          sidebar={
            <AwakenSidebar
              title="Awaken"
              navItems={phase3NavItems}
              activeNavId={activeNavId}
              conversations={phase3Conversations}
              activeConversationId={activeConversationId}
              projects={phase3Projects}
              activeProjectId={activeProjectId}
              expandedIds={expandedIds}
              onSelectNav={setActiveNavId}
              onSelectConversation={setActiveConversationId}
              onSelectProject={setActiveProjectId}
              onExpandedChange={setExpandedIds}
              profileName="Nolan"
              profileAvatarFallback="NG"
              platform={platform}
            />
          }
          main={
            <>
              <TopBar>
                <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                  <Pill>Pro</Pill>
                  <button
                    style={phase4BtnStyle}
                    onClick={() => setPlatform((p) => (p === "macos" ? "windows" : "macos"))}
                  >
                    切换平台
                  </button>
                </div>
              </TopBar>
              <CenterStage>
                <h1
                  style={{
                    margin: "0 0 28px",
                    textAlign: "center",
                    fontSize: "var(--aw-font-size-xl)",
                    lineHeight: "var(--aw-line-height-xl)",
                    fontWeight: 620,
                    color: "var(--aw-text-primary)",
                  }}
                >
                  今天要让 Awaken 完成什么？
                </h1>

                <Composer
                  value={composerValue}
                  onChange={setComposerValue}
                  onSubmit={handleSubmit}
                  mode={mode}
                  onModeChange={setMode}
                  model={selectedModel}
                  models={mockModels}
                  onModelChange={setSelectedModel}
                  isReady={composerValue.trim().length > 0}
                  placeholder="描述工作目标，例如：整理今天的客户会议纪要，提炼待办并生成跟进邮件"
                />

                <QuickActionGrid>
                  {quickActions.map((action) => (
                    <QuickActionChip
                      key={action.id}
                      icon={action.icon}
                      label={action.label}
                      prompt={action.prompt}
                      onClick={(prompt) => setComposerValue(prompt)}
                    />
                  ))}
                </QuickActionGrid>

                {runStatus !== "ready" ? (
                  <div style={{ marginTop: 38 }}>
                    <RunTimeline title="整理任务" status={runStatus} steps={runSteps}>
                      <ArtifactGrid items={artifacts} />
                    </RunTimeline>
                  </div>
                ) : null}
              </CenterStage>
              <Toast
                message="任务已发送"
                visible={toastVisible}
                onClose={() => setToastVisible(false)}
              />
            </>
          }
        />
      </div>
    </PlatformProvider>
  );
}

export default function App() {
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

      {previewMode === "full" ? <FullPage /> : null}

      {previewMode === "primitives" ? <PrimitivePreview /> : null}
      {previewMode === "phase2" ? <Phase2Preview /> : null}
      {previewMode === "phase3" ? <Phase3Preview /> : null}
      {previewMode === "phase4" ? <Phase4Preview /> : null}
      {previewMode === "phase5" ? <Phase5Preview /> : null}
    </div>
  );
}
