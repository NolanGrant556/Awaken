import { createElement, useEffect, useMemo, useState } from "react";
import { AwakenPluginBrandIcon } from "@awaken/design-system";
import type { CliConnectorView, CliProviderId, ComposerToken, RuntimeSkill } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import { builtinConnectorCardId, builtinConnectorProvider, builtinConnectorProviders, cliConnectorProductCopy } from "./cliConnectorProduct";
import { skillSlashActionsFromSkills, type SlashAction } from "./composerSlash";

const entrySkills: Record<CliProviderId, string> = {
  "dingtalk-workspace": "dws",
  "lark-cli": "lark-shared",
};
const brands = { "dingtalk-workspace": "dingtalk", "lark-cli": "feishu" } as const;

export function isBuiltinConnectorReady(view: CliConnectorView): boolean {
  const state = view.readiness;
  return state.activation === "enabled" && state.install === "installed" &&
    state.executable === "available" && state.authentication === "authorized" && state.overall === "ready";
}

/** UI aliases bind the existing global entry Skill; they are not Plugin Store packages. */
export function builtinConnectorMentionActions(views: CliConnectorView[], skills: RuntimeSkill[]): SlashAction[] {
  return builtinConnectorProviders.flatMap(providerId => {
    if (!views.some(view => view.providerId === providerId && isBuiltinConnectorReady(view))) return [];
    const candidates = skills.filter(skill => skill.sourceOrigin === "agents" && !skill.projectPath &&
      skill.name === entrySkills[providerId] && Boolean(skill.rootDir && skill.skillFilePath));
    // Never bind a same-name project/plugin skill or guess between ambiguous identities.
    if (candidates.length !== 1) return [];
    const action = skillSlashActionsFromSkills(candidates)[0];
    if (!action?.composerToken) return [];
    const product = cliConnectorProductCopy(providerId)!;
    const id = builtinConnectorCardId(providerId);
    return [{
      ...action, id, type: "connector", label: product.displayName, sectionLabel: "内置",
      description: product.description, keywords: [product.displayName, entrySkills[providerId]],
      preview: { description: product.description },
      icon: createElement(AwakenPluginBrandIcon, { brand: brands[providerId], size: "menu" }),
      composerToken: { ...action.composerToken, id, label: product.displayName },
    }];
  });
}

export function builtinConnectorTokenBrand(token: Pick<ComposerToken, "id" | "type">): string | undefined {
  const providerId = builtinConnectorProvider(token.id);
  return providerId && token.type === "skill" ? brands[providerId] : undefined;
}

export function useBuiltinConnectorComposer(runtimeClient: RuntimeClient, desktopReady: boolean, catalogRevision: number) {
  const [views, setViews] = useState<CliConnectorView[]>([]);
  const [skills, setSkills] = useState<RuntimeSkill[]>([]);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [skillError, setSkillError] = useState<string | null>(null);
  useEffect(() => {
    if (!desktopReady) { setViews([]); setSkills([]); setConnectionError(null); setSkillError(null); return; }
    let disposed = false;
    const updatedProviders = new Set<CliProviderId>();
    let skillRequest = 0;
    setConnectionError(null);
    setSkillError(null);
    const loadSkills = async () => {
      const request = ++skillRequest;
      try {
        // No project scope: the managed entry skills live in the global Agents directory.
        const next = await runtimeClient.listSkills();
        if (!disposed && request === skillRequest) { setSkills(next); setSkillError(null); }
      } catch {
        if (!disposed && request === skillRequest) { setSkills([]); setSkillError("钉钉、飞书技能暂不可用，请稍后重试。"); }
      }
    };
    const unsubscribe = runtimeClient.subscribeCliConnectorState(({ connector }) => {
      if (disposed) return;
      updatedProviders.add(connector.providerId);
      setConnectionError(null);
      setViews(current => [...current.filter(view => view.providerId !== connector.providerId), connector]);
      if (isBuiltinConnectorReady(connector)) void loadSkills();
    });
    void runtimeClient.listCliConnectors().then(next => {
      if (!disposed) setViews(current => [
        ...next.filter(view => !updatedProviders.has(view.providerId)),
        ...current.filter(view => updatedProviders.has(view.providerId)),
      ]);
    }).catch(() => {
      if (!disposed && updatedProviders.size === 0) { setViews([]); setConnectionError("钉钉、飞书连接状态暂不可用，请稍后重试。"); }
    });
    void loadSkills();
    return () => { disposed = true; unsubscribe(); };
  }, [runtimeClient, desktopReady, catalogRevision]);
  const actions = useMemo(() => builtinConnectorMentionActions(views, skills), [views, skills]);
  return { actions, error: connectionError ?? skillError };
}
