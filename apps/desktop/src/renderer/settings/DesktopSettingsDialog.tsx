import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AwakenAppearanceSettings,
  AwakenButton as Button,
  AwakenConfiguredModels,
  AwakenDialog as Dialog,
  AwakenModelProviderForm,
  AwakenSettings,
  AwakenSettingsProviderCard,
} from "@awaken/design-system";
import { calculateAutoCompactThreshold } from "../../shared/runtime";
import type {
  AwakenThemePreference,
  ModelProfileSaveRequest,
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { PetsSettingsPanel } from "./PetsSettingsPanel";
import { SearchSettingsPanel } from "./SearchSettingsPanel";
import { MemorySettingsPanel } from "./MemorySettingsPanel";

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

type ModelSettingsSection = "configure" | "configured";
type SettingsDialogSection = "models" | "search" | "memory" | "appearance" | "pets";

const SETTINGS_NAV_ITEMS = [
  { id: "models", label: "模型" },
  { id: "search", label: "搜索" },
  { id: "memory", label: "记忆" },
  { id: "appearance", label: "外观" },
  { id: "pets", label: "宠物" },
];

const MODEL_SETTINGS_TABS = [
  { id: "configure", label: "模型配置" },
  { id: "configured", label: "已配置模型" },
];

const SETTINGS_SECTION_TITLES: Record<SettingsDialogSection, string> = {
  models: "模型",
  search: "搜索",
  memory: "记忆",
  appearance: "外观",
  pets: "宠物",
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
  onThemePreferenceChange: (theme: AwakenThemePreference) => void;
};

export function AppearanceSettings({
  themePreference,
  themeSaving,
  onThemePreferenceChange,
}: AppearanceSettingsProps) {
  return (
    <AwakenAppearanceSettings
      value={themePreference}
      disabled={themeSaving}
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
    endpointId: provider.endpointModel ? provider.defaultModel : "",
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
  const modelId = provider.endpointModel
    ? endpointId || draft.modelId
    : draft.modelId;
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
  themePreference,
  themeSaving,
  themeMessage,
  onThemePreferenceChange,
  onOpenChange,
  onProfilesChanged,
  currentProjectId,
  initialSection = "models",
  initialModelSection = "configure",
}: DesktopSettingsDialogProps) {
  const [providers, setProviders] = useState<ModelProvider[]>([]);
  const [profiles, setProfiles] = useState<RuntimeModelProfileSummary[]>([]);
  const [configuredDrafts, setConfiguredDrafts] = useState<
    Record<string, ConfiguredProfileDraft>
  >({});
  const [activeSection, setActiveSection] =
    useState<ModelSettingsSection>("configure");
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
  const [showApiKey, setShowApiKey] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const toastTimerRef = useRef<number | null>(null);

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

  const loadModelSettings = useCallback(async () => {
    setMessage(null);
    try {
      const [nextProviders, nextProfiles] = await Promise.all([
        runtimeClient.listModelProviders(),
        runtimeClient.listModelProfiles(),
      ]);
      setProviders(nextProviders);
      setProfiles(nextProfiles);
      setMessage(credentialRecoveryMessage(nextProfiles));
      setConfiguredDrafts(
        configuredDraftsFromProfiles(nextProfiles, nextProviders),
      );

      try {
        const syncedProviders = await runtimeClient.syncModelCapabilities();
        const refreshedProfiles = await runtimeClient.listModelProfiles();
        setProviders(syncedProviders);
        setProfiles(refreshedProfiles);
        setMessage(credentialRecoveryMessage(refreshedProfiles));
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
  }

  useEffect(() => {
    if (!open) {
      setExpandedProviderId(null);
      setDraft(null);
      setActiveSection(initialModelSection);
      setActiveSettingsSection(initialSection);
      setShowApiKey(false);
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
    setMessage(null);
  }

  function editExistingProfile(profile: RuntimeModelProfileSummary): void {
    const provider = providerById.get(profile.providerId);
    if (!provider) {
      setMessage("无法找到该模型供应商，请刷新后重试。");
      return;
    }
    setActiveSection("configure");
    setExpandedProviderId(provider.id);
    setDraft(draftFromExistingProfile(profile, provider));
    setShowApiKey(false);
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
    setMessage(null);
    try {
      const result = await runtimeClient.testModelProfile(
        toSaveRequest(draft, provider),
      );
      showToast(result.message);
    } catch (error) {
      showToast(
        error instanceof Error
          ? error.message
          : "连接测试失败：测试请求未完成。",
      );
    } finally {
      setTesting(false);
    }
  }

  const configuredHasChanges = profiles.some((profile) => {
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
    setConfiguredDrafts(configuredDraftsFromProfiles(profiles, providers));
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
    const changedProfiles = profiles.filter((profile) => {
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
      showToast("已配置模型已保存");
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

  return (
    <Dialog
      open={open}
      title="设置"
      onOpenChange={onOpenChange}
      footer={
        <Button variant="default" size="md" onClick={() => onOpenChange(false)}>
          关闭
        </Button>
      }
    >
      <AwakenSettings
        navItems={SETTINGS_NAV_ITEMS}
        activeSectionId={activeSettingsSection}
        sectionTitle={SETTINGS_SECTION_TITLES[activeSettingsSection]}
        tabs={activeSettingsSection === "models" ? MODEL_SETTINGS_TABS : undefined}
        activeTabId={activeSection}
        message={
          activeSettingsSection === "models"
            ? message
            : activeSettingsSection === "appearance"
              ? themeMessage
              : null
        }
        toast={toastMessage}
        onSelectSection={(section) => setActiveSettingsSection(section as SettingsDialogSection)}
        onSelectTab={(section) => setActiveSection(section as ModelSettingsSection)}
      >
          {activeSettingsSection === "search" ? (
            <SearchSettingsPanel runtimeClient={runtimeClient} />
          ) : activeSettingsSection === "memory" ? (
            <MemorySettingsPanel runtimeClient={runtimeClient} currentProjectId={currentProjectId} />
          ) : activeSettingsSection === "appearance" ? (
            <AppearanceSettings
              themePreference={themePreference}
              themeSaving={themeSaving}
              onThemePreferenceChange={onThemePreferenceChange}
            />
          ) : activeSettingsSection === "pets" ? (
            <PetsSettingsPanel runtimeClient={runtimeClient} />
          ) : activeSection === "configure" ? (
            <section className="model-provider-list" aria-label="模型配置">
              {providers.map((provider) => {
                const isExpanded = expandedProviderId === provider.id;
                const providerProfiles =
                  profilesByProviderId.get(provider.id) ?? [];
                const configuredModelList = providerProfiles
                  .map((profile) => profile.modelId)
                  .join(" / ");
                const providerDraft =
                  draft?.providerId === provider.id
                    ? draft
                    : createDraftForProvider(provider);
                return (
                  <AwakenSettingsProviderCard
                    key={provider.id}
                    name={provider.displayName}
                    expanded={isExpanded}
                    onExpandedChange={() => selectProvider(provider)}
                  >
                    <AwakenModelProviderForm
                      providerId={provider.id}
                      modelMode={provider.allowCustomModel ? "custom" : provider.endpointModel ? "endpoint" : "select"}
                      models={provider.models}
                      modelId={provider.endpointModel ? providerDraft.endpointId || providerDraft.modelId : providerDraft.modelId}
                      baseURL={providerDraft.baseURL}
                      apiKey={providerDraft.apiKey}
                      apiKeyPlaceholder={`输入 ${provider.keyEnvName}`}
                      apiDocsURL={provider.apiDocsURL}
                      templateNote={
                        configuredModelList
                          ? providerDraft.profileId
                            ? `正在更新已配置模型：${providerDraft.modelId}。新的全局 API key 只有安全保存成功后才会替换原凭据。`
                            : `已配置模型：${configuredModelList}。当前提交会新增模型配置；同厂商同模型 ID 会提示失败。`
                          : null
                      }
                      showApiKey={showApiKey}
                      editableCapabilities={providerHasEditableCapabilities(provider)}
                      supportsVision={providerDraft.advanced?.supportsVision === true}
                      supportsNativePDF={providerDraft.advanced?.supportsNativePDF === true}
                      supportsVideo={providerDraft.advanced?.supportsVideo === true}
                      saving={saving}
                      testing={testing}
                      onModelChange={(nextModelId) =>
                        setDraft({
                          ...providerDraft,
                          endpointId: provider.endpointModel ? nextModelId : providerDraft.endpointId,
                          modelId: nextModelId,
                          advanced: advancedWithModelCapability(
                            provider,
                            nextModelId,
                            providerDraft.advanced,
                            true,
                          ),
                        })
                      }
                      onBaseURLChange={(baseURL) => setDraft({ ...providerDraft, baseURL })}
                      onApiKeyChange={(apiKey) => setDraft({ ...providerDraft, apiKey })}
                      onShowApiKeyChange={setShowApiKey}
                      onCapabilityChange={(capability, checked) => {
                        if (capability === "supportsVision") {
                          setDraft({
                            ...providerDraft,
                            advanced: {
                              ...providerDraft.advanced,
                              supportsVision: checked,
                              supportsVideo: checked ? (providerDraft.advanced?.supportsVideo ?? false) : false,
                            },
                          });
                        } else if (capability === "supportsVideo") {
                          setDraft({
                            ...providerDraft,
                            advanced: {
                              ...providerDraft.advanced,
                              supportsVision: checked ? true : (providerDraft.advanced?.supportsVision ?? false),
                              supportsVideo: checked,
                            },
                          });
                        } else {
                          setDraft({
                            ...providerDraft,
                            advanced: {
                              ...providerDraft.advanced,
                              supportsNativePDF: checked,
                            },
                          });
                        }
                      }}
                      onTest={() => void testDraft()}
                      onSubmit={() => void saveDraft()}
                    />
                  </AwakenSettingsProviderCard>
                );
              })}
            </section>
          ) : (
            <AwakenConfiguredModels
              models={profiles.map((profile) => {
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
                  modelId: profile.modelId,
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
      </AwakenSettings>
    </Dialog>
  );
}
