import type {
  AwakenSkillGroup,
  AwakenSkillItem,
  AwakenSkillProject,
} from "./AwakenSkills.types";

export type AwakenSkillPreviewState = {
  frontmatterRaw: string;
  markdown: string;
};

function skill(
  id: string,
  name: string,
  description: string,
  scope: AwakenSkillItem["scope"],
  sourceLabel: string,
  enabled = true,
  projectId?: string,
  folderPath?: string,
): AwakenSkillItem {
  return {
    id,
    name,
    description,
    scope,
    sourceLabel,
    enabled,
    previewable: true,
    projectId,
    folderPath,
  };
}

export const AwakenSkillsAwakenStates: AwakenSkillItem[] = [
  skill("awaken-word", "Word 文档", "创建、修订并检查专业 Word 文档。", "awaken", "Awaken 内置"),
  skill("awaken-excel", "Excel 表格", "创建、分析并验证电子表格与公式。", "awaken", "Awaken 内置"),
  skill("awaken-ppt", "PowerPoint 演示文稿", "生成并编辑结构清晰的演示文稿。", "awaken", "Awaken 内置"),
  skill("awaken-pdf", "PDF 文档", "读取、生成和视觉检查 PDF 文件。", "awaken", "Awaken 内置"),
  skill("awaken-code-search", "Code Search", "在代码仓库中定位实现、依赖与调用链。", "awaken", "Awaken 内置"),
  skill("awaken-pdf-max", "PDF Max", "使用固定工具链生成高质量排版 PDF。", "awaken", "Awaken 内置", false),
];

export const AwakenSkillsProjectStates: AwakenSkillProject[] = [
  {
    id: "finance",
    name: "01_财务部",
    path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/01_财务部",
    skills: [
      skill("finance-close", "月度结账复核", "检查月结口径、异常科目与待办事项。", "project", "01_财务部", true, "finance"),
      skill("finance-expense", "费用异常核验", "定位费用波动并整理可复核证据。", "project", "01_财务部", true, "finance"),
      skill("finance-snapshot", "经营快照", "输出经营数据的管理层摘要。", "project", "01_财务部", false, "finance"),
      skill("finance-invoice", "发票合规检查", "检查发票字段、税率与业务一致性。", "project", "01_财务部", true, "finance"),
    ],
  },
  {
    id: "operations",
    name: "03_业务运营部",
    path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/03_业务运营部",
    skills: [
      skill("ops-orders", "订单履约跟踪", "追踪订单节点并识别履约风险。", "project", "03_业务运营部", true, "operations"),
      skill("ops-weekly", "运营周报", "将运营数据整理为结构化周报。", "project", "03_业务运营部", true, "operations"),
      skill("ops-ticket", "异常工单归因", "对异常工单进行聚类、归因与建议。", "project", "03_业务运营部", true, "operations"),
    ],
  },
  {
    id: "marketing",
    name: "04_市场部",
    path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/04_市场部",
    skills: [
      skill("market-channel", "渠道投放诊断", "分析投放效率并给出止血建议。", "project", "04_市场部", true, "marketing"),
      skill("market-review", "活动复盘", "整理活动结果、偏差与下一步动作。", "project", "04_市场部", true, "marketing"),
      skill("market-brief", "市场简报", "生成面向管理层的市场信息摘要。", "project", "04_市场部", false, "marketing"),
    ],
  },
  {
    id: "management",
    name: "Nolan 实业项目",
    path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/07_客户服务部",
    folders: [
      {
        path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/07_客户服务部",
        label: "07_客户服务部",
        isPrimary: true,
      },
      {
        path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/08_行政与法务部",
        label: "08_行政与法务部",
        isPrimary: false,
      },
      {
        path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/09_经营管理部",
        label: "09_经营管理部",
        isPrimary: false,
      },
    ],
    skills: [
      skill("management-analysis", "经营分析", "汇总关键指标并解释经营偏差。", "project", "07_客户服务部", true, "management", "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/07_客户服务部"),
      skill("management-weekly", "管理层周报", "将跨部门进展整理为管理层周报。", "project", "08_行政与法务部", true, "management", "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/08_行政与法务部"),
      skill("management-kpi", "关键指标巡检", "巡检核心指标并标注异常变化。", "project", "07_客户服务部", true, "management", "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/07_客户服务部"),
    ],
  },
];

export const AwakenSkillsGlobalStates: AwakenSkillGroup[] = [
  {
    id: "global-user",
    label: "用户安装",
    path: "~/.awaken/skills",
    skills: [
      skill("global-consulting", "咨询分析", "将复杂业务问题拆解成可执行建议。", "global", "用户安装"),
      skill("global-canvas", "Canvas Design", "创建适合交付的视觉画布与海报。", "global", "用户安装"),
      skill("global-doc", "文档协作", "与用户共同完成结构化文档。", "global", "用户安装", false),
    ],
  },
  {
    id: "global-mcp",
    label: "MCP 动态",
    skills: [
      skill("mcp-dingtalk", "钉钉 AI 表格", "读取并分析已连接的钉钉 AI 表格。", "mcp", "DingTalk MCP"),
      skill("mcp-feishu", "飞书云文档", "读取并更新已授权的飞书云文档。", "mcp", "Feishu MCP"),
    ],
  },
];

export const AwakenSkillsAgentStates: AwakenSkillGroup[] = [
  {
    id: "agents-user",
    label: "用户 Agents Skills",
    path: "~/.agents/skills",
    skills: [
      skill("agents-browser", "Browser", "控制浏览器并验证页面交互。", "agents", "Agents 兼容来源"),
      skill("agents-pdf", "PDF", "读取、创建并验证 PDF 文件。", "agents", "Agents 兼容来源"),
    ],
  },
  {
    id: "agents-project",
    label: "项目 Agents Skills",
    path: "<workspace>/.agents/skills",
    skills: [
      skill("agents-project-release", "项目发布检查", "执行当前项目约定的发布前检查。", "agents", "项目兼容来源"),
      skill("agents-project-review", "项目代码复核", "按项目规范执行变更复核。", "agents", "项目兼容来源", false),
    ],
  },
];

function previewState(item: AwakenSkillItem): AwakenSkillPreviewState {
  return {
    frontmatterRaw: `---\nname: ${item.id}\ndescription: ${item.description}\nsource: ${item.sourceLabel ?? item.scope}\n---`,
    markdown: `# ${item.name}\n\n${item.description}\n\n## 使用方式\n\n当任务与 **${item.name}** 的能力范围匹配时使用。组件库中的内容为只读预览，真实正文由 Awaken Desktop 按需加载。\n\n## 执行步骤\n\n1. 核对用户目标与当前工作区。\n2. 读取完成任务所需的最少上下文。\n3. 执行操作并保留可复核证据。\n4. 返回结果、限制与下一步。\n\n> 这是用于组件交互验收的模拟 SKILL.md，不会访问本地文件或运行时。\n\n\`\`\`text\nSkill: ${item.name}\nSource: ${item.sourceLabel ?? item.scope}\nState: read-only preview\n\`\`\``,
  };
}

export const AwakenSkillsAllStates: AwakenSkillItem[] = [
  ...AwakenSkillsAwakenStates,
  ...AwakenSkillsProjectStates.flatMap((project) => project.skills),
  ...AwakenSkillsGlobalStates.flatMap((group) => group.skills),
  ...AwakenSkillsAgentStates.flatMap((group) => group.skills),
];

export const AwakenSkillPreviewStates: Record<string, AwakenSkillPreviewState> =
  Object.fromEntries(
    AwakenSkillsAllStates.map((item) => [item.id, previewState(item)]),
  );
