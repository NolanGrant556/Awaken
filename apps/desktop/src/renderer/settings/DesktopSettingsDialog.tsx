import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AwakenAppearanceSettings,
  AwakenConfiguredModels,
  AwakenModelSettings,
  AwakenModelProviderForm,
  type AwakenProviderBrand,
  type AwakenProviderConnectionStatus,
  AwakenSettings,
  AwakenSettingsNavItems,
  AwakenSettingsProviderCard,
  AwakenSettingsProviderModal,
} from "@awaken/design-system";
import { calculateAutoCompactThreshold } from "../../shared/runtime";
import type {
  AgentPlanAccount,
  AwakenTheme,
  AwakenThemePreference,
  ModelAccessType,
  ModelProfileSaveRequest,
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { providerBrandForId } from "../model-provider-brand";
import { PetsSettingsPanel } from "./PetsSettingsPanel";
import { SearchSettingsPanel } from "./SearchSettingsPanel";
import { MemorySettingsPanel } from "./MemorySettingsPanel";
import { ACPSettingsPanel } from "./ACPSettingsPanel";

export type DraftState = {
  profileId?: string;
  templateProfileId?: string;
  providerId: string;
  modelId: string;
  apiKey: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  enabled: boolean;
  isDefault: boolean;
  endpointId: string;
  advanced?: RuntimeModelProfileSummary["advanced"];
  requiresApiKey?: boolean;
};

type ModelSettingsSection = ModelAccessType;
type ModelSettingsView = "connections" | "llm";
type SettingsDialogSection = "models" | "search" | "memory" | "appearance" | "pets" | "acp";
type ModelConnectionTestFeedback = {
  tone: "success" | "error";
  message: string;
};

const MODEL_SETTINGS_TABS = [
  { id: "coding_plan", label: "Coding Plan" },
  { id: "api", label: "API" },
  { id: "agent_plan", label: "Agent Plan" },
];

const MODEL_SETTINGS_COPY: Record<
  ModelSettingsSection,
  { title: string; description: string; connectionLabel: string }
> = {
  coding_plan: {
    title: "Coding Plan",
    description: "管理套餐凭据与通过 Coding Plan 调用的模型。",
    connectionLabel: "配置 Token",
  },
  api: {
    title: "API",
    description: "管理供应商 API Key 与按量调用的模型。",
    connectionLabel: "配置 API Key",
  },
  agent_plan: {
    title: "Agent Plan",
    description: "管理 Agent Plan 账户以及套餐主模型。",
    connectionLabel: "配置账户",
  },
};

const SETTINGS_SECTION_TITLES: Record<SettingsDialogSection, string> = {
  models: "模型",
  search: "搜索",
  memory: "记忆",
  appearance: "外观",
  pets: "宠物",
  acp: "外部 Agent / ACP",
};

type ConfiguredProfileDraft = {
  enabled: boolean;
  contextWindow: number;
  contextWindowInput: string;
  maxTokens: number;
  maxTokensInput: string;
};

const CONTEXT_WINDOW_MIN = 8_000;
const CONTEXT_WINDOW_MAX = 1024 * 1024;
const CONTEXT_WINDOW_SLIDER_STEP = 1;
const CONTEXT_WINDOW_ANCHOR_RATIO = 0.6;
const CONTEXT_WINDOW_ANCHOR_SNAP_TOLERANCE = 10_000;
const MAX_TOKENS_DEFAULT = 8192;
const MAX_TOKENS_MIN = 256;
const MAX_TOKENS_MAX = 1_000_000;
const DUPLICATE_MODEL_PROFILE_ERROR = "模型配置重复，添加失败";

function providerBrand(provider: ModelProvider): AwakenProviderBrand | undefined {
  return providerBrandForId(provider.id);
}

function providerConnectionStatus(
  providerProfiles: RuntimeModelProfileSummary[],
  hasAgentPlanKey = false,
): AwakenProviderConnectionStatus {
  if (providerProfiles.some((profile) => profile.profileStatus === "ready")) {
    return "connected";
  }
  if (providerProfiles.length > 0 || hasAgentPlanKey) {
    return "attention";
  }
  return "unconfigured";
}

function credentialStatusLabel(
  status: RuntimeModelProfileSummary["profileStatus"],
): string | null {
  switch (status) {
    case "unreadable":
      return "无法读取 ~/.awaken/.env，请重新输入 API key";
    case "missing_key":
      return "未配置 API key";
    default:
      return null;
  }
}

function credentialRecoveryMessage(
  profiles: RuntimeModelProfileSummary[],
): string | null {
  if (profiles.some((profile) => profile.profileStatus === "unreadable")) {
    return "无法读取 ~/.awaken/.env 中的模型密钥。请检查文件权限，或重新输入 API key；其他模型配置会保留。";
  }
  return null;
}

type DesktopSettingsDialogProps = {
  open: boolean;
  runtimeClient: RuntimeClient;
  theme: AwakenTheme;
  themePreference: AwakenThemePreference;
  themeSaving: boolean;
  themeMessage: string | null;
  onThemePreferenceChange: (theme: AwakenThemePreference) => void;
  onOpenChange: (open: boolean) => void;
  onProfilesChanged: () => void;
  currentProjectId?: string;
  initialSection?: SettingsDialogSection;
  initialModelSection?: ModelSettingsSection;
};

type AppearanceSettingsProps = {
  themePreference: AwakenThemePreference;
  themeSaving: boolean;
  message?: string | null;
  onThemePreferenceChange: (theme: AwakenThemePreference) => void;
};

export function AppearanceSettings({
  themePreference,
  themeSaving,
  message,
  onThemePreferenceChange,
}: AppearanceSettingsProps) {
  return (
    <AwakenAppearanceSettings
      value={themePreference}
      disabled={themeSaving}
      message={message}
      onValueChange={onThemePreferenceChange}
    />
  );
}

function formatContextWindow(value: number): string {
  if (value >= 1_000_000) return `${value / 1_000_000}M`;
  return `${Math.round(value / 1_000)}K`;
}

function clampContextWindow(
  value: number | undefined,
  max: number = CONTEXT_WINDOW_MAX,
): number {
  if (!Number.isFinite(value)) return max;
  return Math.min(
    max,
    Math.max(CONTEXT_WINDOW_MIN, Math.round(value ?? max)),
  );
}

function modelCapabilityFor(provider: ModelProvider, modelId: string) {
  return provider.modelCapabilities[modelId];
}

function contextWindowMaxForProfile(
  profile: RuntimeModelProfileSummary,
  providers: ModelProvider[],
): number {
  const provider = providers.find((p) => p.id === profile.providerId);
  const capability = provider
    ? modelCapabilityFor(provider, profile.modelId)
    : undefined;
  return capability?.contextWindow ?? CONTEXT_WINDOW_MAX;
}

function contextWindowForProfile(
  profile: RuntimeModelProfileSummary,
  providers: ModelProvider[],
): number {
  const max = contextWindowMaxForProfile(profile, providers);
  return clampContextWindow(profile.advanced?.contextWindow, max);
}

function clampMaxTokens(value: number | undefined): number {
  if (!Number.isFinite(value)) return MAX_TOKENS_DEFAULT;
  return Math.min(
    MAX_TOKENS_MAX,
    Math.max(MAX_TOKENS_MIN, Math.round(value ?? MAX_TOKENS_DEFAULT)),
  );
}

function maxTokensForProfile(profile: RuntimeModelProfileSummary): number {
  return clampMaxTokens(profile.advanced?.maxTokens);
}

function providerHasEditableCapabilities(provider: ModelProvider): boolean {
  return provider.allowCustomModel || provider.endpointModel;
}

function advancedWithModelCapability(
  provider: ModelProvider,
  modelId: string,
  advanced: RuntimeModelProfileSummary["advanced"],
  overwrite: boolean,
): RuntimeModelProfileSummary["advanced"] {
  const capability =
    modelCapabilityFor(provider, modelId) ??
    (provider.allowCustomModel
      ? modelCapabilityFor(provider, provider.defaultModel)
      : undefined);
  if (!capability) return advanced;
  const inferredMaxTokens =
    capability.maxOutputTokens ?? capability.defaultMaxTokens;
  const shouldUseInferredMaxTokens =
    overwrite || advanced?.maxTokens === undefined;

  return {
    ...advanced,
    contextWindow:
      overwrite || advanced?.contextWindow === undefined
        ? capability.contextWindow
        : advanced.contextWindow,
    maxTokens: shouldUseInferredMaxTokens
      ? inferredMaxTokens
      : advanced.maxTokens,
    supportsVision:
      overwrite || advanced?.supportsVision === undefined
        ? capability.supportsVision
        : advanced.supportsVision,
    supportsNativePDF:
      overwrite || advanced?.supportsNativePDF === undefined
        ? capability.supportsNativePDF
        : advanced.supportsNativePDF,
    supportsVideo:
      overwrite || advanced?.supportsVideo === undefined
        ? capability.supportsVideo
        : advanced.supportsVideo,
    supportsTools: capability.supportsTools ?? advanced?.supportsTools,
    defaultReasoningEffort:
      overwrite || advanced?.defaultReasoningEffort === undefined
        ? capability.reasoningEffort?.defaultLevel
        : advanced.defaultReasoningEffort,
  };
}

function parseContextWindowInput(
  value: string,
  max: number = CONTEXT_WINDOW_MAX,
): number | undefined {
  if (value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? clampContextWindow(parsed, max) : undefined;
}

function parseMaxTokensInput(value: string): number | undefined {
  if (value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? clampMaxTokens(parsed) : undefined;
}

function contextWindowFill(value: number, max: number): number {
  return (
    ((clampContextWindow(value, max) - CONTEXT_WINDOW_MIN) /
      (max - CONTEXT_WINDOW_MIN)) *
    100
  );
}

function contextWindowAnchor(max: number): number {
  return clampContextWindow(
    Math.round(max * CONTEXT_WINDOW_ANCHOR_RATIO),
    max,
  );
}

function snapContextWindowAnchor(value: number, max: number): number {
  const anchor = contextWindowAnchor(max);
  return Math.abs(value - anchor) <= CONTEXT_WINDOW_ANCHOR_SNAP_TOLERANCE
    ? anchor
    : value;
}

function configuredDraftsFromProfiles(
  profiles: RuntimeModelProfileSummary[],
  providers: ModelProvider[],
): Record<string, ConfiguredProfileDraft> {
  return Object.fromEntries(
    profiles.map((profile) => [
      profile.id,
      {
        enabled: profile.enabled,
        contextWindow: contextWindowForProfile(profile, providers),
        contextWindowInput: contextWindowForProfile(profile, providers).toString(),
        maxTokens: maxTokensForProfile(profile),
        maxTokensInput: maxTokensForProfile(profile).toString(),
      },
    ]),
  );
}

function draftFromProvider(provider: ModelProvider): DraftState {
  const advanced = advancedWithModelCapability(
    provider,
    provider.defaultModel,
    undefined,
    true,
  );
  return {
    providerId: provider.id,
    modelId: provider.defaultModel,
    apiKey: "",
    baseURL: provider.baseURL,
    protocol: provider.protocol,
    authScheme: provider.authScheme,
    enabled: true,
    isDefault: true,
    endpointId: "",
    advanced,
  };
}

function draftFromTemplate(
  templateProfile: RuntimeModelProfileSummary | undefined,
  provider: ModelProvider,
  isDefault: boolean,
): DraftState {
  if (!templateProfile) {
    return {
      ...draftFromProvider(provider),
      isDefault,
    };
  }

  const modelId = templateProfile.modelId || provider.defaultModel;
  return {
    templateProfileId: templateProfile.id,
    providerId: templateProfile.providerId,
    modelId,
    apiKey: "",
    baseURL: templateProfile.baseURL || provider.baseURL,
    protocol: templateProfile.protocol || provider.protocol,
    authScheme: templateProfile.authScheme || provider.authScheme,
    enabled: true,
    isDefault,
    endpointId:
      templateProfile.advanced?.endpointId ??
      (provider.endpointModel ? templateProfile.modelId : ""),
    advanced: advancedWithModelCapability(
      provider,
      modelId,
      templateProfile.advanced,
      false,
    ),
  };
}

export function draftFromExistingProfile(
  profile: RuntimeModelProfileSummary,
  provider: ModelProvider,
): DraftState {
  const modelId = profile.modelId || provider.defaultModel;
  return {
    profileId: profile.id,
    providerId: profile.providerId,
    modelId,
    apiKey: "",
    baseURL: profile.baseURL || provider.baseURL,
    protocol: profile.protocol || provider.protocol,
    authScheme: profile.authScheme || provider.authScheme,
    enabled: profile.enabled,
    isDefault: profile.isDefault,
    endpointId:
      profile.advanced?.endpointId ??
      (provider.endpointModel ? profile.modelId : ""),
    advanced: advancedWithModelCapability(
      provider,
      modelId,
      profile.advanced,
      false,
    ),
    requiresApiKey:
      profile.profileStatus === "unreadable" ||
      profile.profileStatus === "missing_key",
  };
}

export function toSaveRequest(
  draft: DraftState,
  provider: ModelProvider,
): ModelProfileSaveRequest {
  const endpointId = draft.endpointId.trim();
  const modelId = draft.modelId;
  const advanced = advancedWithModelCapability(
    provider,
    modelId,
    {
      ...draft.advanced,
      endpointId: provider.endpointModel ? endpointId || undefined : undefined,
    },
    false,
  );
  return {
    id: draft.profileId,
    templateProfileId: draft.templateProfileId,
    providerId: provider.id,
    displayName: provider.displayName,
    modelId,
    baseURL: draft.baseURL,
    protocol: draft.protocol,
    authScheme: draft.authScheme,
    apiKey: draft.apiKey.trim() || undefined,
    enabled: draft.enabled,
    isDefault: draft.isDefault,
    advanced,
  };
}

export function DesktopSettingsDialog({
  open,
  runtimeClient,
  theme,
  themePreference,
  themeSaving,
  themeMessage,
  onThemePreferenceChange,
  onOpenChange,
  onProfilesChanged,
  currentProjectId,
  initialSection = "models",
  initialModelSection = "coding_plan",
}: DesktopSettingsDialogProps) {
  const [providers, setProviders] = useState<ModelProvider[]>([]);
  const [profiles, setProfiles] = useState<RuntimeModelProfileSummary[]>([]);
  const [agentPlanAccount, setAgentPlanAccount] = useState<AgentPlanAccount | null>(null);
  const [configuredDrafts, setConfiguredDrafts] = useState<
    Record<string, ConfiguredProfileDraft>
  >({});
  const [activeSection, setActiveSection] =
    useState<ModelSettingsSection>("coding_plan");
  const [activeModelView, setActiveModelView] =
    useState<ModelSettingsView>("connections");
  const [activeSettingsSection, setActiveSettingsSection] =
    useState<SettingsDialogSection>(initialSection);
  const [expandedProviderId, setExpandedProviderId] = useState<string | null>(
    null,
  );
  const [draft, setDraft] = useState<DraftState | null>(null);
  const [saving, setSaving] = useState(false);
  const [savingConfigured, setSavingConfigured] = useState(false);
  const [deletingProfileId, setDeletingProfileId] = useState<string | null>(
    null,
  );
  const [testing, setTesting] = useState(false);
  const [testFeedback, setTestFeedback] =
    useState<ModelConnectionTestFeedback | null>(null);
  const [testingReasoning, setTestingReasoning] = useState(false);
  const [refreshingAgentPlan, setRefreshingAgentPlan] = useState(false);
  const [deletingAgentPlan, setDeletingAgentPlan] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const toastTimerRef = useRef<number | null>(null);
  const settingsRootRef = useRef<HTMLElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const expandedProviderIdRef = useRef<string | null>(null);

  const providerById = useMemo(() => {
    return new Map(providers.map((provider) => [provider.id, provider]));
  }, [providers]);

  const profilesByProviderId = useMemo(() => {
    const groupedProfiles = new Map<string, RuntimeModelProfileSummary[]>();
    for (const profile of profiles) {
      const existingProfiles = groupedProfiles.get(profile.providerId) ?? [];
      existingProfiles.push(profile);
      groupedProfiles.set(profile.providerId, existingProfiles);
    }
    return groupedProfiles;
  }, [profiles]);

  const activeProviders = useMemo(
    () =>
      providers.filter(
        (provider) =>
          (provider.accessType ?? (provider.codingPreset ? "coding_plan" : "api")) ===
          activeSection,
      ),
    [activeSection, providers],
  );

  const activeProfiles = useMemo(
    () =>
      profiles.filter((profile) => {
        const provider = providerById.get(profile.providerId);
        return (
          (profile.accessType ??
            provider?.accessType ??
            (provider?.codingPreset ? "coding_plan" : "api")) === activeSection
        );
      }),
    [activeSection, profiles, providerById],
  );

  const loadModelSettings = useCallback(async () => {
    setMessage(null);
    try {
      const [nextProviders, nextProfiles, agentPlanAccountResult] = await Promise.all([
        runtimeClient.listModelProviders(),
        runtimeClient.listModelProfiles(),
        runtimeClient.getAgentPlanAccount().then(
          (value) => ({ value, error: null }),
          (error: unknown) => ({ value: null, error }),
        ),
      ]);
      setProviders(nextProviders);
      setProfiles(nextProfiles);
      setAgentPlanAccount(agentPlanAccountResult.value);
      const accountErrorMessage = agentPlanAccountResult.error
        ? agentPlanAccountResult.error instanceof Error
          ? agentPlanAccountResult.error.message
          : "无法读取 Agent Plan 账户配置"
        : null;
      setMessage(accountErrorMessage ?? credentialRecoveryMessage(nextProfiles));
      setConfiguredDrafts(
        configuredDraftsFromProfiles(nextProfiles, nextProviders),
      );

      try {
        const syncedProviders = await runtimeClient.syncModelCapabilities();
        const refreshedProfiles = await runtimeClient.listModelProfiles();
        setProviders(syncedProviders);
        setProfiles(refreshedProfiles);
        setMessage(accountErrorMessage ?? credentialRecoveryMessage(refreshedProfiles));
        setConfiguredDrafts(
          configuredDraftsFromProfiles(refreshedProfiles, syncedProviders),
        );
      } catch {
        // Background capability sync is best-effort; static values remain usable.
      }
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Failed to load model settings",
      );
    }
  }, [runtimeClient]);

  function showToast(message: string): void {
    if (toastTimerRef.current !== null) {
      window.clearTimeout(toastTimerRef.current);
    }
    setToastMessage(message);
    toastTimerRef.current = window.setTimeout(() => {
      setToastMessage(null);
      toastTimerRef.current = null;
    }, 2000);
  }

  function collapseProviderEditor(): void {
    setExpandedProviderId(null);
    setDraft(null);
    setShowApiKey(false);
    setTestFeedback(null);
  }

  useEffect(() => {
    expandedProviderIdRef.current = expandedProviderId;
  }, [expandedProviderId]);

  useEffect(() => {
    if (!open) {
      setExpandedProviderId(null);
      setDraft(null);
      setActiveSection(initialModelSection);
      setActiveModelView("connections");
      setActiveSettingsSection(initialSection);
      setShowApiKey(false);
      setTestFeedback(null);
      setDeletingProfileId(null);
      setToastMessage(null);
      if (toastTimerRef.current !== null) {
        window.clearTimeout(toastTimerRef.current);
        toastTimerRef.current = null;
      }
      return;
    }
    setActiveSection(initialModelSection);
    void loadModelSettings();
  }, [loadModelSettings, open, initialSection, initialModelSection]);

  useEffect(() => {
    if (!open) return;
    restoreFocusRef.current = document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
    const focusFrame = window.requestAnimationFrame(() => {
      settingsRootRef.current?.focus();
    });
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      if (expandedProviderIdRef.current !== null) {
        setExpandedProviderId(null);
        setDraft(null);
        setShowApiKey(false);
        expandedProviderIdRef.current = null;
        return;
      }
      onOpenChange(false);
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.cancelAnimationFrame(focusFrame);
      window.removeEventListener("keydown", handleKeyDown);
      restoreFocusRef.current?.focus();
      restoreFocusRef.current = null;
    };
  }, [onOpenChange, open]);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current !== null) {
        window.clearTimeout(toastTimerRef.current);
      }
    };
  }, []);

  function providerTemplateProfile(
    providerId: string,
  ): RuntimeModelProfileSummary | undefined {
    const providerProfiles = profilesByProviderId.get(providerId) ?? [];
    for (let index = providerProfiles.length - 1; index >= 0; index -= 1) {
      if (providerProfiles[index]?.hasKey) {
        return providerProfiles[index];
      }
    }
    return providerProfiles[providerProfiles.length - 1];
  }

  function createDraftForProvider(provider: ModelProvider): DraftState {
    return draftFromTemplate(
      providerTemplateProfile(provider.id),
      provider,
      profiles.length === 0,
    );
  }

  function selectProvider(provider: ModelProvider): void {
    if (expandedProviderId === provider.id) {
      collapseProviderEditor();
      setMessage(null);
      return;
    }

    setExpandedProviderId(provider.id);
    setDraft(createDraftForProvider(provider));
    setShowApiKey(false);
    setTestFeedback(null);
    setMessage(null);
  }

  function editExistingProfile(profile: RuntimeModelProfileSummary): void {
    const provider = providerById.get(profile.providerId);
    if (!provider) {
      setMessage("无法找到该模型供应商，请刷新后重试。");
      return;
    }
    setActiveSection(provider.accessType ?? (provider.codingPreset ? "coding_plan" : "api"));
    setActiveModelView("connections");
    setExpandedProviderId(provider.id);
    setDraft(draftFromExistingProfile(profile, provider));
    setShowApiKey(false);
    setTestFeedback(null);
    setMessage(
      profile.profileStatus === "unreadable"
        ? "请重新输入 API key。保存成功后会更新 ~/.awaken/.env 中该配置对应的变量。"
        : null,
    );
  }

  async function saveDraft(): Promise<void> {
    if (!draft) return;
    const provider = providerById.get(draft.providerId);
    if (!provider) return;
    if (draft.requiresApiKey && draft.apiKey.trim() === "") {
      setMessage("请先输入新的 API key。");
      return;
    }
    setSaving(true);
    setTestFeedback(null);
    setMessage(null);
    try {
      await runtimeClient.saveModelProfile(toSaveRequest(draft, provider));
      await loadModelSettings();
      collapseProviderEditor();
      onProfilesChanged();
      showToast("模型配置已保存");
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : "Failed to save model profile";
      if (errorMessage.includes(DUPLICATE_MODEL_PROFILE_ERROR)) {
        showToast(DUPLICATE_MODEL_PROFILE_ERROR);
        setMessage(null);
        return;
      }
      setMessage(errorMessage);
    } finally {
      setSaving(false);
    }
  }

  async function testDraft(): Promise<void> {
    if (!draft) return;
    const provider = providerById.get(draft.providerId);
    if (!provider) return;
    setTesting(true);
    setTestFeedback(null);
    setMessage(null);
    try {
      const result = await runtimeClient.testModelProfile(
        toSaveRequest(draft, provider),
      );
      setTestFeedback({
        tone: result.ok ? "success" : "error",
        message: result.message,
      });
    } catch (error) {
      setTestFeedback({
        tone: "error",
        message: error instanceof Error
          ? error.message
          : "连接测试失败：测试请求未完成。",
      });
    } finally {
      setTesting(false);
    }
  }

  async function testDraftReasoning(): Promise<void> {
    if (!draft) return;
    const provider = providerById.get(draft.providerId);
    if (!provider) return;
    setTestingReasoning(true);
    setMessage(null);
    try {
      const result = await runtimeClient.testAgentPlanReasoningCapability(
        toSaveRequest(draft, provider),
      );
      showToast(result.message);
      await loadModelSettings();
    } catch (error) {
      showToast(error instanceof Error ? error.message : "effort 验证失败");
    } finally {
      setTestingReasoning(false);
    }
  }

  async function refreshAgentPlanModels(): Promise<void> {
    setRefreshingAgentPlan(true);
    setMessage(null);
    try {
      const account = await runtimeClient.refreshAgentPlanModels();
      setAgentPlanAccount(account);
      await loadModelSettings();
      showToast(`Agent Plan 模型目录已刷新（${account.modelIds.length} 个）`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "刷新 Agent Plan 模型目录失败");
    } finally {
      setRefreshingAgentPlan(false);
    }
  }

  async function deleteAgentPlanAccount(): Promise<void> {
    if (!window.confirm("删除火山 Agent Plan 账户及其 API Key？")) return;
    setDeletingAgentPlan(true);
    setMessage(null);
    try {
      await runtimeClient.deleteAgentPlanAccount();
      await loadModelSettings();
      showToast("Agent Plan 账户已删除");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "删除 Agent Plan 账户失败");
    } finally {
      setDeletingAgentPlan(false);
    }
  }

  const configuredHasChanges = activeProfiles.some((profile) => {
    const next = configuredDrafts[profile.id];
    if (!next) return false;
    return (
      next.enabled !== profile.enabled ||
      next.contextWindow !== contextWindowForProfile(profile, providers) ||
      next.maxTokens !== maxTokensForProfile(profile)
    );
  });

  function updateConfiguredDraft(
    profileId: string,
    patch: Partial<ConfiguredProfileDraft>,
  ): void {
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      return {
        ...current,
        [profileId]: {
          ...existing,
          ...patch,
        },
      };
    });
    setMessage(null);
  }

  function resetConfiguredDrafts(): void {
    setConfiguredDrafts((current) => ({
      ...current,
      ...configuredDraftsFromProfiles(activeProfiles, providers),
    }));
    setMessage(null);
  }

  function configuredContextWindowMax(profileId: string): number {
    const profile = profiles.find((p) => p.id === profileId);
    return profile
      ? contextWindowMaxForProfile(profile, providers)
      : CONTEXT_WINDOW_MAX;
  }

  function updateConfiguredContextWindow(
    profileId: string,
    value: number,
  ): void {
    const nextValue = clampContextWindow(value, configuredContextWindowMax(profileId));
    updateConfiguredDraft(profileId, {
      contextWindow: nextValue,
      contextWindowInput: nextValue.toString(),
    });
  }

  function snapConfiguredContextWindow(profileId: string): void {
    const max = configuredContextWindowMax(profileId);
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      const nextValue = snapContextWindowAnchor(existing.contextWindow, max);
      if (nextValue === existing.contextWindow) return current;
      return {
        ...current,
        [profileId]: {
          ...existing,
          contextWindow: nextValue,
          contextWindowInput: nextValue.toString(),
        },
      };
    });
  }

  function updateConfiguredContextWindowInput(
    profileId: string,
    value: string,
  ): void {
    const parsed = parseContextWindowInput(value, configuredContextWindowMax(profileId));
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      return {
        ...current,
        [profileId]: {
          ...existing,
          contextWindow: parsed ?? existing.contextWindow,
          contextWindowInput: value,
        },
      };
    });
    setMessage(null);
  }

  function commitConfiguredContextWindowInput(profileId: string): void {
    const max = configuredContextWindowMax(profileId);
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      const nextValue =
        parseContextWindowInput(existing.contextWindowInput, max) ??
        existing.contextWindow;
      return {
        ...current,
        [profileId]: {
          ...existing,
          contextWindow: nextValue,
          contextWindowInput: nextValue.toString(),
        },
      };
    });
  }

  function updateConfiguredMaxTokensInput(
    profileId: string,
    value: string,
  ): void {
    const parsed = parseMaxTokensInput(value);
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      return {
        ...current,
        [profileId]: {
          ...existing,
          maxTokens: parsed ?? existing.maxTokens,
          maxTokensInput: value,
        },
      };
    });
    setMessage(null);
  }

  function commitConfiguredMaxTokensInput(profileId: string): void {
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      const nextValue =
        parseMaxTokensInput(existing.maxTokensInput) ?? existing.maxTokens;
      return {
        ...current,
        [profileId]: {
          ...existing,
          maxTokens: nextValue,
          maxTokensInput: nextValue.toString(),
        },
      };
    });
  }

  async function saveConfiguredDrafts(): Promise<void> {
    const changedProfiles = activeProfiles.filter((profile) => {
      const next = configuredDrafts[profile.id];
      if (!next) return false;
      return (
        next.enabled !== profile.enabled ||
        next.contextWindow !== contextWindowForProfile(profile, providers) ||
        next.maxTokens !== maxTokensForProfile(profile)
      );
    });
    if (changedProfiles.length === 0) return;

    setSavingConfigured(true);
    setMessage(null);
    try {
      for (const profile of changedProfiles) {
        const next = configuredDrafts[profile.id];
        if (!next) continue;
        await runtimeClient.saveModelProfile({
          id: profile.id,
          providerId: profile.providerId,
          displayName: profile.displayName,
          modelId: profile.modelId,
          baseURL: profile.baseURL,
          protocol: profile.protocol,
          authScheme: profile.authScheme,
          enabled: next.enabled,
          isDefault: next.enabled ? profile.isDefault : false,
          advanced: {
            ...profile.advanced,
            contextWindow: next.contextWindow,
            maxTokens: next.maxTokens,
          },
        });
      }
      await loadModelSettings();
      onProfilesChanged();
      showToast("LLM 设置已保存");
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Failed to save configured models",
      );
    } finally {
      setSavingConfigured(false);
    }
  }

  async function deleteConfiguredProfile(
    profile: RuntimeModelProfileSummary,
  ): Promise<void> {
    if (!window.confirm(`删除 ${profile.modelId} 的模型配置？`)) {
      return;
    }

    setDeletingProfileId(profile.id);
    setMessage(null);
    try {
      await runtimeClient.deleteModelProfile(profile.id);
      setConfiguredDrafts((current) => {
        const next = { ...current };
        delete next[profile.id];
        return next;
      });
      if (draft?.profileId === profile.id) {
        setDraft(null);
        setExpandedProviderId(null);
      }
      await loadModelSettings();
      onProfilesChanged();
      showToast("模型配置已删除");
    } catch (error) {
      setMessage(
        error instanceof Error
          ? error.message
          : "Failed to delete model profile",
      );
    } finally {
      setDeletingProfileId(null);
    }
  }

  const selectedProvider = expandedProviderId
    ? activeProviders.find((provider) => provider.id === expandedProviderId) ?? null
    : null;
  const selectedProviderProfiles = selectedProvider
    ? profilesByProviderId.get(selectedProvider.id) ?? []
    : [];
  const selectedProviderDraft = selectedProvider
    ? draft?.providerId === selectedProvider.id
      ? draft
      : createDraftForProvider(selectedProvider)
    : null;
  const selectedProviderStatus = selectedProvider
    ? providerConnectionStatus(
        selectedProviderProfiles,
        selectedProvider.accessType === "agent_plan" && agentPlanAccount?.hasKey === true,
      )
    : "unconfigured";

  if (!open) return null;

  return (
      <AwakenSettings
        rootRef={settingsRootRef}
        presentation="screen"
        className={activeSettingsSection === "models" ? "aw-settings-shell--models" : undefined}
        contentLayout={activeSettingsSection === "memory" || activeSettingsSection === "models" ? "full-bleed" : "standard"}
        navItems={AwakenSettingsNavItems}
        activeSectionId={activeSettingsSection}
        sectionTitle={SETTINGS_SECTION_TITLES[activeSettingsSection]}
        toast={toastMessage}
        onBack={() => onOpenChange(false)}
        onSelectSection={(section) => setActiveSettingsSection(section as SettingsDialogSection)}
      >
          {activeSettingsSection === "search" ? (
            <SearchSettingsPanel runtimeClient={runtimeClient} />
          ) : activeSettingsSection === "memory" ? (
            <MemorySettingsPanel
              runtimeClient={runtimeClient}
              currentProjectId={currentProjectId}
              theme={theme}
            />
          ) : activeSettingsSection === "appearance" ? (
            <AppearanceSettings
              themePreference={themePreference}
              themeSaving={themeSaving}
              message={themeMessage}
              onThemePreferenceChange={onThemePreferenceChange}
            />
          ) : activeSettingsSection === "pets" ? (
            <PetsSettingsPanel runtimeClient={runtimeClient} />
          ) : activeSettingsSection === "acp" ? (
            <ACPSettingsPanel runtimeClient={runtimeClient} />
          ) : (
            <AwakenModelSettings
              theme={theme}
              tabs={MODEL_SETTINGS_TABS}
              activeTabId={activeSection}
              views={[
                { id: "connections", label: MODEL_SETTINGS_COPY[activeSection].connectionLabel },
                { id: "llm", label: "LLM 设置" },
              ]}
              activeViewId={activeModelView}
              title={MODEL_SETTINGS_COPY[activeSection].title}
              description={MODEL_SETTINGS_COPY[activeSection].description}
              message={message}
              onSelectTab={(section) => {
                collapseProviderEditor();
                setActiveSection(section as ModelSettingsSection);
                setMessage(null);
              }}
              onSelectView={(view) => {
                collapseProviderEditor();
                setActiveModelView(view as ModelSettingsView);
                setMessage(null);
              }}
            >
            {activeModelView === "connections" ? (
            <>
              <section className="model-provider-list" aria-label="连接配置">
                {activeProviders.map((provider) => (
                    <AwakenSettingsProviderCard
                      key={provider.id}
                      name={provider.displayName}
                      brand={providerBrand(provider)}
                      selected={expandedProviderId === provider.id}
                      onSelect={() => selectProvider(provider)}
                    />
                ))}
              </section>
              {selectedProvider && selectedProviderDraft ? (
                <AwakenSettingsProviderModal
                  open
                  name={selectedProvider.displayName}
                  brand={providerBrand(selectedProvider)}
                  status={selectedProviderStatus}
                  onClose={collapseProviderEditor}
                >
                    <AwakenModelProviderForm
                      providerId={selectedProvider.id}
                      presentation="modal"
                      modelMode={selectedProvider.allowCustomModel ? "custom" : selectedProvider.endpointModel ? "endpoint" : "select"}
                      models={selectedProvider.models}
                      modelId={selectedProvider.endpointModel ? selectedProviderDraft.endpointId || selectedProviderDraft.modelId : selectedProviderDraft.modelId}
                      baseURL={selectedProviderDraft.baseURL}
                      baseURLReadOnly={
                        selectedProvider.accessType === "agent_plan" || selectedProvider.id === "qwen-token-plan"
                      }
                      apiKey={selectedProviderDraft.apiKey}
                      apiKeyPlaceholder={`输入 ${selectedProvider.keyEnvName}`}
                      apiDocsURL={selectedProvider.apiDocsURL}
                      templateNote={
                        selectedProvider.endpointModel
                            ? `运行时模型必须填写控制台创建的 ep-* 推理接入点；模型族为 ${selectedProvider.defaultModel}。`
                            : selectedProvider.accessType === "agent_plan"
                              ? "首期使用固定单账户；保存 Key 后会从套餐 /models 刷新精确模型 ID，失败时保留 ark-code-latest 与自定义输入。"
                              : null
                      }
                      showApiKey={showApiKey}
                      editableCapabilities={providerHasEditableCapabilities(selectedProvider)}
                      supportsVision={selectedProviderDraft.advanced?.supportsVision === true}
                      supportsNativePDF={selectedProviderDraft.advanced?.supportsNativePDF === true}
                      supportsVideo={selectedProviderDraft.advanced?.supportsVideo === true}
                      saving={saving}
                      testing={testing}
                      testFeedback={testFeedback}
                      testingReasoning={testingReasoning}
                      showReasoningTest={selectedProvider.accessType === "agent_plan" && selectedProviderDraft.modelId !== "ark-code-latest"}
                      showAccountActions={selectedProvider.accessType === "agent_plan"}
                      accountStatus={
                        selectedProvider.accessType === "agent_plan"
                          ? agentPlanAccount?.hasKey
                            ? `${agentPlanAccount.maskedKey ?? "已保存 Key"} · revision ${agentPlanAccount.credentialRevision}`
                            : "尚未保存 Agent Plan Key"
                          : undefined
                      }
                      refreshingAccount={refreshingAgentPlan}
                      deletingAccount={deletingAgentPlan}
                      onModelChange={(nextModelId) => {
                        setTestFeedback(null);
                        setDraft({
                          ...selectedProviderDraft,
                          endpointId: selectedProvider.endpointModel ? nextModelId : selectedProviderDraft.endpointId,
                          modelId: selectedProvider.endpointModel ? selectedProvider.defaultModel : nextModelId,
                          advanced: advancedWithModelCapability(
                            selectedProvider,
                            nextModelId,
                            selectedProviderDraft.advanced,
                            true,
                          ),
                        });
                      }}
                      onBaseURLChange={(baseURL) => {
                        setTestFeedback(null);
                        setDraft({ ...selectedProviderDraft, baseURL });
                      }}
                      onApiKeyChange={(apiKey) => {
                        setTestFeedback(null);
                        setDraft({ ...selectedProviderDraft, apiKey });
                      }}
                      onShowApiKeyChange={setShowApiKey}
                      onCapabilityChange={(capability, checked) => {
                        setTestFeedback(null);
                        if (capability === "supportsVision") {
                          setDraft({
                            ...selectedProviderDraft,
                            advanced: {
                              ...selectedProviderDraft.advanced,
                              supportsVision: checked,
                              supportsVideo: checked ? (selectedProviderDraft.advanced?.supportsVideo ?? false) : false,
                            },
                          });
                        } else if (capability === "supportsVideo") {
                          setDraft({
                            ...selectedProviderDraft,
                            advanced: {
                              ...selectedProviderDraft.advanced,
                              supportsVision: checked ? true : (selectedProviderDraft.advanced?.supportsVision ?? false),
                              supportsVideo: checked,
                            },
                          });
                        } else {
                          setDraft({
                            ...selectedProviderDraft,
                            advanced: {
                              ...selectedProviderDraft.advanced,
                              supportsNativePDF: checked,
                            },
                          });
                        }
                      }}
                      onTest={() => void testDraft()}
                      onTestReasoning={() => void testDraftReasoning()}
                      onRefreshAccount={() => void refreshAgentPlanModels()}
                      onDeleteAccount={() => void deleteAgentPlanAccount()}
                      onCancel={collapseProviderEditor}
                      onSubmit={() => void saveDraft()}
                    />
                </AwakenSettingsProviderModal>
              ) : null}
            </>
            ) : (
            <AwakenConfiguredModels
              models={activeProfiles.map((profile) => {
                const contextWindowMax = contextWindowMaxForProfile(profile, providers);
                const effectiveAnchor = contextWindowAnchor(contextWindowMax);
                const profileDraft = configuredDrafts[profile.id] ?? {
                  enabled: profile.enabled,
                  contextWindow: contextWindowForProfile(profile, providers),
                  contextWindowInput: contextWindowForProfile(profile, providers).toString(),
                  maxTokens: maxTokensForProfile(profile),
                  maxTokensInput: maxTokensForProfile(profile).toString(),
                };
                return {
                  id: profile.id,
                  displayName: profile.displayName,
                  brand: providerById.get(profile.providerId)
                    ? providerBrand(providerById.get(profile.providerId)!)
                    : undefined,
                  modelId: profile.modelId,
                  isDefault: profile.isDefault,
                  credentialStatus: credentialStatusLabel(profile.profileStatus),
                  credentialTone: profile.profileStatus === "unreadable" ? "unreadable" as const : "default" as const,
                  enabled: profileDraft.enabled,
                  contextWindow: profileDraft.contextWindow,
                  contextWindowInput: profileDraft.contextWindowInput,
                  contextWindowMin: CONTEXT_WINDOW_MIN,
                  contextWindowMax,
                  contextWindowStep: CONTEXT_WINDOW_SLIDER_STEP,
                  contextFill: contextWindowFill(profileDraft.contextWindow, contextWindowMax),
                  contextAnchor: contextWindowFill(effectiveAnchor, contextWindowMax),
                  contextAnchorValue: effectiveAnchor,
                  contextAnchorLabel: formatContextWindow(effectiveAnchor),
                  contextEndLabel: formatContextWindow(contextWindowMax),
                  compressionValue: calculateAutoCompactThreshold(profileDraft.contextWindow),
                  maxTokensInput: profileDraft.maxTokensInput,
                };
              })}
              hasChanges={configuredHasChanges}
              saving={savingConfigured}
              deletingModelId={deletingProfileId}
              onEnabledChange={(profileId, enabled) => updateConfiguredDraft(profileId, { enabled })}
              onContextRangeChange={updateConfiguredContextWindow}
              onContextRangeCommit={snapConfiguredContextWindow}
              onContextInputChange={updateConfiguredContextWindowInput}
              onContextInputCommit={commitConfiguredContextWindowInput}
              onContextAnchorSelect={(profileId) =>
                updateConfiguredContextWindow(
                  profileId,
                  contextWindowAnchor(configuredContextWindowMax(profileId)),
                )
              }
              onMaxTokensInputChange={updateConfiguredMaxTokensInput}
              onMaxTokensInputCommit={commitConfiguredMaxTokensInput}
              onSetDefault={(profileId) => {
                void (async () => {
                  setMessage(null);
                  try {
                    await runtimeClient.setDefaultModelProfile(profileId);
                    await loadModelSettings();
                    onProfilesChanged();
                    showToast("默认模型已更新；仅影响新会话初值");
                  } catch (error) {
                    setMessage(error instanceof Error ? error.message : "更新默认模型失败");
                  }
                })();
              }}
              onReset={resetConfiguredDrafts}
              onSave={() => void saveConfiguredDrafts()}
              onUpdateApiKey={(profileId) => {
                const profile = profiles.find((item) => item.id === profileId);
                if (profile) editExistingProfile(profile);
              }}
              onDelete={(profileId) => {
                const profile = profiles.find((item) => item.id === profileId);
                if (profile) void deleteConfiguredProfile(profile);
              }}
            />
            )}
            </AwakenModelSettings>
          )}
      </AwakenSettings>
  );
}
