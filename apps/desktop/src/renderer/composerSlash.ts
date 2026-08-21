import { createElement, type ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";
import type {
  ComputerUseComposerTokenPayload,
  ComposerToken,
  RuntimeSkill,
  SkillProjectRoots,
  SkillComposerTokenPayload,
} from "../shared/runtime";

export type SlashActionType =
  | "skill"
  | "file"
  | "workflow"
  | "prompt"
  | "connector"
  | "capability";

export type SlashAction = {
  id: string;
  type: SlashActionType | string;
  label: string;
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

const TRAILING_SLASH_TRIGGER_PATTERN = /(^|\s)\/([^\s]*)$/;
const TRAILING_MENTION_TRIGGER_PATTERN = /(^|\s)@([^\s]*)$/;

function actionTriggerFromComposerValue(
  value: string,
  marker: ComposerActionTrigger["marker"],
  pattern: RegExp,
): ComposerActionTrigger | null {
  const match = value.match(pattern);
  if (!match || match.index === undefined) {
    return null;
  }
  const leading = match[1] ?? "";
  return {
    marker,
    query: match[2] ?? "",
    start: match.index + leading.length,
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
      (skill) => skill.state === "loaded" && skill.userInvocable !== false,
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

export function computerUseMentionProvider(): SlashProvider {
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
    id: "computer-use",
    label: "Computer Use",
    getActions() {
      return [
        {
          id: token.id,
          type: "capability",
          label: "Computer Use",
          description: "引导模型观察并操作本机图形界面",
          keywords: ["computer", "computer use", "计算机", "电脑", "GUI", "桌面"],
          icon: createElement(AwakenIcon, { concept: "computer-use", size: 14 }),
          preview: {
            description:
              "选择 Computer Use 能力；提交时由运行时向模型提供引导，不在用户输入中显示提示词。",
          },
          payload,
          composerToken: token,
        },
      ];
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
