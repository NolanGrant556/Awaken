import {
  type CSSProperties,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { ChevronDown, Eye, EyeOff, KeyRound } from "lucide-react";
import {
  AwakenButton as Button,
  AwakenDialog as Dialog,
} from "@awaken/design-system";
import { calculateAutoCompactThreshold } from "../shared/runtime";
import type {
  ModelProfileSaveRequest,
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";

type DraftState = {
  profileId?: string;
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
};

type ModelSettingsSection = "configure" | "configured";

type ConfiguredProfileDraft = {
  enabled: boolean;
  contextWindow: number;
  contextWindowInput: string;
  maxTokens: number;
  maxTokensInput: string;
};

const CONTEXT_WINDOW_MIN = 8_000;
const CONTEXT_WINDOW_MAX = 1_000_000;
const CONTEXT_WINDOW_SLIDER_STEP = 1;
const CONTEXT_WINDOW_ANCHOR = 400_000;
const CONTEXT_WINDOW_ANCHOR_SNAP_TOLERANCE = 10_000;
const MAX_TOKENS_DEFAULT = 8192;
const MAX_TOKENS_MIN = 256;
const MAX_TOKENS_MAX = 1_000_000;

type ModelSettingsDialogProps = {
  open: boolean;
  runtimeClient: RuntimeClient;
  onOpenChange: (open: boolean) => void;
  onProfilesChanged: () => void;
};

function clampContextWindow(value: number | undefined): number {
  if (!Number.isFinite(value)) return CONTEXT_WINDOW_MAX;
  return Math.min(
    CONTEXT_WINDOW_MAX,
    Math.max(CONTEXT_WINDOW_MIN, Math.round(value ?? CONTEXT_WINDOW_MAX)),
  );
}

function contextWindowForProfile(profile: RuntimeModelProfileSummary): number {
  return clampContextWindow(profile.advanced?.contextWindow);
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

function modelCapabilityFor(provider: ModelProvider, modelId: string) {
  return provider.modelCapabilities[modelId];
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
    supportsVideo:
      overwrite || advanced?.supportsVideo === undefined
        ? capability.supportsVideo
        : advanced.supportsVideo,
  };
}

function parseContextWindowInput(value: string): number | undefined {
  if (value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? clampContextWindow(parsed) : undefined;
}

function parseMaxTokensInput(value: string): number | undefined {
  if (value.trim() === "") return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? clampMaxTokens(parsed) : undefined;
}

function contextWindowFill(value: number): number {
  return (
    ((clampContextWindow(value) - CONTEXT_WINDOW_MIN) /
      (CONTEXT_WINDOW_MAX - CONTEXT_WINDOW_MIN)) *
    100
  );
}

function snapContextWindowAnchor(value: number): number {
  return Math.abs(value - CONTEXT_WINDOW_ANCHOR) <=
    CONTEXT_WINDOW_ANCHOR_SNAP_TOLERANCE
    ? CONTEXT_WINDOW_ANCHOR
    : value;
}

function configuredDraftsFromProfiles(
  profiles: RuntimeModelProfileSummary[],
): Record<string, ConfiguredProfileDraft> {
  return Object.fromEntries(
    profiles.map((profile) => [
      profile.id,
      {
        enabled: profile.enabled,
        contextWindow: contextWindowForProfile(profile),
        contextWindowInput: contextWindowForProfile(profile).toString(),
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

function draftFromProfile(
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
  };
}

function toSaveRequest(
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

export function ModelSettingsDialog({
  open,
  runtimeClient,
  onOpenChange,
  onProfilesChanged,
}: ModelSettingsDialogProps) {
  const [providers, setProviders] = useState<ModelProvider[]>([]);
  const [profiles, setProfiles] = useState<RuntimeModelProfileSummary[]>([]);
  const [configuredDrafts, setConfiguredDrafts] = useState<
    Record<string, ConfiguredProfileDraft>
  >({});
  const [activeSection, setActiveSection] =
    useState<ModelSettingsSection>("configure");
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

  const profileByProviderId = useMemo(() => {
    return new Map(profiles.map((profile) => [profile.providerId, profile]));
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
      setConfiguredDrafts(configuredDraftsFromProfiles(nextProfiles));
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

  useEffect(() => {
    if (!open) {
      setExpandedProviderId(null);
      setOpenModelPickerProviderId(null);
      setDraft(null);
      setActiveSection("configure");
      setShowApiKey(false);
      setDeletingProfileId(null);
      setToastMessage(null);
      if (toastTimerRef.current !== null) {
        window.clearTimeout(toastTimerRef.current);
        toastTimerRef.current = null;
      }
      return;
    }
    void loadModelSettings();
  }, [loadModelSettings, open]);

  useEffect(() => {
    return () => {
      if (toastTimerRef.current !== null) {
        window.clearTimeout(toastTimerRef.current);
      }
    };
  }, []);

  function selectProvider(provider: ModelProvider): void {
    const existingProfile = profileByProviderId.get(provider.id);
    setExpandedProviderId((current) =>
      current === provider.id ? null : provider.id,
    );
    setDraft(
      existingProfile
        ? draftFromProfile(existingProfile, provider)
        : draftFromProvider(provider),
    );
    setOpenModelPickerProviderId(null);
    setShowApiKey(false);
    setMessage(null);
  }

  async function saveDraft(): Promise<void> {
    if (!draft) return;
    const provider = providerById.get(draft.providerId);
    if (!provider) return;
    setSaving(true);
    setMessage(null);
    try {
      await runtimeClient.saveModelProfile(toSaveRequest(draft, provider));
      await loadModelSettings();
      onProfilesChanged();
      showToast("模型配置已保存");
    } catch (error) {
      setMessage(
        error instanceof Error ? error.message : "Failed to save model profile",
      );
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
    } catch {
      showToast("连接测试失败：测试请求未完成。");
    } finally {
      setTesting(false);
    }
  }

  const configuredHasChanges = profiles.some((profile) => {
    const next = configuredDrafts[profile.id];
    if (!next) return false;
    return (
      next.enabled !== profile.enabled ||
      next.contextWindow !== contextWindowForProfile(profile) ||
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
    setConfiguredDrafts(configuredDraftsFromProfiles(profiles));
    setMessage(null);
  }

  function updateConfiguredContextWindow(
    profileId: string,
    value: number,
  ): void {
    const nextValue = clampContextWindow(value);
    updateConfiguredDraft(profileId, {
      contextWindow: nextValue,
      contextWindowInput: nextValue.toString(),
    });
  }

  function snapConfiguredContextWindow(profileId: string): void {
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      const nextValue = snapContextWindowAnchor(existing.contextWindow);
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
    const parsed = parseContextWindowInput(value);
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
    setConfiguredDrafts((current) => {
      const existing = current[profileId];
      if (!existing) return current;
      const nextValue =
        parseContextWindowInput(existing.contextWindowInput) ??
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
        next.contextWindow !== contextWindowForProfile(profile) ||
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
      title="模型"
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
        <nav className="settings-dialog-nav" aria-label="Settings sections">
          <button
            type="button"
            className="settings-dialog-nav-item settings-dialog-nav-item-active"
          >
            Models
          </button>
        </nav>

        <div className="model-settings">
          <div className="model-settings-toolbar">
            <h1 className="model-settings-title">模型</h1>
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
          </div>
          {message ? (
            <div className="model-settings-message">{message}</div>
          ) : null}

          {activeSection === "configure" ? (
            <section className="model-provider-list" aria-label="模型配置">
              {providers.map((provider) => {
                const isExpanded = expandedProviderId === provider.id;
                const isModelPickerOpen =
                  openModelPickerProviderId === provider.id;
                const existingProfile = profileByProviderId.get(provider.id);
                const providerDraft =
                  draft?.providerId === provider.id
                    ? draft
                    : existingProfile
                      ? draftFromProfile(existingProfile, provider)
                      : draftFromProvider(provider);
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
                    const profileDraft = configuredDrafts[profile.id] ?? {
                      enabled: profile.enabled,
                      contextWindow: contextWindowForProfile(profile),
                      contextWindowInput:
                        contextWindowForProfile(profile).toString(),
                      maxTokens: maxTokensForProfile(profile),
                      maxTokensInput: maxTokensForProfile(profile).toString(),
                    };
                    const contextStyle = {
                      "--context-fill": `${contextWindowFill(profileDraft.contextWindow)}%`,
                      "--context-anchor": `${contextWindowFill(CONTEXT_WINDOW_ANCHOR)}%`,
                    } as CSSProperties;
                    return (
                      <div className="configured-models-row" key={profile.id}>
                        <div className="configured-model-name">
                          <span className="configured-model-icon">
                            {profile.displayName.slice(0, 1)}
                          </span>
                          <span className="configured-model-copy">
                            <strong>{profile.modelId}</strong>
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
                              max={CONTEXT_WINDOW_MAX}
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
                                  CONTEXT_WINDOW_ANCHOR,
                                )
                              }
                              aria-label={`${profile.modelId} 设置上下文长度为 400K`}
                            >
                              400K
                            </button>
                            <span className="configured-context-scale-label configured-context-scale-end">
                              1M
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
