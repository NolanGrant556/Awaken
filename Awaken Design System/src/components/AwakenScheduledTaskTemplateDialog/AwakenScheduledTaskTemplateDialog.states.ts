import type { AwakenScheduledTaskTemplateCategory } from "./AwakenScheduledTaskTemplateDialog";

// Gallery 示例数据，仅演示两分类与卡片层级；真实模板目录由 Renderer 拥有。
export const AwakenScheduledTaskTemplateDialogCategories: AwakenScheduledTaskTemplateCategory[] = [
  {
    id: "cron",
    label: "循环任务",
    templates: [
      {
        templateId: "daily_tech_news_brief",
        name: "工作日科技新闻早报",
        description: "每个工作日早上整理值得关注的科技新闻，输出标题、进展与趋势判断。",
        frequencyText: "工作日 09:30",
      },
      {
        templateId: "pomodoro_break_reminder",
        name: "休息提醒",
        description: "工作时段每 25 分钟提醒起身活动、喝水、放松眼睛。",
        frequencyText: "工作日 09:00–17:00 每 25 分钟",
      },
    ],
  },
  {
    id: "calendar",
    label: "日程任务",
    templates: [
      {
        templateId: "once_text_birthday_reminder",
        name: "生日提醒",
        description: "在所选时区下一个 1 月 1 日早上提醒送上生日祝福，不重复。",
        frequencyText: "下一个 1 月 1 日 09:00，不重复",
      },
      {
        templateId: "repeat_until_agent_weekly_summary",
        name: "周会前工作总结",
        description: "每周五早上基于最近一周 memory 生成工作总结，持续八周。",
        frequencyText: "每周五 08:30，共 8 周",
      },
    ],
  },
];

export const AwakenScheduledTaskTemplateDialogState = {
  title: "从模板创建",
  description: "先选内置模板，再在编辑器里微调关键参数后保存",
  categories: AwakenScheduledTaskTemplateDialogCategories,
};
