import {
  type CSSProperties,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  ChevronDown,
  Eye,
  EyeOff,
  KeyRound,
  Monitor,
  Moon,
  Sun,
} from "lucide-react";
import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
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
    <section className="appearance-settings" aria-label="外观">
      <div className="appearance-settings-row">
        <div className="appearance-settings-copy">
          <h2>主题</h2>
        </div>
        <div
          className="theme-choice-group"
          role="radiogroup"
          aria-label="主题"
        >
          <button
            type="button"
            className={`theme-choice${
              themePreference === "system" ? " theme-choice-active" : ""
            }`}
            role="radio"
            aria-checked={themePreference === "system"}
            disabled={themeSaving}
            onClick={() => onThemePreferenceChange("system")}
          >
            <Monitor size={17} strokeWidth={1.9} />
            <span>跟随系统</span>
          </button>
          <button
            type="button"
            className={`theme-choice${
              themePreference === "light" ? " theme-choice-active" : ""
            }`}
            role="radio"
            aria-checked={themePreference === "light"}
            disabled={themeSaving}
            onClick={() => onThemePreferenceChange("light")}
          >
            <Sun size={17} strokeWidth={1.9} />
            <span>浅色</span>
          </button>
          <button
            type="button"
            className={`theme-choice${
              themePreference === "dark" ? " theme-choice-active" : ""
            }`}
            role="radio"
            aria-checked={themePreference === "dark"}
            disabled={themeSaving}
            onClick={() => onThemePreferenceChange("dark")}
          >
            <Moon size={17} strokeWidth={1.9} />
            <span>黑色</span>
          </button>
        </div>
      </div>
    </section>
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
  const [openModelPickerProviderId, setOpenModelPickerProviderId] = useState<
    string | null
  >(null);
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
    setOpenModelPickerProviderId(null);
    setDraft(null);
    setShowApiKey(false);
  }

  useEffect(() => {
    if (!open) {
      setExpandedProviderId(null);
      setOpenModelPickerProviderId(null);
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
    setOpenModelPickerProviderId(null);
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
    setOpenModelPickerProviderId(null);
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
      <div className="settings-dialog-shell">
        {toastMessage ? (
          <div
            className="model-settings-toast"
            role="status"
            aria-live="polite"
          >
            {toastMessage}
          </div>
        ) : null}
        <div className="settings-dialog-sidebar">
          <div className="settings-dialog-sidebar-title">设置</div>
          <nav className="settings-dialog-nav" aria-label="Settings sections">
            <button
              type="button"
              role="tab"
              aria-selected={activeSettingsSection === "models"}
            className={`settings-dialog-nav-item${
              activeSettingsSection === "models"
                ? " settings-dialog-nav-item-active"
                : ""
            }`}
            onClick={() => setActiveSettingsSection("models")}
          >
            模型
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSettingsSection === "search"}
            className={`settings-dialog-nav-item${
              activeSettingsSection === "search"
                ? " settings-dialog-nav-item-active"
                : ""
            }`}
            onClick={() => setActiveSettingsSection("search")}
          >
            搜索
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSettingsSection === "memory"}
            className={`settings-dialog-nav-item${
              activeSettingsSection === "memory"
                ? " settings-dialog-nav-item-active"
                : ""
            }`}
            onClick={() => setActiveSettingsSection("memory")}
          >
            记忆
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSettingsSection === "appearance"}
            className={`settings-dialog-nav-item${
              activeSettingsSection === "appearance"
                ? " settings-dialog-nav-item-active"
                : ""
            }`}
            onClick={() => setActiveSettingsSection("appearance")}
          >
            外观
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeSettingsSection === "pets"}
            className={`settings-dialog-nav-item${
              activeSettingsSection === "pets"
                ? " settings-dialog-nav-item-active"
                : ""
            }`}
            onClick={() => setActiveSettingsSection("pets")}
          >
            宠物
          </button>
          </nav>
        </div>

        <div className="model-settings">
          <div className="model-settings-toolbar">
            <h1 className="model-settings-title">
              {activeSettingsSection === "models"
                ? "模型"
                : activeSettingsSection === "search"
                  ? "搜索"
                : activeSettingsSection === "memory"
                  ? "记忆"
                : activeSettingsSection === "appearance"
                  ? "外观"
                  : "宠物"}
            </h1>
            {activeSettingsSection === "models" ? (
              <div
                className="model-settings-tabs"
                role="tablist"
                aria-label="模型功能区"
              >
                <button
                  type="button"
                  className={`model-settings-tab${
                    activeSection === "configure"
                      ? " model-settings-tab-active"
                      : ""
                  }`}
                  role="tab"
                  aria-selected={activeSection === "configure"}
                  onClick={() => setActiveSection("configure")}
                >
                  模型配置
                </button>
                <button
                  type="button"
                  className={`model-settings-tab${
                    activeSection === "configured"
                      ? " model-settings-tab-active"
                      : ""
                  }`}
                  role="tab"
                  aria-selected={activeSection === "configured"}
                  onClick={() => setActiveSection("configured")}
                >
                  已配置模型
                </button>
              </div>
            ) : null}
          </div>
          {activeSettingsSection === "models" && message ? (
            <div className="model-settings-message">
              <span>{message}</span>
            </div>
          ) : null}
          {activeSettingsSection === "appearance" && themeMessage ? (
            <div className="model-settings-message">{themeMessage}</div>
          ) : null}

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
                const isModelPickerOpen =
                  openModelPickerProviderId === provider.id;
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
                  <div className="model-provider-card" key={provider.id}>
                    <button
                      type="button"
                      className="model-provider-toggle"
                      aria-expanded={isExpanded}
                      onClick={() => selectProvider(provider)}
                    >
                      <span className="model-provider-icon">
                        {provider.displayName.slice(0, 1)}
                      </span>
                      <span className="model-provider-title">
                        <strong>{provider.displayName}</strong>
                      </span>
                      <ChevronDown
                        className="model-provider-chevron"
                        size={15}
                        strokeWidth={1.9}
                      />
                    </button>
                    {isExpanded ? (
                      <div className="model-provider-form">
                        {configuredModelList ? (
                          <div className="model-provider-template-note">
                            {providerDraft.profileId
                              ? `正在更新已配置模型：${providerDraft.modelId}。新的全局 API key 只有安全保存成功后才会替换原凭据。`
                              : `已配置模型：${configuredModelList}。当前提交会新增模型配置；同厂商同模型 ID 会提示失败。`}
                          </div>
                        ) : null}
                        {provider.allowCustomModel ? (
                          <div className="model-provider-field">
                            <label
                              id={`${provider.id}-model-label`}
                              htmlFor={`${provider.id}-model-input`}
                            >
                              模型
                            </label>
                            <div className="model-custom-picker">
                              <input
                                id={`${provider.id}-model-input`}
                                value={providerDraft.modelId}
                                aria-expanded={isModelPickerOpen}
                                aria-controls={`${provider.id}-model-suggestions`}
                                onClick={() =>
                                  setOpenModelPickerProviderId((current) =>
                                    current === provider.id
                                      ? null
                                      : provider.id,
                                  )
                                }
                                onChange={(event) => {
                                  const nextModelId = event.target.value;
                                  setDraft({
                                    ...providerDraft,
                                    modelId: nextModelId,
                                    advanced: advancedWithModelCapability(
                                      provider,
                                      nextModelId,
                                      providerDraft.advanced,
                                      true,
                                    ),
                                  });
                                }}
                                onKeyDown={(event) => {
                                  if (event.key === "Escape") {
                                    setOpenModelPickerProviderId(null);
                                  }
                                }}
                              />
                              <button
                                type="button"
                                className="model-custom-picker-trigger"
                                aria-haspopup="listbox"
                                aria-expanded={isModelPickerOpen}
                                aria-label="展开模型建议"
                                onClick={() =>
                                  setOpenModelPickerProviderId((current) =>
                                    current === provider.id
                                      ? null
                                      : provider.id,
                                  )
                                }
                              >
                                <ChevronDown
                                  className="model-picker-chevron"
                                  size={16}
                                  strokeWidth={1.9}
                                />
                              </button>
                              {isModelPickerOpen ? (
                                <div
                                  id={`${provider.id}-model-suggestions`}
                                  className="model-picker-menu"
                                  role="listbox"
                                  aria-labelledby={`${provider.id}-model-label`}
                                >
                                  {provider.models.map((model) => (
                                    <button
                                      key={model}
                                      type="button"
                                      className={`model-picker-option${
                                        providerDraft.modelId === model
                                          ? " model-picker-option-selected"
                                          : ""
                                      }`}
                                      role="option"
                                      aria-selected={
                                        providerDraft.modelId === model
                                      }
                                      onClick={() => {
                                        setDraft({
                                          ...providerDraft,
                                          modelId: model,
                                          advanced: advancedWithModelCapability(
                                            provider,
                                            model,
                                            providerDraft.advanced,
                                            true,
                                          ),
                                        });
                                        setOpenModelPickerProviderId(null);
                                      }}
                                    >
                                      <span>{model}</span>
                                    </button>
                                  ))}
                                </div>
                              ) : null}
                            </div>
                          </div>
                        ) : provider.endpointModel ? (
                          <label>
                            <span>模型</span>
                            <input
                              list={`${provider.id}-models`}
                              value={
                                provider.endpointModel
                                  ? providerDraft.endpointId ||
                                    providerDraft.modelId
                                  : providerDraft.modelId
                              }
                              onChange={(event) => {
                                const nextModelId = event.target.value;
                                setDraft({
                                  ...providerDraft,
                                  endpointId: provider.endpointModel
                                    ? nextModelId
                                    : providerDraft.endpointId,
                                  modelId: nextModelId,
                                  advanced: advancedWithModelCapability(
                                    provider,
                                    nextModelId,
                                    providerDraft.advanced,
                                    true,
                                  ),
                                });
                              }}
                            />
                            <datalist id={`${provider.id}-models`}>
                              {provider.models.map((model) => (
                                <option key={model} value={model} />
                              ))}
                            </datalist>
                          </label>
                        ) : (
                          <div className="model-provider-field">
                            <label id={`${provider.id}-model-label`}>
                              模型
                            </label>
                            <div className="model-picker">
                              <button
                                type="button"
                                className="model-picker-trigger"
                                aria-haspopup="listbox"
                                aria-expanded={isModelPickerOpen}
                                aria-labelledby={`${provider.id}-model-label`}
                                onClick={() =>
                                  setOpenModelPickerProviderId((current) =>
                                    current === provider.id
                                      ? null
                                      : provider.id,
                                  )
                                }
                                onKeyDown={(event) => {
                                  if (event.key === "Escape") {
                                    setOpenModelPickerProviderId(null);
                                  }
                                }}
                              >
                                <span>{providerDraft.modelId}</span>
                                <ChevronDown
                                  className="model-picker-chevron"
                                  size={16}
                                  strokeWidth={1.9}
                                />
                              </button>
                              {isModelPickerOpen ? (
                                <div
                                  className="model-picker-menu"
                                  role="listbox"
                                  aria-labelledby={`${provider.id}-model-label`}
                                >
                                  {provider.models.map((model) => (
                                    <button
                                      key={model}
                                      type="button"
                                      className={`model-picker-option${
                                        providerDraft.modelId === model
                                          ? " model-picker-option-selected"
                                          : ""
                                      }`}
                                      role="option"
                                      aria-selected={
                                        providerDraft.modelId === model
                                      }
                                      onClick={() => {
                                        setDraft({
                                          ...providerDraft,
                                          modelId: model,
                                          advanced: advancedWithModelCapability(
                                            provider,
                                            model,
                                            providerDraft.advanced,
                                            true,
                                          ),
                                        });
                                        setOpenModelPickerProviderId(null);
                                      }}
                                    >
                                      <span>{model}</span>
                                    </button>
                                  ))}
                                </div>
                              ) : null}
                            </div>
                          </div>
                        )}
                        <label>
                          <span>Base URL</span>
                          <input
                            value={providerDraft.baseURL}
                            onChange={(event) =>
                              setDraft({
                                ...providerDraft,
                                baseURL: event.target.value,
                              })
                            }
                          />
                        </label>
                        <div className="model-provider-field">
                          <label htmlFor={`${provider.id}-api-key`}>
                            API 密钥
                          </label>
                          <div className="model-provider-secret-field">
                            <input
                              id={`${provider.id}-api-key`}
                              type={showApiKey ? "text" : "password"}
                              value={providerDraft.apiKey}
                              placeholder={`输入 ${provider.keyEnvName}`}
                              onChange={(event) =>
                                setDraft({
                                  ...providerDraft,
                                  apiKey: event.target.value,
                                })
                              }
                            />
                            <button
                              type="button"
                              className="model-provider-secret-toggle"
                              aria-label={
                                showApiKey ? "隐藏 API 密钥" : "显示 API 密钥"
                              }
                              aria-pressed={showApiKey}
                              onClick={() =>
                                setShowApiKey((current) => !current)
                              }
                            >
                              {showApiKey ? (
                                <EyeOff size={15} strokeWidth={1.9} />
                              ) : (
                                <Eye size={15} strokeWidth={1.9} />
                              )}
                            </button>
                          </div>
                        </div>
                        {providerHasEditableCapabilities(provider) ? (
                          <div className="model-provider-field">
                            <label>输入能力</label>
                            <div className="model-provider-capability-grid">
                              <label className="model-provider-capability-card">
                                <span className="model-provider-capability-copy">
                                  <strong>支持图片输入</strong>
                                  <span>
                                    用于输入框图片粘贴、预览与发送 gating。
                                  </span>
                                </span>
                                <span className="configured-model-switch">
                                  <input
                                    type="checkbox"
                                    checked={
                                      providerDraft.advanced?.supportsVision ===
                                      true
                                    }
                                    onChange={(event) =>
                                      setDraft({
                                        ...providerDraft,
                                        advanced: {
                                          ...providerDraft.advanced,
                                          supportsVision: event.target.checked,
                                          supportsVideo: event.target.checked
                                            ? (providerDraft.advanced
                                                ?.supportsVideo ?? false)
                                            : false,
                                        },
                                      })
                                    }
                                    aria-label={`${providerDraft.modelId} 支持图片输入`}
                                  />
                                  <span />
                                </span>
                              </label>
                              <label className="model-provider-capability-card">
                                <span className="model-provider-capability-copy">
                                  <strong>支持原生 PDF 输入</strong>
                                  <span>
                                    开启后 Read 会直接向模型发送 PDF；关闭时使用视觉模型的 Poppler 页面渲染回退。
                                  </span>
                                </span>
                                <span className="configured-model-switch">
                                  <input
                                    type="checkbox"
                                    checked={
                                      providerDraft.advanced?.supportsNativePDF ===
                                      true
                                    }
                                    onChange={(event) =>
                                      setDraft({
                                        ...providerDraft,
                                        advanced: {
                                          ...providerDraft.advanced,
                                          supportsNativePDF: event.target.checked,
                                        },
                                      })
                                    }
                                    aria-label={`${providerDraft.modelId} 支持原生 PDF 输入`}
                                  />
                                  <span />
                                </span>
                              </label>
                              <label className="model-provider-capability-card">
                                <span className="model-provider-capability-copy">
                                  <strong>支持视频输入</strong>
                                  <span>
                                    当前只保存能力位，输入框暂不开放视频上传。
                                  </span>
                                </span>
                                <span className="configured-model-switch">
                                  <input
                                    type="checkbox"
                                    checked={
                                      providerDraft.advanced?.supportsVideo ===
                                      true
                                    }
                                    disabled={
                                      providerDraft.advanced?.supportsVision !==
                                      true
                                    }
                                    onChange={(event) =>
                                      setDraft({
                                        ...providerDraft,
                                        advanced: {
                                          ...providerDraft.advanced,
                                          supportsVision: event.target.checked
                                            ? true
                                            : (providerDraft.advanced
                                                ?.supportsVision ?? false),
                                          supportsVideo: event.target.checked,
                                        },
                                      })
                                    }
                                    aria-label={`${providerDraft.modelId} 支持视频输入`}
                                  />
                                  <span />
                                </span>
                              </label>
                            </div>
                          </div>
                        ) : null}
                        <div className="model-provider-form-footer">
                          <div className="model-provider-link-row">
                            <a
                              href={provider.apiDocsURL}
                              target="_blank"
                              rel="noreferrer"
                            >
                              <KeyRound size={14} />
                              获取 API 密钥
                            </a>
                          </div>
                          <div className="model-provider-form-actions">
                            <Button
                              variant="default"
                              size="md"
                              loading={testing}
                              disabled={saving}
                              onClick={() => {
                                void testDraft();
                              }}
                            >
                              测试连接
                            </Button>
                            <Button
                              variant="primary"
                              size="md"
                              loading={saving}
                              disabled={testing}
                              onClick={() => {
                                void saveDraft();
                              }}
                            >
                              提交
                            </Button>
                          </div>
                        </div>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </section>
          ) : (
            <section className="configured-models" aria-label="已配置模型">
              <div className="configured-models-table">
                <div className="configured-models-row configured-models-head">
                  <span>模型名称</span>
                  <span>启用状态</span>
                  <span>上下文长度</span>
                  <span>自动压缩阈值</span>
                  <span>输出上限</span>
                  <span aria-hidden="true" />
                </div>
                {profiles.length === 0 ? (
                  <div className="configured-models-empty">暂无已配置模型</div>
                ) : (
                  profiles.map((profile) => {
                    const contextWindowMax = contextWindowMaxForProfile(
                      profile,
                      providers,
                    );
                    const effectiveAnchor = contextWindowAnchor(contextWindowMax);
                    const profileDraft = configuredDrafts[profile.id] ?? {
                      enabled: profile.enabled,
                      contextWindow: contextWindowForProfile(profile, providers),
                      contextWindowInput: contextWindowForProfile(
                        profile,
                        providers,
                      ).toString(),
                      maxTokens: maxTokensForProfile(profile),
                      maxTokensInput: maxTokensForProfile(profile).toString(),
                    };
                    const contextStyle = {
                      "--context-fill": `${contextWindowFill(
                        profileDraft.contextWindow,
                        contextWindowMax,
                      )}%`,
                      "--context-anchor": `${contextWindowFill(
                        effectiveAnchor,
                        contextWindowMax,
                      )}%`,
                    } as CSSProperties;
                    return (
                      <div className="configured-models-row" key={profile.id}>
                        <div className="configured-model-name">
                          <span className="configured-model-icon">
                            {profile.displayName.slice(0, 1)}
                          </span>
                          <span className="configured-model-copy">
                            <strong>{profile.modelId}</strong>
                            {credentialStatusLabel(profile.profileStatus) ? (
                              <span className={`configured-model-credential-status configured-model-credential-status-${profile.profileStatus}`}>
                                {credentialStatusLabel(profile.profileStatus)}
                              </span>
                            ) : null}
                          </span>
                        </div>
                        <label className="configured-model-switch">
                          <input
                            type="checkbox"
                            checked={profileDraft.enabled}
                            onChange={(event) =>
                              updateConfiguredDraft(profile.id, {
                                enabled: event.target.checked,
                              })
                            }
                            aria-label={`${profile.modelId} 启用状态`}
                          />
                          <span />
                        </label>
                        <div
                          className="configured-context"
                          style={contextStyle}
                        >
                          <div className="configured-context-main">
                            <input
                              className="configured-context-range"
                              type="range"
                              min={CONTEXT_WINDOW_MIN}
                              max={contextWindowMax}
                              step={CONTEXT_WINDOW_SLIDER_STEP}
                              value={profileDraft.contextWindow}
                              onChange={(event) =>
                                updateConfiguredContextWindow(
                                  profile.id,
                                  Number(event.target.value),
                                )
                              }
                              onPointerUp={() =>
                                snapConfiguredContextWindow(profile.id)
                              }
                              onKeyUp={() =>
                                snapConfiguredContextWindow(profile.id)
                              }
                              onBlur={() =>
                                snapConfiguredContextWindow(profile.id)
                              }
                              aria-label={`${profile.modelId} 上下文长度`}
                            />
                            <input
                              className="configured-context-input"
                              type="text"
                              inputMode="numeric"
                              pattern="[0-9]*"
                              value={profileDraft.contextWindowInput}
                              onChange={(event) =>
                                updateConfiguredContextWindowInput(
                                  profile.id,
                                  event.target.value,
                                )
                              }
                              onBlur={() =>
                                commitConfiguredContextWindowInput(profile.id)
                              }
                              aria-label={`${profile.modelId} 精确上下文长度`}
                            />
                          </div>
                          <div className="configured-context-scale">
                            <span className="configured-context-scale-label configured-context-scale-start">
                              8K
                            </span>
                            <button
                              type="button"
                              className="configured-context-anchor"
                              onClick={() =>
                                updateConfiguredContextWindow(
                                  profile.id,
                                  effectiveAnchor,
                                )
                              }
                              aria-label={`${profile.modelId} 设置上下文长度为 ${effectiveAnchor}`}
                            >
                              {formatContextWindow(effectiveAnchor)}
                            </button>
                            <span className="configured-context-scale-label configured-context-scale-end">
                              {formatContextWindow(contextWindowMax)}
                            </span>
                          </div>
                        </div>
                        <span className="configured-compression-value">
                          {calculateAutoCompactThreshold(
                            profileDraft.contextWindow,
                          )}
                        </span>
                        <input
                          className="configured-token-input"
                          type="text"
                          inputMode="numeric"
                          pattern="[0-9]*"
                          value={profileDraft.maxTokensInput}
                          onChange={(event) =>
                            updateConfiguredMaxTokensInput(
                              profile.id,
                              event.target.value,
                            )
                          }
                          onBlur={() =>
                            commitConfiguredMaxTokensInput(profile.id)
                          }
                          aria-label={`${profile.modelId} 输出 token 上限`}
                        />
                        <div className="configured-models-actions">
                          <div className="configured-models-save-actions">
                            <Button
                              variant="default"
                              size="md"
                              disabled={
                                !configuredHasChanges ||
                                savingConfigured ||
                                Boolean(deletingProfileId)
                              }
                              onClick={resetConfiguredDrafts}
                            >
                              取消
                            </Button>
                            <Button
                              variant="primary"
                              size="md"
                              loading={savingConfigured}
                              disabled={
                                !configuredHasChanges ||
                                Boolean(deletingProfileId)
                              }
                              onClick={() => {
                                void saveConfiguredDrafts();
                              }}
                            >
                              保存
                            </Button>
                          </div>
                          <Button
                            variant="default"
                            size="md"
                            disabled={
                              savingConfigured || Boolean(deletingProfileId)
                            }
                            onClick={() => editExistingProfile(profile)}
                          >
                            更新 API key
                          </Button>
                          <Button
                            className="configured-models-delete"
                            variant="default"
                            size="md"
                            loading={deletingProfileId === profile.id}
                            disabled={
                              savingConfigured || Boolean(deletingProfileId)
                            }
                            onClick={() => {
                              void deleteConfiguredProfile(profile);
                            }}
                          >
                            删除
                          </Button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </section>
          )}
        </div>
      </div>
    </Dialog>
  );
}
