import type {
  AwakenArchivedChatGroup,
  AwakenArchivedChatsSettingsProps,
} from "./AwakenArchivedChatsSettings";

export const AwakenArchivedChatsSettingsSampleGroups: AwakenArchivedChatGroup[] = [
  {
    id: "project-awaken",
    label: "Awaken",
    rows: [
      {
        threadId: "thread_release_notes",
        title: "0.4.0 发布说明整理",
        archivedAtLabel: "2026 年 8 月 21 日",
        archivedAtTitle: "2026-08-21 15:42:08",
      },
      {
        threadId: "thread_icon_audit",
        title: "图标库光学审计修复",
        archivedAtLabel: "2026 年 8 月 18 日",
        archivedAtTitle: "2026-08-18 10:03:55",
      },
    ],
  },
  {
    id: "project-operations",
    label: "经营管理部跨部门协作与资料归档项目",
    rows: [
      {
        threadId: "thread_quarterly_report",
        title: "季度经营分析报告数据核对与图表生成（含各地区分公司明细汇总与同比环比口径说明）",
        archivedAtLabel: "2026 年 8 月 12 日",
        archivedAtTitle: "2026-08-12 09:27:41",
      },
    ],
  },
  {
    id: "standalone-chats",
    label: "对话",
    rows: [
      {
        threadId: "thread_trip_plan",
        title: "十一假期出行计划",
        archivedAtLabel: "2026 年 8 月 9 日",
        archivedAtTitle: "2026-08-09 20:15:30",
      },
    ],
  },
];

export const AwakenArchivedChatsSettingsSampleProjectOptions = [
  { value: "all", label: "所有项目" },
  { value: "project-awaken", label: "Awaken" },
  { value: "project-operations", label: "经营管理部跨部门协作与资料归档项目" },
] as const;

const baseProps = {
  groups: AwakenArchivedChatsSettingsSampleGroups,
  query: "",
  projectFilterOptions: AwakenArchivedChatsSettingsSampleProjectOptions,
  projectFilterValue: "all",
} satisfies Partial<AwakenArchivedChatsSettingsProps>;

export type AwakenArchivedChatsSettingsState = {
  id: string;
  caption: string;
  props: Partial<AwakenArchivedChatsSettingsProps>;
};

export const AwakenArchivedChatsSettingsStates: AwakenArchivedChatsSettingsState[] = [
  {
    id: "grouped",
    caption: "多项目分组 + 独立「对话」组（含长标题截断）",
    props: { ...baseProps },
  },
  {
    id: "busy-row",
    caption: "取消归档/删除进行中（busy 行按钮禁用）",
    props: { ...baseProps, busyThreadIds: ["thread_release_notes"] },
  },
  {
    id: "unknown-project",
    caption: "未知项目组（ID 后 8 位）",
    props: {
      ...baseProps,
      groups: [
        {
          id: "unknown-a1b2c3d4",
          label: "未知项目（a1b2c3d4）",
          rows: [
            {
              threadId: "thread_orphaned",
              title: "旧项目里的会话",
              archivedAtLabel: "2026 年 7 月 30 日",
              archivedAtTitle: "2026-07-30 11:05:12",
            },
          ],
        },
      ],
    },
  },
  {
    id: "empty-library",
    caption: "空库",
    props: { ...baseProps, groups: [] },
  },
  {
    id: "no-results",
    caption: "搜索/筛选无结果（保留清除条件入口）",
    props: { ...baseProps, groups: [], query: "报表", filtersActive: true },
  },
  {
    id: "loading",
    caption: "加载中",
    props: { ...baseProps, groups: [], loading: true },
  },
  {
    id: "error",
    caption: "载入失败（可重试，不伪装成空库）",
    props: { ...baseProps, groups: [], errorMessage: "已归档的聊天载入失败，请检查运行时后重试。" },
  },
];
