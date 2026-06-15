import {
  ArrowLeft,
  ArrowRight,
  Bot,
  CircleDot,
  Clock3,
  PanelLeft,
  PenLine,
  Puzzle,
  Search,
  Settings,
} from "lucide-react";
import type { ConversationItem, ProjectItem } from "../types";
import { ConversationList } from "./ConversationList";
import { IconButton } from "./UiPrimitives";
import { ProjectTree } from "./ProjectTree";

type SidebarProps = {
  conversations: ConversationItem[];
  projects: ProjectItem[];
  activeItemId?: string;
  onSelectItem?: (id: string) => void;
};

export function Sidebar({ conversations, projects, activeItemId, onSelectItem }: SidebarProps) {
  return (
    <div className="sidebar-shell">
      <div className="traffic-row">
        <span className="traffic-dot traffic-red" />
        <span className="traffic-dot traffic-yellow" />
        <span className="traffic-dot traffic-green" />
        <IconButton label="收起侧边栏" className="traffic-action">
          <PanelLeft size={17} />
        </IconButton>
        <IconButton label="返回" className="traffic-action">
          <ArrowLeft size={18} />
        </IconButton>
        <IconButton label="前进" className="traffic-action">
          <ArrowRight size={18} />
        </IconButton>
      </div>

      <nav className="primary-nav" aria-label="主导航">
        <button className="nav-row">
          <PenLine size={19} />
          <span>新对话</span>
        </button>
        <button className="nav-row">
          <Search size={20} />
          <span>搜索</span>
        </button>
        <button className="nav-row">
          <Puzzle size={20} />
          <span>插件</span>
        </button>
        <button className="nav-row">
          <Clock3 size={20} />
          <span>自动化</span>
        </button>
      </nav>

      <div className="sidebar-scroll">
        <ConversationList items={conversations} activeItemId={activeItemId} onSelectItem={onSelectItem} />
        <ProjectTree items={projects} activeItemId={activeItemId} onSelectItem={onSelectItem} />
      </div>

      <div className="sidebar-footer">
        <button className="nav-row footer-settings">
          <Settings size={20} />
          <span>设置</span>
        </button>
        <span className="local-badge" title="本地运行">
          <Bot size={15} />
          <CircleDot size={11} />
        </span>
      </div>
    </div>
  );
}
