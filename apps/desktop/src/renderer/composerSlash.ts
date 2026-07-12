import { createElement, type ReactNode } from "react";
import { Wrench } from "lucide-react";
import type {
  ComposerToken,
  RuntimeSkill,
  SkillComposerTokenPayload,
} from "../shared/runtime";

export type SlashActionType =
  | "skill"
  | "file"
  | "workflow"
  | "prompt"
  | "connector";

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
};

export type SlashProviderContext = {
  workspaceRootForRun: string;
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

export function slashTriggerFromComposerValue(
  value: string,
): SlashTrigger | null {
  const match = value.match(TRAILING_SLASH_TRIGGER_PATTERN);
  if (!match || match.index === undefined) {
    return null;
  }
  const leading = match[1] ?? "";
  return {
    query: match[2] ?? "",
    start: match.index + leading.length,
    end: value.length,
  };
}

export function clearSlashTriggerFromComposerValue(value: string): string {
  const trigger = slashTriggerFromComposerValue(value);
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
        icon: createElement(Wrench, { size: 13, strokeWidth: 1.85 }),
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
  listSkills: (workspaceRootForRun: string) => Promise<RuntimeSkill[]>,
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
