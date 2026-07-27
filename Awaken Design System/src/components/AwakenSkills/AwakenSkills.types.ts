export type AwakenSkillsView = "awaken" | "project" | "global" | "agents";

export type AwakenSkillScope =
  | "awaken"
  | "project"
  | "global"
  | "agents"
  | "mcp"
  | "other";

export type AwakenSkillItem = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  scope: AwakenSkillScope;
  sourceLabel?: string;
  diagnostic?: string;
  previewable?: boolean;
  busy?: boolean;
  toggleDisabled?: boolean;
  projectId?: string;
  folderPath?: string;
};

export type AwakenSkillGroup = {
  id: string;
  label: string;
  path?: string;
  skills: AwakenSkillItem[];
};

export type AwakenSkillProject = {
  id: string;
  name: string;
  path?: string;
  folders?: Array<{
    path: string;
    label?: string;
    isPrimary?: boolean;
  }>;
  skills: AwakenSkillItem[];
};
