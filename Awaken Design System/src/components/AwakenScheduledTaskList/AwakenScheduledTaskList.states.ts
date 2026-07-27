import type {
  AwakenScheduledTaskListItem,
  AwakenScheduledTaskListProps,
} from "./AwakenScheduledTaskList";

export const AwakenScheduledTaskListDemoItems: AwakenScheduledTaskListItem[] = [
  {
    taskId: "sched_tech_news",
    name: "工作日科技新闻早报",
    kindLabel: "Agent",
    projectName: "Awaken",
    scheduleText: "工作日 09:30 · Asia/Shanghai",
    enabled: true,
    state: "running",
    stateLabel: "运行中",
    nextRunText: "下次：明天 09:30",
    lastRunText: "最近：今天 09:30 成功",
    running: true,
  },
  {
    taskId: "sched_pomodoro",
    name: "休息提醒",
    kindLabel: "提醒",
    projectName: "个人",
    scheduleText: "工作日 09:00–17:00 每 25 分钟 · Asia/Shanghai",
    enabled: true,
    state: "enabled",
    stateLabel: "已启用",
    nextRunText: "下次：今天 14:25",
    lastRunText: "最近：今天 14:00 成功",
    running: false,
  },
  {
    taskId: "sched_weekend_movies",
    name: "周末电影推荐",
    kindLabel: "Agent",
    projectName: "个人",
    scheduleText: "周六/周日 10:00 · Asia/Shanghai",
    enabled: false,
    state: "paused",
    stateLabel: "已暂停",
    nextRunText: null,
    lastRunText: "最近：上周六 10:00 成功",
    running: false,
  },
  {
    taskId: "sched_pet_care",
    name: "宠物驱虫/疫苗提醒",
    kindLabel: "提醒",
    projectName: "个人",
    scheduleText: "每月 15 日 20:00 · Asia/Shanghai",
    enabled: false,
    state: "completed",
    stateLabel: "已完成",
    nextRunText: null,
    lastRunText: "最近：7 月 15 日 20:00 成功",
    running: false,
  },
  {
    taskId: "sched_birthday",
    name: "生日提醒",
    kindLabel: "提醒",
    projectName: "个人",
    scheduleText: "1 月 1 日 09:00 · Asia/Shanghai",
    enabled: true,
    state: "invalid",
    stateLabel: "配置无效",
    nextRunText: null,
    lastRunText: "项目已移除，等待修正",
    running: false,
  },
  {
    taskId: "sched_medicine",
    name: "吃药提醒（14次）",
    kindLabel: "提醒",
    projectName: "个人",
    scheduleText: "每天 09:00，共 14 次 · Asia/Shanghai",
    enabled: true,
    state: "enabled",
    stateLabel: "已启用",
    nextRunText: "下次：明天 09:00",
    lastRunText: "最近：今天 09:00 失败",
    running: false,
  },
];

export type AwakenScheduledTaskListDemoState = {
  id: string;
  caption: string;
  props: Partial<AwakenScheduledTaskListProps>;
};

export const AwakenScheduledTaskListStates: AwakenScheduledTaskListDemoState[] = [
  {
    id: "empty",
    caption: "空状态（同时提供两种创建入口）",
    props: { items: [] },
  },
  {
    id: "loading",
    caption: "加载中",
    props: { items: [], loading: true },
  },
  {
    id: "populated",
    caption: "任务列表（运行中 / 已启用 / 已暂停 / 已完成 / 配置无效 / 上次失败）",
    props: { items: AwakenScheduledTaskListDemoItems },
  },
];
