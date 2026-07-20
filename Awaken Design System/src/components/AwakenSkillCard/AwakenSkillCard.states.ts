import type { AwakenSkillItem } from "../AwakenSkills/AwakenSkills.types";

export type AwakenSkillCardState = {
  id: string;
  label: string;
  skill: AwakenSkillItem;
};

export const AwakenSkillCardStates: AwakenSkillCardState[] = [
  {
    id: "enabled",
    label: "已启用，可预览",
    skill: {
      id: "card-enabled",
      name: "管理层周报",
      description: "将跨部门进展整理为管理层周报。",
      enabled: true,
      previewable: true,
      scope: "project",
      sourceLabel: "09_经营管理部",
    },
  },
  {
    id: "disabled",
    label: "已停用",
    skill: {
      id: "card-disabled",
      name: "经营快照",
      description: "输出经营数据的管理层摘要。",
      enabled: false,
      previewable: true,
      scope: "project",
      sourceLabel: "01_财务部",
    },
  },
  {
    id: "diagnostic",
    label: "诊断状态",
    skill: {
      id: "card-diagnostic",
      name: "渠道投放诊断",
      description: "分析投放效率并给出止血建议。",
      diagnostic: "依赖的渠道数据文件暂不可用",
      enabled: true,
      previewable: true,
      scope: "project",
      sourceLabel: "04_市场部",
    },
  },
];
