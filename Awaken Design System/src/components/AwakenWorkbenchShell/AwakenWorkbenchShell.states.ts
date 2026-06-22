import type { ComponentProps } from "react";
import type { AwakenWorkbenchShell } from "./AwakenWorkbenchShell";

export type AwakenWorkbenchShellState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchShell>>;
};

export const AwakenWorkbenchShellStates: AwakenWorkbenchShellState[] = [
  {
    id: "stage-with-inspector",
    label: "主舞台和 Inspector",
    props: {
      stage: "运行舞台：执行流和 Composer 由父级放入这里。",
      inspector: "Inspector：变更、终端和文件详情由父级切换。",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchShell>>,
  },
  {
    id: "stage-only",
    label: "仅主舞台",
    props: {
      stage: "运行舞台：当前没有选中任何 Inspector 内容。",
      inspector: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchShell>>,
  },
  {
    id: "long-content",
    label: "较长布局内容",
    props: {
      stage: "较长的主舞台内容：用于验证运行叙事、运行提示和底部 Composer 被放入 stage 时不会与 Inspector 职责混淆。",
      inspector: "较长的 Inspector 内容：用于承载所选工具输出、文件变更或审批详情。",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchShell>>,
  },
];
