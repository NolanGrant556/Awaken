import type { ComponentProps } from "react";
import type { AwakenCenterStage } from "./AwakenCenterStage";

export type AwakenCenterStageState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenCenterStage>>;
};

export const AwakenCenterStageStates: AwakenCenterStageState[] = [
  {
    id: "welcome-composer",
    label: "居中起始区",
    props: {
      children: "今天想让 Awaken 帮你完成什么？",
      width: 768,
      topMargin: 96,
    } as unknown as Partial<ComponentProps<typeof AwakenCenterStage>>,
  },
  {
    id: "narrow-stage",
    label: "窄内容区",
    props: {
      children: "用于较窄的提示或空状态说明。",
      width: 560,
      topMargin: 64,
    } as unknown as Partial<ComponentProps<typeof AwakenCenterStage>>,
  },
  {
    id: "long-intro",
    label: "长起始文案",
    props: {
      children: "将核心任务入口限制在主工作区中心，父级负责放入 Composer、空状态卡片或下一步建议。",
      width: 720,
      topMargin: 120,
    } as unknown as Partial<ComponentProps<typeof AwakenCenterStage>>,
  },
];
