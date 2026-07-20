import "./AwakenSidebar.css";
import "./AwakenSidebar.demo.css";
import { useEffect, useRef, useState } from "react";
import { FolderPlus, ListFilter } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenNewTaskButton } from "../AwakenNewTaskButton";
import { AwakenPrimaryNav } from "../AwakenPrimaryNav";
import { AwakenProjectTree } from "../AwakenProjectTree";
import { AwakenSidebarSectionAction } from "../AwakenSidebarSectionAction";
import { AwakenSidebarThreadSpinner } from "../AwakenSidebarThreadSpinner";
import { AwakenWorkbenchSidebarFrame } from "../AwakenWorkbenchSidebarFrame";
import { AwakenSidebar } from "./AwakenSidebar";
import { AwakenSidebarStates } from "./AwakenSidebar.states";

export function AwakenSidebarDemo() {
  const desktopProps = AwakenSidebarStates[0]?.props ?? {};
  const navItems = desktopProps.navItems ?? [];
  const projects = desktopProps.projects ?? [];
  const [activeNavId, setActiveNavId] = useState(desktopProps.activeNavId);
  const [activeProjectId, setActiveProjectId] = useState(desktopProps.activeProjectId);
  const [expandedIds, setExpandedIds] = useState<string[]>(desktopProps.expandedIds ?? []);
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

  const projectSectionActions = (
    <span className="aw-sidebar-demo__section-actions">
      <AwakenSidebarSectionAction
        label="Filter projects"
        onClick={() => setLastAction("当前产品提示：Project filters are not available yet.")}
      >
        <ListFilter size={13} strokeWidth={1.85} />
      </AwakenSidebarSectionAction>
      <AwakenSidebarSectionAction
        label="Add folder"
        onClick={() => setLastAction("已触发 Add folder 回调（Gallery 模拟）。")}
      >
        <FolderPlus size={13} strokeWidth={1.85} />
      </AwakenSidebarSectionAction>
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
    setLastAction(`已选择${topLevel?.label ?? child?.label ?? id}。`);
  }

  return (
    <section className="ads-component-demo aw-sidebar-demo" aria-label="「左侧栏」组件库 AwakenSidebar 演示">
      <header className="ads-component-demo-header">
        <h2>「左侧栏」组件库 <code>AwakenSidebar</code></h2>
        <p>先查看 Awaken Desktop 当前完整组合，再逐个识别和操作其中可复用的小组件。</p>
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
                activeNavId={activeNavId}
                activeProjectId={activeProjectId}
                expandedIds={expandedIds}
                projectSectionActions={projectSectionActions}
                onSelectNav={selectNav}
                onSelectProject={selectProject}
                onExpandedChange={setExpandedIds}
                onNewTask={() => setLastAction("已触发 New Agent 回调（Gallery 模拟）。")}
                onProfileClick={() => setLastAction("已触发 Nolan 个人入口回调（Gallery 模拟）。")}
              />
            </AwakenWorkbenchSidebarFrame>
          </div>
          <div className="aw-sidebar-demo__composition-note">
            <span>当前 Desktop 组合</span>
            <strong>无品牌行、无最近对话区</strong>
            <p>保留 New Agent、连接器/技能导航、Projects 项目树、运行中线程指示、行内菜单和 Nolan 个人入口。</p>
            <output aria-live="polite">{lastAction}</output>
          </div>
        </div>
      </section>

      <section className="aw-sidebar-demo__parts" aria-labelledby="aw-sidebar-parts-title">
        <header className="aw-sidebar-demo__section-heading">
          <h3 id="aw-sidebar-parts-title">可独立组合的小组件</h3>
          <p>每个组件仍保留独立目录、Props、states 与 Demo，但统一在当前组件库入口中查看。</p>
        </header>

        <div className="aw-sidebar-demo__part-list">
          <article className="aw-sidebar-demo__part">
            <header>
              <h4>新建任务按钮</h4>
              <code>AwakenNewTaskButton</code>
              <p>提供 Desktop 左上角的 New Agent 主入口。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width">
              <AwakenNewTaskButton
                shortcut=""
                platform="macos"
                onClick={() => setLastAction("已触发 New Agent 回调（Gallery 模拟）。")}
              >
                New Agent
              </AwakenNewTaskButton>
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
              <h4>项目树</h4>
              <code>AwakenProjectTree</code>
              <p>共享项目选中与展开状态；悬停项目或线程可查看新建会话与更多菜单。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__part-canvas--sidebar-width">
              <AwakenProjectTree
                items={projects}
                activeId={activeProjectId}
                expandedIds={expandedIds}
                onExpandedChange={setExpandedIds}
                onSelect={selectProject}
              />
            </div>
          </article>

          <article className="aw-sidebar-demo__part">
            <header>
              <h4>线程运行指示</h4>
              <code>AwakenSidebarThreadSpinner</code>
              <p>Desktop 仅在对应线程存在 running 或 cancelling run 时显示。</p>
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
              <code>AwakenSidebarSectionAction</code>
              <p>承载筛选与添加文件夹等紧凑图标操作，并提供 Tooltip。</p>
            </header>
            <div className="aw-sidebar-demo__part-canvas aw-sidebar-demo__standalone-actions">
              {projectSectionActions}
              <span>{lastAction}</span>
            </div>
          </article>

        </div>
      </section>
    </section>
  );
}
