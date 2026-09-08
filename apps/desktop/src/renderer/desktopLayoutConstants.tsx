import { AwakenIcon } from "@awaken/design-system";

export const DEFAULT_SIDEBAR_WIDTH = 300;

export const MIN_SIDEBAR_WIDTH = 228;

export const MAX_SIDEBAR_WIDTH = 420;

export const LAYOUT_STEP = 12;

export const INSPECTOR_RESPONSIVE_COLLAPSE_QUERY = "(max-width: 1180px)";

export const INSPECTOR_FILE_ITEMS = [
  {
    id: "files",
    label: "文件",
    tone: "neutral" as const,
    icon: <AwakenIcon concept="project" size={16} />,
  },
  {
    id: "browser",
    label: "浏览器",
    tone: "neutral" as const,
    icon: <AwakenIcon concept="browser" size={16} />,
  },
];
