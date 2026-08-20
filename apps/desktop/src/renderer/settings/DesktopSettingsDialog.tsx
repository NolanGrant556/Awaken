import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import { AgentPlanMediaToolsPanel } from "./AgentPlanMediaToolsPanel";
import { ComputerSettingsPanel } from "./ComputerSettingsPanel";
import { HooksSettingsPanel } from "./HooksSettingsPanel";
import { GlobalAgentsSettingsPanel } from "./GlobalAgentsSettingsPanel";

export type DraftState = {
  profileId?: string;
  templateProfileId?: string;
  providerId: string;
  modelId: string;
  apiKey: string;
  managementApiKey: string;
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
type SettingsDialogSection =
  | "models"
  | "search"
  | "memory"
  | "instructions"
  | "appearance"
  | "pets"
  | "computer"
  | "hooks"
  | "acp";
type ModelConnectionTestFeedback = {
  tone: "success" | "error";
  message: string;
};

const MODEL_SETTINGS_TABS = [
  { id: "coding_plan", label: "Coding Plan" },
  { id: "api", label: "API" },
  { id: "agent_plan", label: "Agent Plan" },
  { id: "custom", label: "自定义" },
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
    description: "管理 Agent Plan 账户、模型与生成工具。",
    connectionLabel: "配置账户",
  },
  custom: {
    title: "自定义",
    description: "管理自行声明协议与推理地址的模型连接。",
    connectionLabel: "配置 API Key",
  },
};

const SETTINGS_SECTION_TITLES: Record<SettingsDialogSection, string> = {
  models: "模型",
  search: "搜索",
  memory: "记忆",
  instructions: "全局规则",
  appearance: "外观",
  pets: "宠物",
  computer: "计算机操作",
  hooks: "钩子",
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
const PROTOCOL_LABELS: Record<string, string> = {
  openai_chat: "OpenAI Chat Completions",
  openai_responses: "OpenAI Responses",
  anthropic_messages: "Anthropic Messages",
  minimax_chat: "MiniMax Legacy",
};
const AUTH_SCHEME_OPTIONS = [
  { value: "bearer", label: "Authorization: Bearer" },
  { value: "x-api-key", label: "x-api-key" },
  { value: "api-key", label: "api-key（Azure）" },
  { value: "x-goog-api-key", label: "x-goog-api-key" },
  { value: "none", label: "无鉴权" },
];
function protocolOptions(provider: ModelProvider, currentProtocol?: string) {
  const protocols = provider.supportedProtocols?.length
    ? provider.supportedProtocols
    : [provider.protocol];
  const availableProtocols =
    currentProtocol && !protocols.includes(currentProtocol)
      ? [...protocols, currentProtocol]
      : protocols;
  return availableProtocols.map((value) => ({
    value,
    label: PROTOCOL_LABELS[value] ??
      (value === "minimax_chat" ? "MiniMax Legacy" : value),
  }));
}

function authSchemeOptions() {
  return AUTH_SCHEME_OPTIONS;
}

function providerBrand(
  provider: ModelProvider,
): AwakenProviderBrand | undefined {
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
  if (!Number.isFinite(value) || value === undefined || value <= 0) return 0;
  return Math.min(max, Math.max(CONTEXT_WINDOW_MIN, Math.round(value ?? max)));
}

function modelCapabilityFor(provider: ModelProvider, modelId: string) {
  return provider.modelCapabilities[modelId];
}

export function modelCandidatesForProtocol(
  provider: ModelProvider,
  protocol: string,
): string[] {
  return provider.models.filter((modelId) => {
    const confirmed = modelCapabilityFor(provider, modelId)?.supportedProtocols;
    // Sparse /models responses and manually entered future model IDs remain
    // admissible. Only an explicit per-model contract narrows suggestions.
    return !confirmed?.length || confirmed.includes(protocol);
  });
}

function contextWindowMaxForProfile(
  profile: RuntimeModelProfileSummary,
  providers: ModelProvider[],
): number {
  const provider = providers.find((p) => p.id === profile.providerId);
  const capability = provider
    ? modelCapabilityFor(provider, profile.modelId)
    : undefined;
  return profile.advanced?.maxInputTokens ??
    capability?.maxInputTokens ??
    capability?.contextWindow ??
    0;
}

function contextWindowForProfile(
  profile: RuntimeModelProfileSummary,
  providers: ModelProvider[],
): number {
  const max = contextWindowMaxForProfile(profile, providers);
  if (max <= 0) {
    const savedValue = profile.advanced?.contextWindow;
    return Number.isFinite(savedValue) && (savedValue ?? 0) > 0
      ? Math.round(savedValue ?? 0)
      : 0;
  }
  return clampContextWindow(profile.advanced?.contextWindow, max);
}

function contextWindowInputValue(value: number): string {
  return value > 0 ? value.toString() : "";
}

function clampMaxTokens(value: number | undefined): number {
  if (!Number.isFinite(value) || value === undefined || value <= 0) return 0;
  return Math.min(
    MAX_TOKENS_MAX,
    Math.max(MAX_TOKENS_MIN, Math.round(value ?? MAX_TOKENS_DEFAULT)),
  );
}

function maxTokensForProfile(profile: RuntimeModelProfileSummary): number {
  return clampMaxTokens(profile.advanced?.maxTokens);
}

function advancedWithModelCapability(
  provider: ModelProvider,
  modelId: string,
  advanced: RuntimeModelProfileSummary["advanced"],
  overwrite: boolean,
): RuntimeModelProfileSummary["advanced"] {
  const capability = modelCapabilityFor(provider, modelId);
  const customVision =
    provider.id === "custom" ? advanced?.supportsVision ?? false : undefined;
  if (!capability) {
    const nextAdvanced = overwrite
      ? {
          endpointId: advanced?.endpointId,
          inferenceEndpoint: advanced?.inferenceEndpoint,
        }
      : advanced;
    return provider.id === "custom"
      ? {
          ...nextAdvanced,
          supportsVision: customVision,
          supportsNativePDF: false,
        }
      : nextAdvanced;
  }
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
    maxInputTokens:
      overwrite || advanced?.maxInputTokens === undefined
        ? capability.maxInputTokens
        : advanced.maxInputTokens,
    maxTokens: shouldUseInferredMaxTokens
      ? inferredMaxTokens
      : advanced.maxTokens,
    supportsVision:
      provider.id === "custom"
        ? customVision
        : overwrite || advanced?.supportsVision === undefined
          ? capability.supportsVision
          : advanced.supportsVision,
    supportsNativePDF:
      provider.id === "custom"
        ? false
        : overwrite || advanced?.supportsNativePDF === undefined
          ? capability.supportsNativePDF
          : advanced.supportsNativePDF,
    supportsVideo:
      overwrite || advanced?.supportsVideo === undefined
        ? capability.supportsVideo
        : advanced.supportsVideo,
    supportsTools: capability.supportsTools ?? advanced?.supportsTools,
    supportsPromptCache:
      overwrite || advanced?.supportsPromptCache === undefined
        ? capability.supportsPromptCache
        : advanced.supportsPromptCache,
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
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined;
  if (max <= 0) {
    return Math.min(
      CONTEXT_WINDOW_MAX,
      Math.max(CONTEXT_WINDOW_MIN, Math.round(parsed)),
    );
  }
  return clampContextWindow(parsed, max);
}

function parseMaxTokensInput(value: string): number | undefined {
  if (value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? clampMaxTokens(parsed) : undefined;
}

function contextWindowFill(value: number, max: number): number {
  if (max <= CONTEXT_WINDOW_MIN) return 0;
  return (
    ((clampContextWindow(value, max) - CONTEXT_WINDOW_MIN) /
      (max - CONTEXT_WINDOW_MIN)) *
    100
  );
}

function contextWindowAnchor(max: number): number {
  if (max <= 0) return 0;
  return clampContextWindow(Math.round(max * CONTEXT_WINDOW_ANCHOR_RATIO), max);
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
    profiles.map((profile) => {
      const contextWindow = contextWindowForProfile(profile, providers);
      const maxTokens = maxTokensForProfile(profile);
      return [
        profile.id,
        {
          enabled: profile.enabled,
          contextWindow,
          contextWindowInput: contextWindowInputValue(contextWindow),
          maxTokens,
          maxTokensInput: maxTokens.toString(),
        },
      ];
    }),
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
    managementApiKey: "",
    baseURL: provider.baseURL,
    protocol: provider.protocol,
    authScheme: provider.authScheme,
    enabled: true,
    isDefault: true,
    endpointId: "",
    advanced,
  };
}

function isEndpointModelId(value: string): boolean {
  return /^ep-[A-Za-z0-9_-]+$/.test(value.trim());
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
    managementApiKey: "",
    baseURL: templateProfile.baseURL || provider.baseURL,
    protocol: templateProfile.protocol || provider.protocol,
    authScheme: templateProfile.authScheme || provider.authScheme,
    enabled: true,
    isDefault,
    endpointId:
      templateProfile.advanced?.endpointId ??
      (provider.endpointModel && isEndpointModelId(templateProfile.modelId)
        ? templateProfile.modelId
        : ""),
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
    managementApiKey: "",
    baseURL: profile.baseURL || provider.baseURL,
    protocol: profile.protocol || provider.protocol,
    authScheme: profile.authScheme || provider.authScheme,
    enabled: profile.enabled,
    isDefault: profile.isDefault,
    endpointId:
      profile.advanced?.endpointId ??
      (provider.endpointModel && isEndpointModelId(profile.modelId)
        ? profile.modelId
        : ""),
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
      endpointId:
        provider.endpointModel && isEndpointModelId(endpointId)
          ? endpointId
          : undefined,
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
    managementApiKey: draft.managementApiKey.trim() || undefined,
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
  const [agentPlanAccount, setAgentPlanAccount] =
    useState<AgentPlanAccount | null>(null);
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
  const [refreshingAgentPlan, setRefreshingAgentPlan] = useState(false);
  const [refreshingCatalog, setRefreshingCatalog] = useState(false);
  const [catalogStatus, setCatalogStatus] = useState<string | null>(null);
  const [deletingAgentPlan, setDeletingAgentPlan] = useState(false);
  const [showApiKey, setShowApiKey] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const toastTimerRef = useRef<number | null>(null);
  const settingsRootRef = useRef<HTMLElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const expandedProviderIdRef = useRef<string | null>(null);
  const catalogRefreshIdRef = useRef(0);

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
          (provider.accessType ??
            (provider.codingPreset ? "coding_plan" : "api")) === activeSection,
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
      const [nextProviders, nextProfiles, agentPlanAccountResult] =
        await Promise.all([
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
      setMessage(
        accountErrorMessage ?? credentialRecoveryMessage(nextProfiles),
      );
      setConfiguredDrafts(
        configuredDraftsFromProfiles(nextProfiles, nextProviders),
      );

      try {
        const syncedProviders = await runtimeClient.syncModelCapabilities();
        const refreshedProfiles = await runtimeClient.listModelProfiles();
        setProviders(syncedProviders);
        setProfiles(refreshedProfiles);
        setMessage(
          accountErrorMessage ?? credentialRecoveryMessage(refreshedProfiles),
        );
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
    expandedProviderIdRef.current = null;
    catalogRefreshIdRef.current += 1;
    setExpandedProviderId(null);
    setDraft(null);
    setShowApiKey(false);
    setTestFeedback(null);
    setCatalogStatus(null);
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
    restoreFocusRef.current =
      document.activeElement instanceof HTMLElement
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
        setCatalogStatus(null);
        setRefreshingCatalog(false);
        catalogRefreshIdRef.current += 1;
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

    const nextDraft = createDraftForProvider(provider);
    expandedProviderIdRef.current = provider.id;
    catalogRefreshIdRef.current += 1;
    setRefreshingCatalog(false);
    setExpandedProviderId(provider.id);
    setDraft(nextDraft);
    setShowApiKey(false);
    setTestFeedback(null);
    setMessage(null);
    setCatalogStatus(
      provider.id === "custom"
        ? null
        : provider.catalogStrategy === "none"
        ? `${provider.displayName} 未提供可由当前推理凭据调用的模型目录 API，请使用官方清单或手填模型 ID。`
        : provider.catalogObservedAt
        ? `当前目录来自供应商 API，观测时间 ${provider.catalogObservedAt}`
        : "当前先展示官方清单与本地缓存；填写或复用 API Key 后可从供应商 API 刷新。",
    );

    const template = providerTemplateProfile(provider.id);
    if (
      provider.catalogStrategy !== "none" &&
      (nextDraft.authScheme === "none" ||
        nextDraft.apiKey.trim() !== "" ||
        template?.hasKey)
    ) {
      void refreshCatalog(provider, nextDraft, true);
    }
  }

  function editExistingProfile(profile: RuntimeModelProfileSummary): void {
    const provider = providerById.get(profile.providerId);
    if (!provider) {
      setMessage("无法找到该模型供应商，请刷新后重试。");
      return;
    }
    setActiveSection(
      provider.accessType ?? (provider.codingPreset ? "coding_plan" : "api"),
    );
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
    if (!draft || (draft.authScheme !== "none" && draft.apiKey.trim() === "")) return;
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
        message:
          error instanceof Error
            ? error.message
            : "连接测试失败：测试请求未完成。",
      });
    } finally {
      setTesting(false);
    }
  }

  async function refreshCatalog(
    provider: ModelProvider,
    catalogDraft: DraftState,
    background = false,
  ): Promise<void> {
    const refreshId = catalogRefreshIdRef.current + 1;
    catalogRefreshIdRef.current = refreshId;
    setRefreshingCatalog(true);
    if (!background) setCatalogStatus(null);
    try {
      const result = await runtimeClient.refreshModelCatalog(
        toSaveRequest(catalogDraft, provider),
      );
      setProviders((current) =>
        current.map((item) =>
          item.id === result.provider.id ? result.provider : item,
        ),
      );
      setDraft((current) => {
        if (!current || current.providerId !== result.provider.id) return current;
        const selectedModelId = current.modelId || result.provider.defaultModel;
        return {
          ...current,
          modelId: selectedModelId,
          advanced: advancedWithModelCapability(
            result.provider,
            selectedModelId,
            current.advanced,
            false,
          ),
        };
      });
      if (
        catalogRefreshIdRef.current === refreshId &&
        expandedProviderIdRef.current === provider.id
      ) {
        setCatalogStatus(result.message);
      }
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : "刷新模型目录失败";
      if (
        catalogRefreshIdRef.current === refreshId &&
        expandedProviderIdRef.current === provider.id
      ) {
        setCatalogStatus(
          background
            ? `供应商目录暂未刷新：${errorMessage} 当前继续使用官方清单或本地缓存。`
            : errorMessage,
        );
      }
    } finally {
      if (catalogRefreshIdRef.current === refreshId) {
        setRefreshingCatalog(false);
      }
    }
  }

  async function refreshSelectedProviderCatalog(): Promise<void> {
    if (!draft) return;
    const provider = providerById.get(draft.providerId);
    if (!provider) return;
    await refreshCatalog(provider, draft);
  }

  async function refreshAgentPlanModels(): Promise<void> {
    setRefreshingAgentPlan(true);
    setMessage(null);
    setCatalogStatus(null);
    try {
      const account = await runtimeClient.refreshAgentPlanModels();
      setAgentPlanAccount(account);
      await loadModelSettings();
      const imageCount = account.catalog.filter((item) => item.kind === "image").length;
      const videoCount = account.catalog.filter((item) => item.kind === "video").length;
      const status = `Agent Plan 目录已刷新：主模型 ark-code-latest，图片 ${imageCount} 个，视频 ${videoCount} 个`;
      setCatalogStatus(status);
      showToast(status);
    } catch (error) {
      const status =
        error instanceof Error ? error.message : "刷新 Agent Plan 模型目录失败";
      setCatalogStatus(status);
      setMessage(status);
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
      setMessage(
        error instanceof Error ? error.message : "删除 Agent Plan 账户失败",
      );
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
    const nextValue = clampContextWindow(
      value,
      configuredContextWindowMax(profileId),
    );
    updateConfiguredDraft(profileId, {
      contextWindow: nextValue,
      contextWindowInput: contextWindowInputValue(nextValue),
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
          contextWindowInput: contextWindowInputValue(nextValue),
        },
      };
    });
  }

  function updateConfiguredContextWindowInput(
    profileId: string,
    value: string,
  ): void {
    const parsed = parseContextWindowInput(
      value,
      configuredContextWindowMax(profileId),
    );
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
          contextWindowInput: contextWindowInputValue(nextValue),
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
    if (!window.confirm(`删除 ${profile.modelDisplayName ?? profile.modelId} 的模型配置？`)) {
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
    ? (activeProviders.find((provider) => provider.id === expandedProviderId) ??
      null)
    : null;
  const selectedProviderProfiles = selectedProvider
    ? (profilesByProviderId.get(selectedProvider.id) ?? [])
    : [];
  const selectedProviderDraft = selectedProvider
    ? draft?.providerId === selectedProvider.id
      ? draft
      : createDraftForProvider(selectedProvider)
    : null;
  const selectedProviderStatus = selectedProvider
    ? providerConnectionStatus(
        selectedProviderProfiles,
        selectedProvider.accessType === "agent_plan" &&
          agentPlanAccount?.hasKey === true,
      )
    : "unconfigured";

  if (!open) return null;

  return (
    <AwakenSettings
      rootRef={settingsRootRef}
      presentation="screen"
      className={
        activeSettingsSection === "models"
          ? "aw-settings-shell--models"
          : undefined
      }
      contentLayout={
        activeSettingsSection === "memory" || activeSettingsSection === "models"
          ? "full-bleed"
          : "standard"
      }
      navItems={AwakenSettingsNavItems}
      activeSectionId={activeSettingsSection}
      sectionTitle={SETTINGS_SECTION_TITLES[activeSettingsSection]}
      toast={toastMessage}
      onBack={() => onOpenChange(false)}
      onSelectSection={(section) =>
        setActiveSettingsSection(section as SettingsDialogSection)
      }
    >
      {activeSettingsSection === "search" ? (
        <SearchSettingsPanel runtimeClient={runtimeClient} />
      ) : activeSettingsSection === "memory" ? (
        <MemorySettingsPanel
          runtimeClient={runtimeClient}
          currentProjectId={currentProjectId}
          theme={theme}
        />
      ) : activeSettingsSection === "instructions" ? (
        <GlobalAgentsSettingsPanel runtimeClient={runtimeClient} />
      ) : activeSettingsSection === "appearance" ? (
        <AppearanceSettings
          themePreference={themePreference}
          themeSaving={themeSaving}
          message={themeMessage}
          onThemePreferenceChange={onThemePreferenceChange}
        />
      ) : activeSettingsSection === "pets" ? (
        <PetsSettingsPanel runtimeClient={runtimeClient} />
      ) : activeSettingsSection === "computer" ? (
        <ComputerSettingsPanel runtimeClient={runtimeClient} />
		) : activeSettingsSection === "hooks" ? (
			<HooksSettingsPanel runtimeClient={runtimeClient} currentProjectId={currentProjectId} />
      ) : activeSettingsSection === "acp" ? (
        <ACPSettingsPanel runtimeClient={runtimeClient} />
      ) : (
        <AwakenModelSettings
          theme={theme}
          tabs={MODEL_SETTINGS_TABS}
          activeTabId={activeSection}
          views={[
            {
              id: "connections",
              label: MODEL_SETTINGS_COPY[activeSection].connectionLabel,
            },
            {
              id: "llm",
              label: activeSection === "agent_plan" ? "模型与工具" : "LLM 设置",
            },
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
                    modelMode={
                      selectedProvider.id === "custom"
                        ? "custom"
                        : selectedProvider.endpointModel
                          ? "endpoint"
                          : selectedProvider.allowCustomModel
                            ? "editable"
                          : "select"
                    }
                    models={modelCandidatesForProtocol(
                      selectedProvider,
                      selectedProviderDraft.protocol,
                    )}
                    modelDisplayNames={selectedProvider.modelDisplayNames}
                    modelId={
                      selectedProvider.endpointModel
                        ? selectedProviderDraft.endpointId ||
                          selectedProviderDraft.modelId
                        : selectedProviderDraft.modelId
                    }
                    protocol={selectedProviderDraft.protocol}
                    protocolOptions={protocolOptions(
                      selectedProvider,
                      selectedProviderDraft.protocol,
                    )}
                    authScheme={selectedProviderDraft.authScheme}
                    authSchemeOptions={authSchemeOptions()}
                    showAuthScheme={selectedProvider.id === "custom"}
                    baseURL={selectedProviderDraft.baseURL}
                    showBaseURL
                    baseURLReadOnly={selectedProvider.id !== "custom"}
                    apiKey={selectedProviderDraft.apiKey}
                    managementApiKey={selectedProviderDraft.managementApiKey}
                    maskedManagementApiKey={
                      selectedProviderProfiles.find((profile) => profile.hasManagementKey)
                        ?.maskedManagementKey
                    }
                    showManagementApiKey={selectedProvider.id === "openai" || selectedProvider.id === "anthropic"}
                    apiKeyPlaceholder={`输入 ${selectedProvider.keyEnvName}`}
                    apiDocsURL={selectedProvider.apiDocsURL}
                    templateNote={
                      selectedProvider.endpointModel
                        ? "按量调用可选择官方 Model ID；使用自建推理接入点时可输入 ep-* Endpoint ID。"
                        : selectedProvider.id === "custom"
                          ? "选择接口协议并填写对应的 Base URL。"
                        : null
                    }
                    showApiKey={showApiKey}
                    showVisionCapability={selectedProvider.id === "custom"}
                    supportsVision={
                      selectedProvider.id === "custom"
                        ? selectedProviderDraft.advanced?.supportsVision ?? false
                        : false
                    }
                    saving={saving}
                    testing={testing}
                    testFeedback={testFeedback}
                    showCatalogRefresh={
                      selectedProvider.id !== "custom" &&
                      selectedProvider.accessType !== "agent_plan" &&
                      selectedProvider.catalogStrategy !== "none"
                    }
                    refreshingCatalog={refreshingCatalog}
                    catalogStatus={
                      selectedProvider.id === "custom"
                        ? undefined
                        : catalogStatus ??
                          (selectedProvider.catalogStrategy === "none"
                            ? `${selectedProvider.displayName} 未提供可由当前推理凭据调用的模型目录 API，请使用官方清单或手填模型 ID。`
                            : undefined)
                    }
                    showAccountActions={
                      selectedProvider.accessType === "agent_plan"
                    }
                    accountStatus={
                      selectedProvider.accessType === "agent_plan"
                        ? agentPlanAccount?.hasKey
                          ? "API Key 已保存"
                          : "尚未配置 API Key"
                        : undefined
                    }
                    refreshingAccount={refreshingAgentPlan}
                    deletingAccount={deletingAgentPlan}
                    onModelChange={(nextModelId) => {
                      setTestFeedback(null);
                      const usesEndpoint =
                        selectedProvider.endpointModel &&
                        isEndpointModelId(nextModelId);
                      setDraft({
                        ...selectedProviderDraft,
                        endpointId: selectedProvider.endpointModel
                          ? usesEndpoint
                            ? nextModelId
                            : ""
                          : selectedProviderDraft.endpointId,
                        modelId: nextModelId,
                        advanced: advancedWithModelCapability(
                          selectedProvider,
                          nextModelId,
                          selectedProviderDraft.advanced,
                          true,
                        ),
                      });
                    }}
                    onProtocolChange={(protocol) => {
                      setTestFeedback(null);
                      const knownOfficialBaseURLs = new Set([
                        selectedProvider.baseURL.replace(/\/+$/, ""),
                        ...Object.values(selectedProvider.protocolBaseURLs).map(
                          (value) => value.replace(/\/+$/, ""),
                        ),
                      ]);
                      const currentBaseURL = selectedProviderDraft.baseURL.replace(
                        /\/+$/,
                        "",
                      );
                      const knownOfficialAuthSchemes = new Set([
                        selectedProvider.authScheme,
                        ...Object.values(selectedProvider.protocolAuthSchemes),
                      ]);
                      setDraft({
                        ...selectedProviderDraft,
                        protocol,
                        baseURL: knownOfficialBaseURLs.has(currentBaseURL)
                          ? selectedProvider.protocolBaseURLs[protocol] ??
                            selectedProvider.baseURL
                          : selectedProviderDraft.baseURL,
                        authScheme: knownOfficialAuthSchemes.has(
                          selectedProviderDraft.authScheme,
                        )
                          ? selectedProvider.protocolAuthSchemes[protocol] ??
                            selectedProvider.authScheme
                          : selectedProviderDraft.authScheme,
                      });
                    }}
                    onAuthSchemeChange={(authScheme) => {
                      setTestFeedback(null);
                      setDraft({ ...selectedProviderDraft, authScheme });
                    }}
                    onBaseURLChange={(baseURL) => {
                      setTestFeedback(null);
                      setDraft({ ...selectedProviderDraft, baseURL });
                    }}
                    onApiKeyChange={(apiKey) => {
                      setTestFeedback(null);
                      setDraft({ ...selectedProviderDraft, apiKey });
                    }}
                    onManagementApiKeyChange={(managementApiKey) => {
                      setTestFeedback(null);
                      setDraft({ ...selectedProviderDraft, managementApiKey });
                    }}
                    onSupportsVisionChange={(supportsVision) => {
                      setTestFeedback(null);
                      setDraft({
                        ...selectedProviderDraft,
                        advanced: {
                          ...selectedProviderDraft.advanced,
                          supportsVision,
                          supportsNativePDF: false,
                        },
                      });
                    }}
                    onRefreshCatalog={() => {
                      void refreshSelectedProviderCatalog();
                    }}
                    onShowApiKeyChange={setShowApiKey}
                    onTest={() => void testDraft()}
                    onRefreshAccount={() => void refreshAgentPlanModels()}
                    onDeleteAccount={() => void deleteAgentPlanAccount()}
                    onCancel={collapseProviderEditor}
                    onSubmit={() => void saveDraft()}
                  />
                </AwakenSettingsProviderModal>
              ) : null}
            </>
          ) : (
            <div
              className={
                activeSection === "agent_plan"
                  ? "agent-plan-models-and-tools"
                  : undefined
              }
            >
              <AwakenConfiguredModels
                models={activeProfiles.map((profile) => {
                  const contextWindowMax = contextWindowMaxForProfile(
                    profile,
                    providers,
                  );
                  const effectiveAnchor = contextWindowAnchor(contextWindowMax);
                  const profileDraft = configuredDrafts[profile.id] ?? {
                    enabled: profile.enabled,
                    contextWindow: contextWindowForProfile(profile, providers),
                    contextWindowInput: contextWindowInputValue(
                      contextWindowForProfile(profile, providers),
                    ),
                    maxTokens: maxTokensForProfile(profile),
                    maxTokensInput: maxTokensForProfile(profile).toString(),
                  };
                  return {
                    id: profile.id,
                    displayName: profile.displayName,
                    brand: providerById.get(profile.providerId)
                      ? providerBrand(providerById.get(profile.providerId)!)
                      : undefined,
                    modelId: profile.modelDisplayName ?? profile.modelId,
                    protocolLabel: PROTOCOL_LABELS[profile.protocol] ?? profile.protocol,
                    isDefault: profile.isDefault,
                    credentialStatus: credentialStatusLabel(
                      profile.profileStatus,
                    ),
                    credentialTone:
                      profile.profileStatus === "unreadable"
                        ? ("unreadable" as const)
                        : ("default" as const),
                    enabled: profileDraft.enabled,
                    contextWindow: profileDraft.contextWindow,
                    contextUnknown: contextWindowMax <= 0,
                    contextWindowInput: profileDraft.contextWindowInput,
                    contextWindowMin: CONTEXT_WINDOW_MIN,
                    contextWindowMax: Math.max(CONTEXT_WINDOW_MIN, contextWindowMax),
                    contextWindowStep: CONTEXT_WINDOW_SLIDER_STEP,
                    contextFill: contextWindowFill(
                      profileDraft.contextWindow,
                      contextWindowMax,
                    ),
                    contextAnchor: contextWindowFill(
                      effectiveAnchor,
                      contextWindowMax,
                    ),
                    contextAnchorValue: effectiveAnchor,
                    contextAnchorLabel: formatContextWindow(effectiveAnchor),
                    contextEndLabel: formatContextWindow(contextWindowMax),
                    compressionValue: contextWindowMax > 0
                      ? calculateAutoCompactThreshold(profileDraft.contextWindow)
                      : "未知",
                    maxTokensInput: profileDraft.maxTokensInput,
                  };
                })}
                hasChanges={configuredHasChanges}
                saving={savingConfigured}
                deletingModelId={deletingProfileId}
                onEnabledChange={(profileId, enabled) =>
                  updateConfiguredDraft(profileId, { enabled })
                }
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
                      setMessage(
                        error instanceof Error
                          ? error.message
                          : "更新默认模型失败",
                      );
                    }
                  })();
                }}
                onReset={resetConfiguredDrafts}
                onSave={() => void saveConfiguredDrafts()}
                onUpdateApiKey={(profileId) => {
                  const profile = profiles.find(
                    (item) => item.id === profileId,
                  );
                  if (profile) editExistingProfile(profile);
                }}
                onDelete={(profileId) => {
                  const profile = profiles.find(
                    (item) => item.id === profileId,
                  );
                  if (profile) void deleteConfiguredProfile(profile);
                }}
              />
              {activeSection === "agent_plan" ? (
                <AgentPlanMediaToolsPanel
                  account={agentPlanAccount}
                  runtimeClient={runtimeClient}
                  onConfigureAccount={() => {
                    const provider = providers.find(
                      (item) => item.id === "volcengine-agent-plan",
                    );
                    setActiveModelView("connections");
                    if (provider) selectProvider(provider);
                  }}
                  onSettingsChanged={loadModelSettings}
                  onToast={showToast}
                />
              ) : null}
            </div>
          )}
        </AwakenModelSettings>
      )}
    </AwakenSettings>
  );
}
