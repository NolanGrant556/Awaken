import type { AwakenSkillProject } from "../AwakenSkills/AwakenSkills.types";

export const AwakenProjectSkillsDialogState: AwakenSkillProject = {
  id: "management-dialog",
  name: "09_经营管理部",
  path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/09_经营管理部",
  skills: [
    {
      id: "dialog-analysis",
      name: "经营分析",
      description: "汇总关键指标并解释经营偏差。",
      enabled: true,
      previewable: true,
      scope: "project",
      sourceLabel: "09_经营管理部",
    },
    {
      id: "dialog-weekly",
      name: "管理层周报",
      description: "将跨部门进展整理为管理层周报。",
      enabled: true,
      previewable: true,
      scope: "project",
      sourceLabel: "09_经营管理部",
    },
    {
      id: "dialog-kpi",
      name: "关键指标巡检",
      description: "巡检核心指标并标注异常变化。",
      enabled: false,
      previewable: true,
      scope: "project",
      sourceLabel: "09_经营管理部",
    },
  ],
};
