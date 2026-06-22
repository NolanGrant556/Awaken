import type { ComponentProps } from "react";
import type { AwakenActivityLine } from "./AwakenActivityLine";

export type AwakenActivityLineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenActivityLine>>;
};

export const AwakenActivityLineStates: AwakenActivityLineState[] = [
  {
    id: "active",
    label: "进行中",
    props: {
      title: "正在读取项目结构",
      detail: "扫描工作区文件，整理当前任务可用的上下文。",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "已完成依赖检查",
      detail: "组件样式入口、导出文件和可观测状态均已确认。",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
  {
    id: "error",
    label: "出错",
    props: {
      title: "运行测试失败",
      detail: "需要查看终端输出，定位失败断言或构建错误。",
      status: "error",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
  {
    id: "long-content",
    label: "长活动说明",
    props: {
      title: "正在处理一条较长的活动说明，用于验证标题在窄容器里仍然保持可读并正确换行",
      detail: "适合用于执行时间线、工具调用摘要或任务进度列表中的单条记录。",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenActivityLine>>,
  },
];
