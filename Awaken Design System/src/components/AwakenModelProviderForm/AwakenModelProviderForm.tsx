import "./AwakenModelProviderForm.css";
import { useId, useState } from "react";
import {
  Check,
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  Eye,
  EyeOff,
  KeyRound,
} from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenSelect } from "../AwakenSelect";
import { AwakenSettingsSwitch } from "../AwakenSettingsSwitch";

export type AwakenModelProviderCapability =
  | "supportsVision"
  | "supportsNativePDF"
  | "supportsVideo";

export type AwakenModelProviderTestFeedback = {
  tone: "success" | "error";
  message: string;
};

export type AwakenModelProviderFormProps = {
  providerId: string;
  presentation?: "inline" | "modal";
  modelMode: "custom" | "endpoint" | "select";
  models: string[];
  modelId: string;
  baseURL: string;
  baseURLReadOnly?: boolean;
  apiKey: string;
  apiKeyPlaceholder: string;
  apiDocsURL: string;
  templateNote?: string | null;
  showApiKey?: boolean;
  editableCapabilities?: boolean;
  supportsVision?: boolean;
  supportsNativePDF?: boolean;
  supportsVideo?: boolean;
  saving?: boolean;
  testing?: boolean;
  testFeedback?: AwakenModelProviderTestFeedback | null;
  testingReasoning?: boolean;
  showReasoningTest?: boolean;
  accountStatus?: string;
  showAccountActions?: boolean;
  refreshingAccount?: boolean;
  deletingAccount?: boolean;
  onModelChange: (modelId: string) => void;
  onBaseURLChange: (baseURL: string) => void;
  onApiKeyChange: (apiKey: string) => void;
  onShowApiKeyChange?: (show: boolean) => void;
  onCapabilityChange?: (
    capability: AwakenModelProviderCapability,
    checked: boolean,
  ) => void;
  onTest?: () => void;
  onTestReasoning?: () => void;
  onRefreshAccount?: () => void;
  onDeleteAccount?: () => void;
  onCancel?: () => void;
  onSubmit?: () => void;
};

export function AwakenModelProviderForm({
  providerId,
  presentation = "inline",
  modelMode,
  models,
  modelId,
  baseURL,
  baseURLReadOnly = false,
  apiKey,
  apiKeyPlaceholder,
  apiDocsURL,
  templateNote,
  showApiKey = false,
  editableCapabilities = false,
  supportsVision = false,
  supportsNativePDF = false,
  supportsVideo = false,
  saving = false,
  testing = false,
  testFeedback = null,
  testingReasoning = false,
  showReasoningTest = false,
  accountStatus,
  showAccountActions = false,
  refreshingAccount = false,
  deletingAccount = false,
  onModelChange,
  onBaseURLChange,
  onApiKeyChange,
  onShowApiKeyChange,
  onCapabilityChange,
  onTest,
  onTestReasoning,
  onRefreshAccount,
  onDeleteAccount,
  onCancel,
  onSubmit,
}: AwakenModelProviderFormProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const fieldId = useId();
  const modelLabelId = `${fieldId}-model-label`;
  const modelInputId = `${fieldId}-model-input`;
  const modelSuggestionsId = `${fieldId}-model-suggestions`;

  function chooseModel(nextModelId: string): void {
    onModelChange(nextModelId);
    setPickerOpen(false);
  }

  return (
    <>
      {templateNote ? (
        <div className="model-provider-template-note">{templateNote}</div>
      ) : null}

      {modelMode === "custom" ? (
        <div className="model-provider-field">
          <label id={modelLabelId} htmlFor={modelInputId}>模型</label>
          <div className="model-custom-picker">
            <input
              id={modelInputId}
              value={modelId}
              aria-expanded={pickerOpen}
              aria-controls={modelSuggestionsId}
              onClick={() => setPickerOpen((open) => !open)}
              onChange={(event) => onModelChange(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape") setPickerOpen(false);
              }}
            />
            <button
              type="button"
              className="model-custom-picker-trigger"
              aria-haspopup="listbox"
              aria-expanded={pickerOpen}
              aria-label="展开模型建议"
              onClick={() => setPickerOpen((open) => !open)}
            >
              <ChevronDown className="aw-select__chevron" size={16} strokeWidth={1.9} />
            </button>
            {pickerOpen ? (
              <div
                id={modelSuggestionsId}
                className="aw-select__menu"
                role="listbox"
                aria-labelledby={modelLabelId}
              >
                {models.map((model) => (
                  <button
                    key={model}
                    type="button"
                    className="aw-select__option"
                    role="option"
                    aria-selected={modelId === model}
                    onClick={() => chooseModel(model)}
                  >
                    <span>{model}</span>
                    {modelId === model ? (
                      <Check size={15} strokeWidth={2} aria-hidden="true" />
                    ) : null}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </div>
      ) : modelMode === "endpoint" ? (
        <label>
          <span>模型</span>
          <input
            list={`${providerId}-models`}
            value={modelId}
            onChange={(event) => onModelChange(event.target.value)}
          />
          <datalist id={`${providerId}-models`}>
            {models.map((model) => <option key={model} value={model} />)}
          </datalist>
        </label>
      ) : (
        <div className="model-provider-field">
          <label id={modelLabelId}>模型</label>
          <AwakenSelect
            value={modelId}
            options={models.map((model) => ({ value: model, label: model }))}
            labelledBy={modelLabelId}
            onValueChange={onModelChange}
          />
        </div>
      )}

      <div className="model-provider-field">
        <label htmlFor={`${fieldId}-api-key`}>API 密钥</label>
        <div className="model-provider-secret-field">
          <input
            id={`${fieldId}-api-key`}
            type={showApiKey ? "text" : "password"}
            value={apiKey}
            placeholder={apiKeyPlaceholder}
            onChange={(event) => onApiKeyChange(event.target.value)}
          />
          <button
            type="button"
            className="model-provider-secret-toggle"
            aria-label={showApiKey ? "隐藏 API 密钥" : "显示 API 密钥"}
            aria-pressed={showApiKey}
            onClick={() => onShowApiKeyChange?.(!showApiKey)}
          >
            {showApiKey ? <EyeOff size={15} strokeWidth={1.9} /> : <Eye size={15} strokeWidth={1.9} />}
          </button>
        </div>
      </div>

      {presentation === "modal" ? (
        <div className="model-provider-test-block">
          <AwakenButton
            className="model-provider-primary-test"
            variant="default"
            size="md"
            loading={testing}
            disabled={saving}
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
                <CircleAlert size={16} strokeWidth={2} aria-hidden="true" />
              )}
              <span>{testFeedback.message}</span>
            </div>
          ) : null}
        </div>
      ) : null}

      <details className={`model-provider-advanced${presentation === "inline" ? " model-provider-advanced--open" : ""}`} open={presentation === "inline" ? true : undefined}>
        {presentation === "modal" ? (
          <summary>
            <span>高级设置</span>
            <span>Base URL 与输入能力</span>
          </summary>
        ) : null}
        <div className="model-provider-advanced__body">
          <label>
            <span>Base URL</span>
            <input readOnly={baseURLReadOnly} value={baseURL} onChange={(event) => onBaseURLChange(event.target.value)} />
          </label>

          {editableCapabilities ? (
            <div className="model-provider-field">
              <label>输入能力</label>
              <div className="model-provider-capability-grid">
                <label className="model-provider-capability-card">
                  <span className="model-provider-capability-copy"><strong>支持图片输入</strong><span>用于输入框图片粘贴、预览与发送 gating。</span></span>
                  <AwakenSettingsSwitch variant="control" label={`${modelId} 支持图片输入`} checked={supportsVision} onCheckedChange={(checked) => onCapabilityChange?.("supportsVision", checked)} />
                </label>
                <label className="model-provider-capability-card">
                  <span className="model-provider-capability-copy"><strong>支持原生 PDF 输入</strong><span>开启后 Read 会直接向模型发送 PDF；关闭时使用视觉模型的 Poppler 页面渲染回退。</span></span>
                  <AwakenSettingsSwitch variant="control" label={`${modelId} 支持原生 PDF 输入`} checked={supportsNativePDF} onCheckedChange={(checked) => onCapabilityChange?.("supportsNativePDF", checked)} />
                </label>
                <label className="model-provider-capability-card">
                  <span className="model-provider-capability-copy"><strong>支持视频输入</strong><span>当前只保存能力位，输入框暂不开放视频上传。</span></span>
                  <AwakenSettingsSwitch variant="control" label={`${modelId} 支持视频输入`} checked={supportsVideo} disabled={!supportsVision} onCheckedChange={(checked) => onCapabilityChange?.("supportsVideo", checked)} />
                </label>
              </div>
            </div>
          ) : null}

          {presentation === "inline" || showReasoningTest || showAccountActions ? (
            <div className="model-provider-verification-actions">
              {presentation === "inline" ? (
                <AwakenButton variant="default" size="md" loading={testing} disabled={saving} onClick={onTest}>测试连接</AwakenButton>
              ) : null}
              {showReasoningTest ? (
                <AwakenButton variant="default" size="md" loading={testingReasoning} disabled={saving || testing} onClick={onTestReasoning}>验证 effort（少量消耗）</AwakenButton>
              ) : null}
              {showAccountActions ? (
                <>
                  <AwakenButton variant="default" size="md" loading={refreshingAccount} disabled={saving || deletingAccount} onClick={onRefreshAccount}>刷新模型目录</AwakenButton>
                  <AwakenButton variant="default" size="md" loading={deletingAccount} disabled={saving || refreshingAccount} onClick={onDeleteAccount}>删除账户</AwakenButton>
                </>
              ) : null}
            </div>
          ) : null}
        </div>
      </details>

      <div className="model-provider-form-footer">
        <div className="model-provider-link-row">
          <a href={apiDocsURL} target="_blank" rel="noreferrer"><KeyRound size={14} />获取 API 密钥</a>
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
