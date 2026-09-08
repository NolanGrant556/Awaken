import { createElement, type ReactNode } from "react";
import {
  AwakenIcon,
  AwakenPluginBrandIcon,
  isAwakenPluginBrand,
} from "@awaken/design-system";
import type {
  ComputerUseComposerTokenPayload,
  ComposerToken,
  PluginAgentComposerTokenPayload,
  PluginCommandComposerTokenPayload,
  PluginComposerTokenPayload,
  PluginRuleComposerTokenPayload,
  RuntimePlugin,
  RuntimePluginComponent,
  RuntimeSkill,
  SkillProjectRoots,
  SkillComposerTokenPayload,
  WorkflowComposerTokenPayload,
} from "../shared/runtime";

export type SlashActionType =
  | "skill"
  | "plugin"
  | "file"
  | "workflow"
  | "prompt"
  | "connector"
  | "capability";

export type SlashAction = {
  id: string;
  type: SlashActionType | string;
  label: string;
  sectionLabel?: string;
  description?: string;
  keywords?: string[];
  icon?: ReactNode;
  disabled?: boolean;
  disabledReason?: string;
  preview?: {
    description?: string;
    path?: string;
  };
  payload: unknown;
  composerToken?: ComposerToken;
  composerText?: string;
};

export type SlashProviderContext = {
  workspaceRootForRun: SkillProjectRoots;
};

export type SlashProvider = {
  id: string;
  label: string;
  getActions: (
    context: SlashProviderContext,
  ) => SlashAction[] | Promise<SlashAction[]>;
};

export type SlashTrigger = {
  query: string;
  start: number;
  end: number;
};

export type ComposerActionTrigger = SlashTrigger & {
  marker: "/" | "@";
};

export type ComposerActionTriggerDraft = {
  value: string;
  tokens: Array<Pick<ComposerToken, "id" | "leadingText">>;
  leadingTexts: Record<string, string>;
  selection?: { anchor: number; focus: number };
};

export type ComposerTokenPlacement = "before" | "after";

export type ComposerTokenKeyboardDeleteState = {
  key: string;
  selectionStart: number;
  selectionEnd: number;
  valueLength: number;
  tokens: Pick<ComposerToken, "id">[];
  placements: Record<string, ComposerTokenPlacement | undefined>;
};

// Composer actions are caret-driven and require a semantic boundary: the
// marker must start the current text segment or be preceded by whitespace.
// This keeps ordinary Chinese, Latin, numeric, URL-like, and path-like text
// from opening the palette while still allowing users to move the caret back
// into an existing sentence, insert a space, and then type / or @.
const TRAILING_SLASH_TRIGGER_PATTERN = /\/([^\s/@]*)$/u;
const TRAILING_MENTION_TRIGGER_PATTERN = /@([^\s/@]*)$/u;

function actionTriggerFromComposerValue(
  value: string,
  marker: ComposerActionTrigger["marker"],
  pattern: RegExp,
): ComposerActionTrigger | null {
  const match = value.match(pattern);
  if (!match || match.index === undefined) {
    return null;
  }
  if (match.index > 0 && !/\s/u.test(value[match.index - 1] ?? "")) {
    return null;
  }
  return {
    marker,
    query: match[1] ?? "",
    start: match.index,
    end: value.length,
  };
}

export function slashTriggerFromComposerValue(
  value: string,
): ComposerActionTrigger | null {
  return actionTriggerFromComposerValue(value, "/", TRAILING_SLASH_TRIGGER_PATTERN);
}

export function mentionTriggerFromComposerValue(
  value: string,
): ComposerActionTrigger | null {
  return actionTriggerFromComposerValue(value, "@", TRAILING_MENTION_TRIGGER_PATTERN);
}

export function composerActionTriggerFromComposerValue(
  value: string,
): ComposerActionTrigger | null {
  return mentionTriggerFromComposerValue(value) ?? slashTriggerFromComposerValue(value);
}

function actionTriggerFromTextAtCaret(
  text: string,
  caretOffset: number,
  logicalTextStart: number,
): ComposerActionTrigger | null {
  const textBeforeCaret = text.slice(0, Math.max(0, Math.min(caretOffset, text.length)));
  const trigger =
    actionTriggerFromComposerValue(textBeforeCaret, "@", TRAILING_MENTION_TRIGGER_PATTERN) ??
    actionTriggerFromComposerValue(textBeforeCaret, "/", TRAILING_SLASH_TRIGGER_PATTERN);
  return trigger
    ? {
        ...trigger,
        start: logicalTextStart + trigger.start,
        end: logicalTextStart + trigger.end,
      }
    : null;
}

export function composerActionTriggerFromDraft({
  value,
  tokens,
  leadingTexts,
  selection,
}: ComposerActionTriggerDraft): ComposerActionTrigger | null {
  const draftLength = tokens.reduce(
    (length, token) => length + (token.leadingText ?? leadingTexts[token.id] ?? "").length + 1,
    value.length,
  );
  const anchor = selection?.anchor ?? draftLength;
  const focus = selection?.focus ?? draftLength;
  if (anchor !== focus) {
    return null;
  }

  let logicalOffset = 0;
  for (const token of tokens) {
    const leadingText = token.leadingText ?? leadingTexts[token.id] ?? "";
    const leadingEnd = logicalOffset + leadingText.length;
    if (focus >= logicalOffset && focus <= leadingEnd) {
      return actionTriggerFromTextAtCaret(leadingText, focus - logicalOffset, logicalOffset);
    }
    logicalOffset = leadingEnd + 1;
  }

  const tailEnd = logicalOffset + value.length;
  if (focus < logicalOffset || focus > tailEnd) {
    return null;
  }
  return actionTriggerFromTextAtCaret(value, focus - logicalOffset, logicalOffset);
}

export function clearSlashTriggerFromComposerValue(value: string): string {
  const trigger = slashTriggerFromComposerValue(value);
  if (!trigger) {
    return value;
  }
  return `${value.slice(0, trigger.start)}${value.slice(trigger.end)}`;
}

export function clearComposerActionTriggerFromComposerValue(value: string): string {
  const trigger = composerActionTriggerFromComposerValue(value);
  if (!trigger) {
    return value;
  }
  return `${value.slice(0, trigger.start)}${value.slice(trigger.end)}`;
}

export function composerTokenIdForKeyboardDelete({
  key,
  selectionStart,
  selectionEnd,
  valueLength: _valueLength,
  tokens,
  placements: _placements,
}: ComposerTokenKeyboardDeleteState): string | undefined {
  if (selectionStart !== selectionEnd) {
    return undefined;
  }

  if (key === "Backspace" && selectionStart === 0) {
    return tokens.length > 0 ? tokens[tokens.length - 1]?.id : undefined;
  }

  return undefined;
}

export function composerTokenPlacementForSlashTrigger(
  trigger: SlashTrigger | null,
): ComposerTokenPlacement {
  return trigger && trigger.start > 0 ? "after" : "before";
}

function skillComposerTokenFromAction(
  actionId: string,
  label: string,
  skill: RuntimeSkill,
): ComposerToken {
  const payload: SkillComposerTokenPayload = {
    skillId: skill.id,
    sourceOrigin: skill.sourceOrigin,
    skillFilePath: skill.skillFilePath ?? "",
    rootDir: skill.rootDir,
  };
  return {
    id: actionId,
    type: "skill",
    label,
    payload: payload as unknown as Record<string, unknown>,
  };
}

export function pluginComposerTokenFromAction(plugin: RuntimePlugin): ComposerToken {
  const payload: PluginComposerTokenPayload = {
    pluginName: plugin.name,
    pluginDigest: plugin.digest ?? "",
  };
  return {
    id: `plugin:${plugin.name}:${plugin.digest ?? ""}`,
    type: "plugin",
    label: plugin.displayName?.trim() || plugin.name,
    payload: payload as unknown as Record<string, unknown>,
  };
}

export function pluginSkillComposerTokenFromAction(
  component: RuntimePluginComponent,
): ComposerToken {
  const catalogId = component.skill?.catalogId?.trim();
  if (!catalogId) {
    throw new Error("PLUGIN_SKILL_ID_UNAVAILABLE: Core did not provide a catalog identity");
  }
  const payload: SkillComposerTokenPayload = {
    skillId: catalogId,
    sourceOrigin: "plugin",
    skillFilePath: "",
  };
  return {
    id: `skill:${catalogId}:plugin`,
    type: "skill",
    label:
      component.displayName ||
      component.declaredName ||
      component.derivedName ||
      component.logicalId,
    payload: payload as unknown as Record<string, unknown>,
  };
}

export function pluginRuleComposerTokenFromAction(
  plugin: RuntimePlugin,
  component: RuntimePluginComponent,
): ComposerToken {
  const payload: PluginRuleComposerTokenPayload = {
    pluginName: plugin.name,
    pluginDigest: plugin.digest ?? "",
    ruleLogicalId: component.logicalId,
  };
  return {
    id: `plugin-rule:${plugin.name}:${component.logicalId}:${plugin.digest ?? ""}`,
    type: "rule",
    label: component.displayName || component.declaredName || component.derivedName || component.logicalId,
    payload: payload as unknown as Record<string, unknown>,
  };
}

export function pluginCommandComposerTokenFromAction(
  plugin: RuntimePlugin,
  component: RuntimePluginComponent,
  commandArguments: string[],
): ComposerToken {
  const payload: PluginCommandComposerTokenPayload = {
    pluginName: plugin.name,
    pluginDigest: plugin.digest ?? "",
    commandLogicalId: component.logicalId,
    commandArguments: JSON.stringify(commandArguments),
  };
  return {
    id: `plugin-command:${plugin.name}:${component.logicalId}:${plugin.digest ?? ""}:${JSON.stringify(commandArguments)}`,
    type: "command",
    label: component.displayName || component.declaredName || component.derivedName || component.logicalId,
    payload: payload as unknown as Record<string, unknown>,
  };
}

export function pluginAgentComposerTokenFromAction(
  plugin: RuntimePlugin,
  component: RuntimePluginComponent,
): ComposerToken {
  const payload: PluginAgentComposerTokenPayload = {
    pluginName: plugin.name,
    pluginDigest: plugin.digest ?? "",
    agentLogicalId: component.logicalId,
  };
  return {
    id: `plugin-agent:${plugin.name}:${component.logicalId}:${plugin.digest ?? ""}`,
    type: "agent",
    label: component.displayName || component.declaredName || component.derivedName || component.logicalId,
    payload: payload as unknown as Record<string, unknown>,
  };
}

function cloneComposerToken(token: ComposerToken): ComposerToken {
  return {
    ...token,
    payload: token.payload ? { ...token.payload } : undefined,
  };
}

export function skillSlashActionsFromSkills(
  skills: RuntimeSkill[],
): SlashAction[] {
  return skills
    .filter(
      (skill) =>
        skill.sourceOrigin !== "plugin" &&
        skill.state === "loaded" &&
        skill.userInvocable !== false,
    )
    .map((skill) => {
      const label = skill.displayName || skill.name || skill.id;
      const description = skill.description || skill.whenToUse || "";
      const id = `skill:${skill.id}:${skill.sourceOrigin}`;
      return {
        id,
        type: "skill",
        label,
        description,
        keywords: [
          skill.id,
          skill.name,
          skill.displayName,
          skill.description,
          skill.whenToUse,
          skill.skillFilePath,
        ].filter((value): value is string => Boolean(value)),
        // 技能条目用技能自己的图形，不再借用工具图标。
        icon: createElement(AwakenIcon, { concept: "skill", size: 14 }),
        preview: {
          description: skill.whenToUse || skill.description,
          path: skill.skillFilePath,
        },
        payload: skill,
        composerToken: skillComposerTokenFromAction(id, label, skill),
      };
    });
}

export function skillSlashProvider(
  listSkills: (workspaceRootForRun: SkillProjectRoots) => Promise<RuntimeSkill[]>,
): SlashProvider {
  return {
    id: "skills",
    label: "Skills",
    async getActions({ workspaceRootForRun }) {
      const skills = await listSkills(workspaceRootForRun);
      return skillSlashActionsFromSkills(skills);
    },
  };
}

function computerUseMentionAction(): SlashAction {
  const payload: ComputerUseComposerTokenPayload = {
    capabilityId: "computer-use",
  };
  const token: ComposerToken = {
    id: "capability:computer-use",
    type: "capability",
    label: "Computer Use",
    payload: payload as unknown as Record<string, unknown>,
  };
  return {
    id: token.id,
    type: "capability",
    label: "Computer Use",
    sectionLabel: "内置",
    description: "引导模型观察并操作本机图形界面",
    keywords: ["computer", "computer use", "计算机", "电脑", "GUI", "桌面"],
    icon: createElement(AwakenIcon, { concept: "computer-use", size: 14 }),
    preview: {
      description:
        "选择 Computer Use 能力；提交时由运行时向模型提供引导，不在用户输入中显示提示词。",
    },
    payload,
    composerToken: token,
  };
}

function deepResearchMentionAction(): SlashAction {
  const payload: WorkflowComposerTokenPayload = {
    workflowId: "deep-research",
  };
  const token: ComposerToken = {
    id: "workflow:deep-research",
    type: "workflow",
    label: "深度研究",
    payload: payload as unknown as Record<string, unknown>,
  };
  return {
    id: token.id,
    type: "workflow",
    label: "深度研究",
    sectionLabel: "内置",
    description: "宿主编排的只读多源研究报告",
    keywords: [
      "deep-research",
      "deep research",
      "研究",
      "调研",
      "research",
    ],
    icon: createElement(AwakenIcon, { concept: "search", size: 14 }),
    preview: {
      description:
        "插入 @deep-research token 后输入研究问题。无问题不会启动。定时任务编辑器不提供该入口。",
    },
    payload,
    composerToken: token,
  };
}

export function builtinMentionActions(): SlashAction[] {
  return [deepResearchMentionAction(), computerUseMentionAction()];
}

export function computerUseMentionProvider(): SlashProvider {
  return {
    id: "computer-use",
    label: "Computer Use",
    getActions: () => [computerUseMentionAction()],
  };
}

export function deepResearchMentionProvider(): SlashProvider {
  return {
    id: "deep-research",
    label: "深度研究",
    getActions: () => [deepResearchMentionAction()],
  };
}

export function pluginMentionActionsFromPlugins(
  plugins: RuntimePlugin[],
): SlashAction[] {
  return plugins
    .filter((plugin) => {
      if (!plugin.enabled || !plugin.digest) return false;
      const usableCount = plugin.skillCount + plugin.serverCount;
      if (!plugin.builtin) return usableCount > 0;
      const declaredCount =
        (plugin.componentCounts?.skill ?? 0) +
        (plugin.componentCounts?.mcp_server ?? plugin.componentCounts?.mcp ?? 0);
      return usableCount > 0 || declaredCount > 0;
    })
    .toSorted((left, right) => {
      if (Boolean(left.builtin) !== Boolean(right.builtin)) {
        return left.builtin ? -1 : 1;
      }
      if (left.builtin && right.builtin) {
        const order = (left.builtinOrder ?? 0) - (right.builtinOrder ?? 0);
        if (order !== 0) return order;
        return (left.displayName || left.name).localeCompare(
          right.displayName || right.name,
          "zh-CN",
        );
      }
      return 0;
    })
    .map((plugin) => {
      const token = pluginComposerTokenFromAction(plugin);
      const label = plugin.displayName?.trim() || plugin.name;
      const usable = plugin.skillCount + plugin.serverCount > 0;
      const description =
        (!usable && plugin.builtin
          ? "请先在插件详情完成账号连接或必填配置。"
          : plugin.description) ||
        `${plugin.skillCount} Skills · ${plugin.serverCount} MCP Servers`;
      return {
        id: token.id,
        type: "plugin",
        label,
        sectionLabel: plugin.builtin ? "内置" : "插件",
        description,
        keywords: [
          plugin.name,
          plugin.displayName,
          plugin.version,
          plugin.description,
          ...(plugin.keywords ?? []),
        ].filter((value): value is string => Boolean(value)),
        icon: isAwakenPluginBrand(plugin.brandIcon)
          ? createElement(AwakenPluginBrandIcon, {
              brand: plugin.brandIcon,
              size: "menu",
            })
          : createElement(AwakenIcon, { concept: "plugin-default", size: 14 }),
        preview: { description },
        disabled: !usable,
        disabledReason: !usable ? "请先完成插件设置" : undefined,
        payload: plugin,
        composerToken: token,
      };
    });
}

export function pluginMentionProvider(
  listPlugins: () => Promise<RuntimePlugin[]>,
): SlashProvider {
  return {
    id: "plugins",
    label: "Plugins",
    async getActions() {
      const plugins = await listPlugins();
      return pluginMentionActionsFromPlugins(plugins);
    },
  };
}

export async function actionsFromSlashProviders(
  providers: SlashProvider[],
  context: SlashProviderContext,
): Promise<SlashAction[]> {
  const providerActions = await Promise.all(
    providers.map((provider) => Promise.resolve(provider.getActions(context))),
  );
  return providerActions.flat();
}

export function filterSlashActions(
  actions: SlashAction[],
  query: string,
): SlashAction[] {
  const normalized = query.trim().toLowerCase();
  if (!normalized) {
    return actions;
  }
  return actions.filter((action) => {
    return [
      action.label,
      action.description,
      action.preview?.description,
      action.preview?.path,
      ...(action.keywords ?? []),
    ]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(normalized));
  });
}

// Keep the complete filtered catalog. Rendering windows, scrolling and future
// virtualization must never become capability-discovery limits.
export function composerPaletteActions(
  actions: SlashAction[],
  query: string,
): SlashAction[] {
  return filterSlashActions(actions, query);
}

export function activeComposerPaletteActions(
  marker: ComposerActionTrigger["marker"] | undefined,
  slashActions: SlashAction[],
  mentionActions: SlashAction[],
): SlashAction[] {
  return marker === "@" ? mentionActions : slashActions;
}

export function firstSelectableSlashAction(
  actions: SlashAction[],
): SlashAction | undefined {
  return actions.find((action) => !action.disabled) ?? actions[0];
}

export function composerTokenFromSlashAction(
  action: SlashAction,
): ComposerToken | null {
  if (action.composerToken) {
    return cloneComposerToken(action.composerToken);
  }
  if (action.type !== "skill") {
    return null;
  }
  const skill = action.payload as RuntimeSkill;
  return skillComposerTokenFromAction(action.id, action.label, skill);
}
