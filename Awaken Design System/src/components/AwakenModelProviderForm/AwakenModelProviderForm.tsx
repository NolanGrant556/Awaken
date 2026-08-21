import "./AwakenModelProviderForm.css";
import { AwakenIcon } from "../AwakenIcon";
import { useEffect, useId, useRef, useState } from "react";
import { CheckCircle2 } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenSelect } from "../AwakenSelect";
import { AwakenSwitch } from "../AwakenSwitch";

type AwakenModelProviderChoiceOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

const EMPTY_MODEL_DISPLAY_NAMES: Record<string, string> = {};

export type AwakenModelProviderTestFeedback = {
  tone: "success" | "error";
  message: string;
};

export type AwakenModelProviderFormProps = {
  providerId: string;
  presentation?: "inline" | "modal";
  modelMode: "custom" | "editable" | "endpoint" | "select";
  models: string[];
  modelDisplayNames?: Record<string, string>;
  modelId: string;
  protocol?: string;
  protocolOptions?: Array<{ value: string; label: string }>;
  authScheme?: string;
  authSchemeOptions?: Array<{ value: string; label: string }>;
  showAuthScheme?: boolean;
  baseURL: string;
  showBaseURL?: boolean;
  baseURLReadOnly?: boolean;
  apiKey: string;
  managementApiKey?: string;
  maskedManagementApiKey?: string;
  showManagementApiKey?: boolean;
  apiKeyPlaceholder: string;
  apiDocsURL: string;
  templateNote?: string | null;
  showApiKey?: boolean;
  showVisionCapability?: boolean;
  supportsVision?: boolean;
  saving?: boolean;
  testing?: boolean;
  testFeedback?: AwakenModelProviderTestFeedback | null;
  showCatalogRefresh?: boolean;
  refreshingCatalog?: boolean;
  catalogStatus?: string;
  accountStatus?: string;
  showAccountActions?: boolean;
  refreshingAccount?: boolean;
  deletingAccount?: boolean;
  onModelChange: (modelId: string) => void;
  onProtocolChange?: (protocol: string) => void;
  onAuthSchemeChange?: (authScheme: string) => void;
  onBaseURLChange: (baseURL: string) => void;
  onApiKeyChange: (apiKey: string) => void;
  onManagementApiKeyChange?: (apiKey: string) => void;
  onShowApiKeyChange?: (show: boolean) => void;
  onSupportsVisionChange?: (supported: boolean) => void;
  onTest?: () => void;
  onRefreshCatalog?: () => void;
  onRefreshAccount?: () => void;
  onDeleteAccount?: () => void;
  onCancel?: () => void;
  onSubmit?: () => void;
};

function authHeaderExample(authScheme: string): string {
  switch (authScheme) {
    case "bearer":
      return "Authorization: Bearer <API_KEY>";
    case "x-api-key":
      return "x-api-key: <API_KEY>";
    case "api-key":
      return "api-key: <API_KEY>";
    case "x-goog-api-key":
      return "x-goog-api-key: <API_KEY>";
    default:
      return "无鉴权（不发送认证 Header）";
  }
}

function modelDisplayValue(
  modelDisplayNames: Record<string, string>,
  modelId: string,
): string {
  return modelDisplayNames[modelId] ?? modelId;
}

export function AwakenModelProviderForm({
  providerId,
  presentation = "inline",
  modelMode,
  models,
  modelDisplayNames = EMPTY_MODEL_DISPLAY_NAMES,
  modelId,
  protocol = "openai_chat",
  protocolOptions = [{ value: "openai_chat", label: "OpenAI Chat Completions" }],
  authScheme = "bearer",
  authSchemeOptions = [{ value: "bearer", label: "Authorization: Bearer" }],
  showAuthScheme = false,
  baseURL,
  showBaseURL = false,
  baseURLReadOnly = false,
  apiKey,
  managementApiKey = "",
  maskedManagementApiKey,
  showManagementApiKey = false,
  apiKeyPlaceholder,
  apiDocsURL,
  templateNote,
  showApiKey = false,
  showVisionCapability = false,
  supportsVision = false,
  saving = false,
  testing = false,
  testFeedback = null,
  showCatalogRefresh = false,
  refreshingCatalog = false,
  catalogStatus,
  accountStatus,
  showAccountActions = false,
  refreshingAccount = false,
  deletingAccount = false,
  onModelChange,
  onProtocolChange,
  onAuthSchemeChange,
  onBaseURLChange,
  onApiKeyChange,
  onManagementApiKeyChange,
  onShowApiKeyChange,
  onSupportsVisionChange,
  onTest,
  onRefreshCatalog,
  onRefreshAccount,
  onDeleteAccount,
  onCancel,
  onSubmit,
}: AwakenModelProviderFormProps) {
  const [modelCatalogOpen, setModelCatalogOpen] = useState(false);
  const [modelInputValue, setModelInputValue] = useState(() =>
    modelDisplayValue(modelDisplayNames, modelId),
  );
  const modelCatalogRef = useRef<HTMLDivElement>(null);
  const hasDraftApiKey = authScheme === "none" || apiKey.trim().length > 0;
  const hasTestTarget =
    modelId.trim().length > 0 && baseURL.trim().length > 0;
  const canTest = hasDraftApiKey && hasTestTarget;
  const fieldId = useId();
  const modelLabelId = `${fieldId}-model-label`;
  const modelInputId = `${fieldId}-model-input`;
  const modelCatalogId = `${fieldId}-model-catalog`;
  const authHeaderExampleId = `${fieldId}-auth-header-example`;

  useEffect(() => {
    setModelInputValue(modelDisplayValue(modelDisplayNames, modelId));
  }, [modelId, modelDisplayNames]);

  useEffect(() => {
    if (!modelCatalogOpen) return;
    const closeOutside = (event: PointerEvent) => {
      if (!modelCatalogRef.current?.contains(event.target as Node)) {
        setModelCatalogOpen(false);
      }
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [modelCatalogOpen]);

  function renderChoiceControl(
    labelId: string,
    value: string,
    options: readonly AwakenModelProviderChoiceOption[],
    onValueChange?: (nextValue: string) => void,
  ) {
    if (options.length > 1) {
      return (
        <AwakenSelect
          value={value}
          options={options}
          labelledBy={labelId}
          onValueChange={(nextValue) => onValueChange?.(nextValue)}
        />
      );
    }

    return (
      <input
        className="model-provider-readonly-choice"
        type="text"
        value={options[0]?.label ?? value}
        readOnly
        aria-labelledby={labelId}
      />
    );
  }

  return (
    <>
      {templateNote ? (
        <div className="model-provider-template-note">{templateNote}</div>
      ) : null}

      <div className="model-provider-field">
        <label id={`${fieldId}-protocol-label`}>接口协议</label>
        {renderChoiceControl(
          `${fieldId}-protocol-label`,
          protocol,
          protocolOptions,
          onProtocolChange,
        )}
      </div>

      {showManagementApiKey ? (
        <div className="model-provider-field">
          <label htmlFor={`${fieldId}-management-api-key`}>组织用量管理密钥（可选）</label>
          <input
            id={`${fieldId}-management-api-key`}
            type="password"
            value={managementApiKey}
            placeholder={maskedManagementApiKey ? `已保存 ${maskedManagementApiKey}；留空保持不变` : "仅用于组织用量与成本 API，不参与推理"}
            onChange={(event) => onManagementApiKeyChange?.(event.target.value)}
          />
        </div>
      ) : null}

      {modelMode === "custom" ? (
        <div className="model-provider-field">
          <label id={modelLabelId} htmlFor={modelInputId}>模型</label>
          <input
            id={modelInputId}
            type="text"
            value={modelId}
            placeholder="输入模型 ID"
            autoComplete="off"
            onChange={(event) => onModelChange(event.target.value)}
          />
        </div>
      ) : modelMode === "endpoint" || modelMode === "editable" ? (
        <div className="model-provider-field">
          <label id={modelLabelId} htmlFor={modelInputId}>
            {modelMode === "endpoint" ? "模型 / 推理接入点" : "模型"}
          </label>
          <div className="model-provider-model-combobox" ref={modelCatalogRef}>
            <input
              id={modelInputId}
              role="combobox"
              value={modelInputValue}
              placeholder={
                modelMode === "endpoint"
                  ? "选择或输入 Model ID / Endpoint ID"
                  : "选择或输入模型 ID"
              }
              autoComplete="off"
              aria-labelledby={modelLabelId}
              aria-controls={modelCatalogId}
              aria-expanded={modelCatalogOpen}
              aria-autocomplete="list"
              onFocus={() => setModelCatalogOpen(true)}
              onKeyDown={(event) => {
                if (event.key === "Escape") setModelCatalogOpen(false);
                if (event.key === "ArrowDown") setModelCatalogOpen(true);
              }}
              onChange={(event) => {
                setModelInputValue(event.target.value);
                onModelChange(event.target.value);
                setModelCatalogOpen(true);
              }}
            />
            <button
              type="button"
              className="model-provider-model-combobox__toggle"
              aria-label={modelCatalogOpen ? "收起模型目录" : "展开模型目录"}
              aria-controls={modelCatalogId}
              aria-expanded={modelCatalogOpen}
              onClick={() => setModelCatalogOpen((open) => !open)}
            >
              <AwakenIcon concept="dropdown" size={16} />
            </button>
            {modelCatalogOpen && models.length > 0 ? (
              <div
                id={modelCatalogId}
                className="model-provider-model-combobox__catalog"
                role="listbox"
                aria-labelledby={modelLabelId}
              >
                {models.map((model) => (
                  <button
                    key={model}
                    type="button"
                    className="model-provider-model-combobox__option"
                    role="option"
                    aria-selected={model === modelId}
                    onClick={() => {
                      setModelInputValue(modelDisplayValue(modelDisplayNames, model));
                      onModelChange(model);
                      setModelCatalogOpen(false);
                    }}
                  >
                    <span>{modelDisplayValue(modelDisplayNames, model)}</span>
                    {model === modelId ? <AwakenIcon concept="success" size={16} /> : null}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      ) : (
        <div className="model-provider-field">
          <label id={modelLabelId}>模型</label>
          {renderChoiceControl(
            modelLabelId,
            modelId,
            models.map((model) => ({
              value: model,
              label: modelDisplayValue(modelDisplayNames, model),
            })),
            onModelChange,
          )}
        </div>
      )}

      {showAuthScheme ? (
        <div className="model-provider-field">
          <label id={`${fieldId}-auth-label`}>API 密钥发送方式</label>
          {renderChoiceControl(
            `${fieldId}-auth-label`,
            authScheme,
            authSchemeOptions,
            onAuthSchemeChange,
          )}
          <p id={authHeaderExampleId} className="model-provider-auth-header-example">
            当前 Header：
            <code>{authHeaderExample(authScheme)}</code>
          </p>
        </div>
      ) : null}

      <div className="model-provider-field">
        <label htmlFor={`${fieldId}-api-key`}>{authScheme === "none" ? "API 密钥（不需要）" : "API 密钥"}</label>
        <div className="model-provider-secret-field">
          <input
            id={`${fieldId}-api-key`}
            type={showApiKey ? "text" : "password"}
            value={apiKey}
            placeholder={apiKeyPlaceholder}
            onChange={(event) => onApiKeyChange(event.target.value)}
            disabled={authScheme === "none"}
          />
          <button
            type="button"
            className="model-provider-secret-toggle"
            aria-label={showApiKey ? "隐藏 API 密钥" : "显示 API 密钥"}
            aria-pressed={showApiKey}
            onClick={() => onShowApiKeyChange?.(!showApiKey)}
          >
            {showApiKey ? <AwakenIcon concept="conceal" size={16} /> : <AwakenIcon concept="reveal" size={16} />}
          </button>
        </div>
      </div>

      {showVisionCapability ? (
        <div className="model-provider-capability-toggle">
          <div>
            <strong>视觉多模态</strong>
            <p>开启后允许发送图片，并可把 PDF 页面渲染为图片读取；不代表接口支持原生 PDF。</p>
          </div>
          <AwakenSwitch
            checked={supportsVision}
            label="视觉多模态"
            size="compact"
            onCheckedChange={onSupportsVisionChange}
          />
        </div>
      ) : null}

      {showBaseURL ? (
        <div className="model-provider-field">
          <label htmlFor={`${fieldId}-base-url`}>Base URL</label>
          <input
            id={`${fieldId}-base-url`}
            readOnly={baseURLReadOnly}
            value={baseURL}
            onChange={(event) => onBaseURLChange(event.target.value)}
          />
        </div>
      ) : null}

      {showCatalogRefresh || showAccountActions ? (
        <div className="model-provider-verification-actions">
          {showCatalogRefresh ? (
            <AwakenButton
              variant="default"
              size="md"
              loading={refreshingCatalog}
              disabled={saving || deletingAccount || refreshingAccount || !hasDraftApiKey}
              onClick={onRefreshCatalog}
            >
              刷新模型目录
            </AwakenButton>
          ) : null}
          {showAccountActions ? (
            <>
              <AwakenButton variant="default" size="md" loading={refreshingAccount} disabled={saving || deletingAccount} onClick={onRefreshAccount}>刷新模型目录</AwakenButton>
              <AwakenButton variant="default" size="md" loading={deletingAccount} disabled={saving || refreshingAccount} onClick={onDeleteAccount}>删除账户</AwakenButton>
            </>
          ) : null}
        </div>
      ) : null}

      {catalogStatus ? (
        <p className="model-provider-status-note" role="status" aria-live="polite">
          {catalogStatus}
        </p>
      ) : null}

      {presentation === "modal" ? (
        <div className="model-provider-test-block">
          <AwakenButton
            className="model-provider-primary-test"
            variant="default"
            size="md"
            loading={testing}
            disabled={saving || !canTest}
            onClick={onTest}
          >
            测试连接
          </AwakenButton>
          {testFeedback ? (
            <div
              className={`model-provider-test-feedback model-provider-test-feedback--${testFeedback.tone}`}
              role={testFeedback.tone === "error" ? "alert" : "status"}
              aria-live={testFeedback.tone === "error" ? "assertive" : "polite"}
            >
              {testFeedback.tone === "success" ? (
                <CheckCircle2 size={16} strokeWidth={2} aria-hidden="true" />
              ) : (
                <AwakenIcon concept="error" size={16} />
              )}
              <span>{testFeedback.message}</span>
            </div>
          ) : null}
        </div>
      ) : null}

      <div className="model-provider-form-footer">
        <div className="model-provider-link-row">
          <a href={apiDocsURL} target="_blank" rel="noreferrer"><AwakenIcon concept="api-key" size={14} />获取 API 密钥</a>
          {accountStatus ? <span className="model-provider-account-status">{accountStatus}</span> : null}
        </div>
        <div className="model-provider-form-actions">
          {presentation === "modal" && onCancel ? (
            <AwakenButton variant="default" size="md" disabled={saving} onClick={onCancel}>取消</AwakenButton>
          ) : null}
          <AwakenButton variant="primary" size="md" loading={saving} disabled={testing} onClick={onSubmit}>保存连接</AwakenButton>
        </div>
      </div>
    </>
  );
}
