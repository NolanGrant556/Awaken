import type { AwakenProjectSkillFolderProps } from "./AwakenProjectSkillFolder";

export type AwakenProjectSkillFolderState = {
  id: string;
  label: string;
  props: AwakenProjectSkillFolderProps;
  skills: string[];
};

const noop = () => {};

export const AwakenProjectSkillFolderStates: AwakenProjectSkillFolderState[] = [
  {
    id: "finance",
    label: "财务项目",
    props: {
      projectName: "01_财务部",
      skillCount: 4,
      onOpen: noop,
    },
    skills: ["月度结账复核", "费用异常核验", "经营快照", "发票合规检查"],
  },
  {
    id: "operations",
    label: "业务运营项目",
    props: {
      projectName: "03_业务运营部",
      skillCount: 7,
      onOpen: noop,
    },
    skills: ["订单履约跟踪", "经营数据核对", "异常工单归因", "运营周报"],
  },
  {
    id: "marketing",
    label: "市场项目",
    props: {
      projectName: "04_市场部",
      skillCount: 5,
      onOpen: noop,
    },
    skills: ["渠道投放诊断", "活动复盘", "市场简报", "素材检查"],
  },
  {
    id: "management",
    label: "经营管理项目",
    props: {
      projectName: "09_经营管理部",
      skillCount: 3,
      onOpen: noop,
    },
    skills: ["经营分析", "管理层周报", "关键指标巡检"],
  },
];
