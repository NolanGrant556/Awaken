import "./AwakenSidebar.css";
import { AwakenIcon } from "../AwakenIcon";
import "./AwakenSidebar.demo.css";
import { useEffect, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenNewTaskButton } from "../AwakenNewTaskButton";
import { AwakenPrimaryNav } from "../AwakenPrimaryNav";
import { AwakenProjectFoldersEditor } from "../AwakenProjectFoldersEditor";
import { AwakenProjectTree } from "../AwakenProjectTree";
import { AwakenSelectableCard } from "../AwakenSelectableCard";
import { AwakenSidebarSectionAction } from "../AwakenSidebarSectionAction";
import { AwakenSidebarThreadSpinner } from "../AwakenSidebarThreadSpinner";
import { AwakenWorkbenchSidebarFrame } from "../AwakenWorkbenchSidebarFrame";
import { AwakenSidebar } from "./AwakenSidebar";
import { AwakenSidebarStates } from "./AwakenSidebar.states";
import { AwakenSidebarUpdateButton } from "./AwakenSidebarUpdateButton";

type SidebarDialogMode =
  | "create-project"
  | "edit-project"
  | "detach-project"
  | "delete-conversations"
  | "rename-thread"
  | null;

const DEMO_PROJECT_FOLDER_PATHS = [
  "/Users/awaken/Projects/awaken",
  "/Users/awaken/Projects/awaken-design-system",
];

export function AwakenSidebarDemo() {
  const desktopProps = AwakenSidebarStates[0]?.props ?? {};
  const updateStates = AwakenSidebarStates.filter((state) => state.id.startsWith("update-"));
  const navItems = desktopProps.navItems ?? [];
  const [projects, setProjects] = useState(desktopProps.projects ?? []);
  const [conversations, setConversations] = useState(desktopProps.conversations ?? []);
  const [activeNavId, setActiveNavId] = useState(desktopProps.activeNavId);
  const [activeProjectId, setActiveProjectId] = useState(desktopProps.activeProjectId);
  const [expandedIds, setExpandedIds] = useState<string[]>(desktopProps.expandedIds ?? []);
  const [conversationSectionExpanded, setConversationSectionExpanded] = useState(true);
  const [conversationBatchMode, setConversationBatchMode] = useState(false);
  const [selectedConversationIds, setSelectedConversationIds] = useState<string[]>([]);
  const [dialogMode, setDialogMode] = useState<SidebarDialogMode>(null);
  const [renameDraft, setRenameDraft] = useState("设计系统清理");
  const [projectNameDraft, setProjectNameDraft] = useState("");
  const [folderPaths, setFolderPaths] = useState<string[]>(DEMO_PROJECT_FOLDER_PATHS);
  const [primaryFolderPath, setPrimaryFolderPath] = useState(DEMO_PROJECT_FOLDER_PATHS[0]);
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [lastAction, setLastAction] = useState("当前展示 Awaken Desktop 正在使用的左侧栏组合。");
  const originalThemeRef = useRef<string | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", theme);
  }, [theme]);

  useEffect(() => () => {
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  const allProjectsExpanded = projects.length > 0 && projects.every((project) => expandedIds.includes(project.id));
  const projectSectionActions = (
    <span className="aw-sidebar-demo__section-actions">
      <AwakenSidebarSectionAction
        label={allProjectsExpanded ? "收起全部项目" : "展开全部项目"}
        onClick={() => {
          setExpandedIds(allProjectsExpanded ? [] : projects.map((project) => project.id));
          setLastAction(allProjectsExpanded ? "已收起全部项目。" : "已展开全部项目。");
        }}
      >
        {allProjectsExpanded ? (
          <AwakenIcon concept="collapse-all" size={14} />
        ) : (
          <AwakenIcon concept="expand-all" size={14} />
        )}
      </AwakenSidebarSectionAction>
      <AwakenSidebarSectionAction
        label="创建项目"
        onClick={() => setLastAction("已触发创建项目回调（Gallery 模拟）。")}
      >
        <AwakenIcon concept="project-add" size={14} />
      </AwakenSidebarSectionAction>
    </span>
  );
  const eligibleConversationIds = conversations
    .filter((conversation) => !conversation.isLoading)
    .map((conversation) => conversation.id);
  const selectedConversationIdSet = new Set(
    selectedConversationIds.filter((id) => eligibleConversationIds.includes(id)),
  );
  const allEligibleConversationsSelected =
    eligibleConversationIds.length > 0 &&
    selectedConversationIdSet.size === eligibleConversationIds.length;
  const presentedConversations = conversations.map((conversation) => conversationBatchMode
    ? {
        ...conversation,
        selection: {
          checked: selectedConversationIdSet.has(conversation.id),
          disabled: Boolean(conversation.isLoading),
          disabledReason: conversation.isLoading ? "正在运行，停止后可删除" : undefined,
          onChange: (checked: boolean) => setSelectedConversationIds((current) => checked
            ? current.includes(conversation.id) ? current : [...current, conversation.id]
            : current.filter((id) => id !== conversation.id)),
        },
      }
    : conversation);
  const conversationSectionActions = (
    <span className="aw-sidebar-demo__section-actions">
      {conversationBatchMode ? (
        <>
          <AwakenSidebarSectionAction
            label={allEligibleConversationsSelected ? "取消全选" : "全选可删除对话"}
            disabled={eligibleConversationIds.length === 0}
            onClick={() => setSelectedConversationIds(
              allEligibleConversationsSelected ? [] : eligibleConversationIds,
            )}
          >
            <AwakenIcon concept="select-all" size={14} />
          </AwakenSidebarSectionAction>
          <AwakenSidebarSectionAction
            label={selectedConversationIdSet.size > 0
              ? `删除 ${selectedConversationIdSet.size} 个对话`
              : "请选择要删除的对话"}
            className="aw-sidebar-demo__batch-delete-action"
            disabled={selectedConversationIdSet.size === 0}
            onClick={() => setDialogMode("delete-conversations")}
          >
            <AwakenIcon concept="delete" size={14} />
          </AwakenSidebarSectionAction>
          <AwakenSidebarSectionAction
            label="退出批量管理"
            onClick={() => {
              setConversationBatchMode(false);
              setSelectedConversationIds([]);
            }}
          >
            <AwakenIcon concept="close" size={14} />
          </AwakenSidebarSectionAction>
        </>
      ) : (
        <>
          <AwakenSidebarSectionAction
            label={conversationSectionExpanded ? "收起对话" : "展开对话"}
            onClick={() => {
              setConversationSectionExpanded((expanded) => !expanded);
              setLastAction(conversationSectionExpanded ? "已收起对话。" : "已展开对话。");
            }}
          >
            {conversationSectionExpanded ? (
              <AwakenIcon concept="collapse-all" size={14} />
            ) : (
              <AwakenIcon concept="expand-all" size={14} />
            )}
          </AwakenSidebarSectionAction>
          <AwakenSidebarSectionAction
            label="批量管理对话"
            disabled={conversations.length === 0}
            onClick={() => {
              setConversationSectionExpanded(true);
              setSelectedConversationIds([]);
              setConversationBatchMode(true);
            }}
          >
            <AwakenIcon concept="select-mode" size={14} />
          </AwakenSidebarSectionAction>
          <AwakenSidebarSectionAction
            label="新增对话"
            onClick={() => setLastAction("已触发新增对话回调（Gallery 模拟）。")}
          >
            <AwakenIcon concept="thread-new" size={14} />
          </AwakenSidebarSectionAction>
        </>
      )}
    </span>
  );

  function selectNav(id: string): void {
    setActiveNavId(id);
    const label = navItems.find((item) => item.id === id)?.label ?? id;
    setLastAction(`已切换到${label}。`);
  }

  function selectProject(id: string): void {
    setActiveProjectId(id);
    const topLevel = projects.find((item) => item.id === id);
    const child = projects.flatMap((item) => item.children ?? []).find((item) => item.id === id);
    const conversation = conversations.find((item) => item.id === id);
    setLastAction(`已选择${topLevel?.label ?? child?.label ?? conversation?.label ?? id}。`);
  }

  function reorderProjects(ids: string[]): void {
    const byId = new Map(projects.map((project) => [project.id, project]));
    setProjects(ids.flatMap((id) => {
      const project = byId.get(id);
      return project ? [project] : [];
    }));
    setLastAction("已调整项目顺序（Gallery 模拟）。");
  }

  function closeDialog(result?: string): void {
    setDialogMode(null);
    if (result) {
      setLastAction(result);
    }
  }

  function removeFolder(path: string): void {
    setFolderPaths((current) => {
      const next = current.filter((item) => item !== path);
      if (path === primaryFolderPath) {
        setPrimaryFolderPath(next[0] ?? "");
      }
      return next;
    });
    setLastAction(`已模拟把 ${path} 从项目移除（磁盘文件保留）。`);
  }

  // 创建项目时文件夹还没挂到项目上，没有废纸篓这一档；编辑项目才有。
  function renderFoldersEditor(withTrash: boolean) {
    return (
      <AwakenProjectFoldersEditor
        folders={folderPaths.map((path) => ({
          path,
          isPrimary: path === primaryFolderPath,
          onMakePrimary: (target: string) => {
            setPrimaryFolderPath(target);
            setLastAction(`已模拟把 ${target} 设为主文件夹。`);
          },
          onRemove: removeFolder,
          actions: withTrash
            ? [
                {
                  id: "trash",
                  label: "移到废纸篓",
                  tone: "danger" as const,
                  onSelect: (target: string) =>
                    setLastAction(`已模拟把 ${target} 移到废纸篓（Gallery 不触碰磁盘）。`),
                },
              ]
            : undefined,
        }))}
        emptyHint="可以先不选文件夹。项目会直接创建，之后随时能加。"
        onAddFolder={() => setLastAction("已触发选择文件夹（Gallery 模拟，不打开系统对话框）。")}
      />
    );
  }

  const dialogTitle = dialogMode === "create-project"
    ? "创建项目"
    : dialogMode === "edit-project"
      ? "编辑项目"
      : dialogMode === "detach-project"
        ? "从 Awaken 移除项目"
        : dialogMode === "delete-conversations"
          ? `删除 ${selectedConversationIdSet.size} 个对话？`
          : "重命名会话";

  const dialogFooter = dialogMode === "create-project" ? (
    <>
      <div className="aw-sidebar-demo__dialog-source">
        <AwakenSelectableCard
          compact
          selected
          icon={<AwakenIcon concept="theme-system" size={16} />}
          title="Local"
        />
      </div>
      <AwakenButton onClick={() => closeDialog()}>取消</AwakenButton>
      <AwakenButton variant="primary" onClick={() => closeDialog("已模拟创建本地项目。")}>创建项目</AwakenButton>
    </>
  ) : dialogMode === "edit-project" ? (
    <AwakenButton variant="primary" onClick={() => closeDialog("已关闭编辑项目弹窗。")}>完成</AwakenButton>
  ) : dialogMode === "detach-project" ? (
    <>
      <AwakenButton onClick={() => closeDialog()}>取消</AwakenButton>
      <AwakenButton variant="primary" onClick={() => closeDialog("已模拟从 Awaken 移除项目。")}>确认移除</AwakenButton>
    </>
  ) : dialogMode === "delete-conversations" ? (
    <>
      <AwakenButton onClick={() => closeDialog()}>取消</AwakenButton>
      <AwakenButton
        variant="primary"
        onClick={() => {
          setConversations((current) => current.filter((item) => !selectedConversationIdSet.has(item.id)));
          setConversationBatchMode(false);
          setSelectedConversationIds([]);
          closeDialog(`已模拟删除 ${selectedConversationIdSet.size} 个对话。`);
        }}
      >
        删除 {selectedConversationIdSet.size} 个对话
      </AwakenButton>
    </>
  ) : (
    <>
      <AwakenButton onClick={() => closeDialog()}>取消</AwakenButton>
      <AwakenButton
        variant="primary"
        disabled={!renameDraft.trim()}
        onClick={() => closeDialog(`已模拟把会话重命名为“${renameDraft.trim()}”。`)}
      >
        确认
      </AwakenButton>
    </>
  );

  return (
    <section className="ads-component-demo aw-sidebar-demo" aria-label="「左侧栏」组件库 AwakenSidebar 演示">
      <header className="ads-component-demo-header">
        <h2>「左侧栏」组件库 <code>AwakenSidebar + AwakenDialog</code></h2>
        <p>先查看 Awaken Desktop 当前完整组合，再操作项目菜单、对话批量选择、区块工具提示和统一确认弹窗。</p>
      </header>

      <div className="aw-sidebar-demo__controls">
        <p>完整组合与下方小组件共享导航、项目展开和选中状态。</p>
        <div className="aw-sidebar-demo__theme-toggle" role="group" aria-label="主题预览">
          <AwakenButton
            size="sm"
            variant={theme === "light" ? "default" : "ghost"}
            aria-pressed={theme === "light"}
            onClick={() => setTheme("light")}
          >
            浅色
          </AwakenButton>
          <AwakenButton
            size="sm"
            variant={theme === "dark" ? "default" : "ghost"}
            aria-pressed={theme === "dark"}
            onClick={() => setTheme("dark")}
          >
            暗色
          </AwakenButton>
        </div>
      </div>

      <section className="aw-sidebar-demo__composition" aria-labelledby="aw-sidebar-composition-title">
        <header className="aw-sidebar-demo__section-heading">
          <h3 id="aw-sidebar-composition-title">完整组合</h3>
          <p><code>AwakenWorkbenchSidebarFrame + AwakenSidebar</code>，配置与当前 Desktop 使用方式保持一致。</p>
        </header>
        <div className="aw-sidebar-demo__composition-canvas">
          <div className="aw-sidebar-demo__preview">
            <AwakenWorkbenchSidebarFrame>
              <AwakenSidebar
                {...desktopProps}
                projects={projects}
                conversations={presentedConversations}
                activeNavId={activeNavId}
                activeProjectId={activeProjectId}
                expandedIds={expandedIds}
                projectSectionActions={projectSectionActions}
                conversationSectionExpanded={conversationSectionExpanded}
                conversationSectionActions={conversationSectionActions}
                conversationSectionLabel={conversationBatchMode
                  ? `对话 · 已选 ${selectedConversationIdSet.size}`
                  : "对话"}
                onSelectNav={selectNav}
                onSelectProject={selectProject}
                onSelectConversation={selectProject}
                onReorderProjects={reorderProjects}
                onExpandedChange={setExpandedIds}
                onNewTask={() => setLastAction("已触发新建任务回调（Gallery 模拟）。")}
                onProfileClick={() => setLastAction("已触发 Nolan 个人入口回调（Gallery 模拟）。")}
              />
            </AwakenWorkbenchSidebarFrame>
          </div>
          <div className="aw-sidebar-demo__composition-note">
            <span>当前 Desktop 组合</span>
            <strong>项目与独立对话共用一个滚动区</strong>
            <p>保留新建任务、连接器/技能导航、项目工作区、独立对话、运行中线程指示、行内菜单和 Nolan 个人入口；顶部与账户区固定。</p>
            <output aria-live="polite">{lastAction}</output>
          </div>
        </div>
      </section>

      <section className="aw-sidebar-demo__parts" aria-labelledby="aw-sidebar-parts-title">
        <header className="aw-sidebar-demo__section-heading">
          <h3 id="aw-sidebar-parts-title">可独立组合的小组件</h3>
          <p>组件源码、Props 与 states 继续独立维护；真实组合统一在当前组件库入口中查看，不再为内部小组件建立重复的顶层 Demo。</p>
        </header>

        <div className="aw-sidebar-demo__part-list">
          <article className="aw-sidebar-demo__part">
            <header>
              <h4>新建任务按钮</h4>
              <code>AwakenNewTaskButton</code>
              <p>提供 Desktop 左上角的新建任务主入口。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width">
              <AwakenNewTaskButton
                onClick={() => setLastAction("已触发新建任务回调（Gallery 模拟）。")}
              >
                新建任务
              </AwakenNewTaskButton>
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>更新入口</h4>
              <code>AwakenSidebarUpdateButton</code>
              <p>位于账户入口上方的更新提示按钮：有新版本、下载中（未知/已知进度）、待重启与失败重试；点击只回传回调。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width">
              <div className="aw-sidebar-demo__update-list">
                {updateStates.map((state) =>
                  state.props.profileUpdateAction ? (
                    <AwakenSidebarUpdateButton
                      key={state.id}
                      action={{
                        ...state.props.profileUpdateAction,
                        onClick: () =>
                          setLastAction(`已触发更新入口回调（${state.label}，Gallery 模拟）。`),
                      }}
                    />
                  ) : null,
                )}
              </div>
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>主导航</h4>
              <code>AwakenPrimaryNav</code>
              <p>受控展示连接器与技能入口，并把选择交还给 consumer。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width">
              <AwakenPrimaryNav items={navItems} activeId={activeNavId} onSelect={selectNav} />
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>项目树与更多菜单</h4>
              <code>AwakenProjectTree + AwakenDropdown</code>
              <p>共享项目选中与展开状态；本观测区常显省略号，点击后查看 Desktop 正在使用的更多菜单。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width aw-sidebar-demo__part-canvas--menu-visible">
              <AwakenProjectTree
                items={projects}
                activeId={activeProjectId}
                expandedIds={expandedIds}
                onExpandedChange={setExpandedIds}
                onSelect={selectProject}
                onReorder={reorderProjects}
              />
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>线程运行指示</h4>
              <code>AwakenSidebarThreadSpinner</code>
              <p>Desktop 仅在对应线程存在 running 或 cancelling run 时显示 14px 方形点阵。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__spinner-row">
              <span>项目附件确认</span>
              <AwakenSidebarThreadSpinner label="线程运行中" />
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>项目树空状态</h4>
              <code>AwakenProjectTree</code>
              <p>区分尚未添加任何项目，以及项目已展开但暂无会话。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__empty-states">
              <section>
                <span>无项目</span>
                <AwakenProjectTree items={[]} />
              </section>
              <section>
                <span>空项目已展开</span>
                <AwakenProjectTree
                  items={[{ id: "empty-project", label: "02_人力资源部", kind: "project" }]}
                  activeId="empty-project"
                  expandedIds={["empty-project"]}
                  onSelect={selectProject}
                />
              </section>
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>侧栏区块动作</h4>
              <code>AwakenSidebarSectionAction + AwakenTooltip</code>
              <p>承载项目/对话展开收起、创建项目与新增对话等紧凑图标操作，并提供 Tooltip。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__standalone-actions">
              {projectSectionActions}
              {conversationSectionActions}
              <span>{lastAction}</span>
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>侧栏弹窗流程</h4>
              <code>AwakenDialog + AwakenIconButton + AwakenSelectableCard + AwakenProjectFoldersEditor</code>
              <p>集中观测 App 从左侧栏触发的创建项目、编辑项目、移除项目、重命名会话和批量删除确认；关闭图标按钮、本地来源选择卡片和文件夹编辑器均使用真实组合。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__dialog-actions">
              <AwakenButton onClick={() => setDialogMode("create-project")}>创建项目</AwakenButton>
              <AwakenButton onClick={() => setDialogMode("edit-project")}>编辑项目</AwakenButton>
              <AwakenButton onClick={() => setDialogMode("detach-project")}>移除项目</AwakenButton>
              <AwakenButton onClick={() => setDialogMode("rename-thread")}>重命名会话</AwakenButton>
            </div>
          </article>

        </div>
      </section>

      <AwakenDialog
        open={dialogMode !== null}
        title={dialogTitle}
        onOpenChange={(open) => {
          if (!open) {
            closeDialog();
          }
        }}
        footer={dialogFooter}
      >
        {dialogMode === "create-project" ? (
          <div className="aw-sidebar-demo__dialog-card">
            <label className="aw-sidebar-demo__rename-field">
              <span>项目名称</span>
              <input
                value={projectNameDraft}
                onChange={(event) => setProjectNameDraft(event.target.value)}
                placeholder="例如：季度复盘"
              />
            </label>
            {renderFoldersEditor(false)}
          </div>
        ) : dialogMode === "edit-project" ? (
          <div className="aw-sidebar-demo__dialog-card">{renderFoldersEditor(true)}</div>
        ) : dialogMode === "detach-project" ? (
          <p className="aw-sidebar-demo__dialog-copy">
            “Awaken”会从侧栏移除。3 条历史会话和磁盘文件都会保留。
          </p>
        ) : dialogMode === "delete-conversations" ? (
          <p className="aw-sidebar-demo__dialog-copy">
            将删除所选对话记录和 Awaken 管理的附件；对话产生的文件不会被删除。此操作无法撤销。
          </p>
        ) : dialogMode === "rename-thread" ? (
          <label className="aw-sidebar-demo__rename-field">
            <span>会话标题</span>
            <input
              value={renameDraft}
              onChange={(event) => setRenameDraft(event.target.value)}
              placeholder="请输入会话标题"
            />
          </label>
        ) : null}
      </AwakenDialog>
    </section>
  );
}
