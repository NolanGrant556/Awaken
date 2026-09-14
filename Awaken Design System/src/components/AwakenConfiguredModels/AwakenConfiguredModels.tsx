import "./AwakenConfiguredModels.css";
import type { CSSProperties } from "react";
import { AwakenButton } from "../AwakenButton";
import {
  AwakenProviderLogo,
  type AwakenProviderBrand,
} from "../AwakenProviderLogo";
import { AwakenSettingsSwitch } from "../AwakenSettingsSwitch";

export type AwakenConfiguredModelItem = {
  id: string;
  displayName: string;
  brand?: AwakenProviderBrand;
  modelId: string;
  protocolLabel?: string;
  isDefault?: boolean;
  credentialStatus?: string | null;
  credentialTone?: "default" | "unreadable";
  enabled: boolean;
  contextWindow: number;
  contextUnknown?: boolean;
  contextWindowInput: string;
  contextWindowMin: number;
  contextWindowMax: number;
  contextWindowStep?: number;
  contextFill: number;
  contextAnchor: number;
  contextAnchorValue: number;
  contextAnchorLabel: string;
  contextStartLabel?: string;
  contextEndLabel: string;
  compressionValue: number | string;
  maxTokensInput: string;
};

export type AwakenConfiguredModelsProps = {
  models: AwakenConfiguredModelItem[];
  hasChanges?: boolean;
  saving?: boolean;
  deletingModelId?: string | null;
  onEnabledChange?: (id: string, checked: boolean) => void;
  onContextRangeChange?: (id: string, value: number) => void;
  onContextRangeCommit?: (id: string) => void;
  onContextInputChange?: (id: string, value: string) => void;
  onContextInputCommit?: (id: string) => void;
  onContextAnchorSelect?: (id: string) => void;
  onMaxTokensInputChange?: (id: string, value: string) => void;
  onMaxTokensInputCommit?: (id: string) => void;
  onSetDefault?: (id: string) => void;
  onReset?: () => void;
  onSave?: () => void;
  onUpdateApiKey?: (id: string) => void;
  onDelete?: (id: string) => void;
};

export function AwakenConfiguredModels({
  models,
  hasChanges = false,
  saving = false,
  deletingModelId = null,
  onEnabledChange,
  onContextRangeChange,
  onContextRangeCommit,
  onContextInputChange,
  onContextInputCommit,
  onContextAnchorSelect,
  onMaxTokensInputChange,
  onMaxTokensInputCommit,
  onSetDefault,
  onReset,
  onSave,
  onUpdateApiKey,
  onDelete,
}: AwakenConfiguredModelsProps) {
  return (
    <section className="configured-models" aria-label="LLM 设置">
      {models.length === 0 ? (
        <div className="configured-models-empty">
          <strong>暂无已配置模型</strong>
          <span>请先完成连接配置，再添加这一接入类型的模型。</span>
        </div>
      ) : (
        <div className="configured-model-cards">
          {models.map((model) => {
          const contextStyle = {
            "--context-fill": `${model.contextFill}%`,
            "--context-anchor": `${model.contextAnchor}%`,
          } as CSSProperties;
          const anyDeleteBusy = Boolean(deletingModelId);
          const contextSummary = model.contextWindowInput.trim()
            ? `${model.contextWindowInput.trim()} context`
            : "上下文未知";
          const outputSummary = model.maxTokensInput.trim()
            ? `${model.maxTokensInput.trim()} output`
            : "output 未知";
          return (
            <article className="configured-model-card" key={model.id}>
              <header className="configured-model-card__header">
                <div className="configured-model-name">
                <span className="configured-model-icon">
                  <AwakenProviderLogo brand={model.brand} name={model.displayName} />
                </span>
                <span className="configured-model-copy">
                  <strong title={`${model.displayName}：${model.modelId}`}>
                    {model.displayName}：{model.modelId}
                  </strong>
                  {model.protocolLabel ? (
                    <span className="configured-model-credential-status">{model.protocolLabel}</span>
                  ) : null}
                  {model.credentialStatus ? (
                    <span className={`configured-model-credential-status${model.credentialTone === "unreadable" ? " configured-model-credential-status-unreadable" : ""}`}>
                      {model.credentialStatus}
                    </span>
                  ) : null}
                </span>
              </div>
                <div className="configured-model-card__controls">
                  <button
                    type="button"
                    className={`configured-model-default${model.isDefault ? " is-default" : ""}`}
                    disabled={model.isDefault || hasChanges || saving || anyDeleteBusy}
                    onClick={() => onSetDefault?.(model.id)}
                  >
                    {model.isDefault ? "默认模型" : "设为默认"}
                  </button>
                  <AwakenSettingsSwitch
                    variant="control"
                    label={`${model.modelId} 启用状态`}
                    checked={model.enabled}
                    onCheckedChange={(checked) => onEnabledChange?.(model.id, checked)}
                  />
                </div>
              </header>
              <details className="configured-model-card__details">
                <summary>
                  <span>LLM 参数</span>
                  <span>{contextSummary} · {outputSummary}</span>
                </summary>
                <div className="configured-model-card__body">
                  <div className="configured-parameter configured-parameter--context">
                    <span className="configured-parameter__label">上下文长度</span>
                    {model.contextUnknown ? (
                      <div className="configured-context-manual">
                        <input
                          className="configured-context-input"
                          type="text"
                          inputMode="numeric"
                          pattern="[0-9]*"
                          value={model.contextWindowInput}
                          placeholder="手动输入上下文长度"
                          onChange={(event) => onContextInputChange?.(model.id, event.target.value)}
                          onBlur={() => onContextInputCommit?.(model.id)}
                          aria-label={`${model.modelId} 手动上下文长度`}
                        />
                        <p className="configured-context-manual-note">
                          当前目录没有返回上下文窗口，可手动输入后保存。
                        </p>
                      </div>
                    ) : <div className="configured-context" style={contextStyle}>
                      <div className="configured-context-main">
                        <input
                          className="configured-context-range"
                          type="range"
                          min={model.contextWindowMin}
                          max={model.contextWindowMax}
                          step={model.contextWindowStep ?? 1}
                          value={model.contextWindow}
                          onChange={(event) => onContextRangeChange?.(model.id, Number(event.target.value))}
                          onPointerUp={() => onContextRangeCommit?.(model.id)}
                          onKeyUp={() => onContextRangeCommit?.(model.id)}
                          onBlur={() => onContextRangeCommit?.(model.id)}
                          aria-label={`${model.modelId} 上下文长度`}
                        />
                        <input
                          className="configured-context-input"
                          type="text"
                          inputMode="numeric"
                          pattern="[0-9]*"
                          value={model.contextWindowInput}
                          onChange={(event) => onContextInputChange?.(model.id, event.target.value)}
                          onBlur={() => onContextInputCommit?.(model.id)}
                          aria-label={`${model.modelId} 精确上下文长度`}
                        />
                      </div>
                      <div className="configured-context-scale">
                        <span className="configured-context-scale-label configured-context-scale-start">{model.contextStartLabel ?? "8K"}</span>
                        <button type="button" className="configured-context-anchor" onClick={() => onContextAnchorSelect?.(model.id)} aria-label={`${model.modelId} 设置上下文长度为 ${model.contextAnchorValue}`}>
                          {model.contextAnchorLabel}
                        </button>
                        <span className="configured-context-scale-label configured-context-scale-end">{model.contextEndLabel}</span>
                      </div>
                    </div>}
                  </div>
                  <div className="configured-parameter configured-parameter--compression">
                    <span className="configured-parameter__label">自动压缩阈值</span>
                    <strong className="configured-compression-value">{model.compressionValue}</strong>
                  </div>
                  <label className="configured-parameter">
                    <span className="configured-parameter__label">输出上限</span>
                    <input
                      className="configured-token-input"
                      type="text"
                      inputMode="numeric"
                      pattern="[0-9]*"
                      value={model.maxTokensInput}
                      onChange={(event) => onMaxTokensInputChange?.(model.id, event.target.value)}
                      onBlur={() => onMaxTokensInputCommit?.(model.id)}
                      aria-label={`${model.modelId} 输出 token 上限`}
                    />
                  </label>
                </div>
                <footer className="configured-model-card__footer">
                  <div className="configured-models-save-actions">
                    <AwakenButton variant="default" size="sm" disabled={!hasChanges || saving || anyDeleteBusy} onClick={onReset}>取消修改</AwakenButton>
                    <AwakenButton variant="primary" size="sm" loading={saving} disabled={!hasChanges || anyDeleteBusy} onClick={onSave}>保存 LLM 设置</AwakenButton>
                  </div>
                  <div className="configured-models-actions">
                    <AwakenButton variant="default" size="sm" disabled={saving || anyDeleteBusy} onClick={() => onUpdateApiKey?.(model.id)}>连接设置</AwakenButton>
                    <AwakenButton className="configured-models-delete" variant="default" size="sm" loading={deletingModelId === model.id} disabled={saving || anyDeleteBusy} onClick={() => onDelete?.(model.id)}>删除模型</AwakenButton>
                  </div>
                </footer>
              </details>
            </article>
          );
        })}
        </div>
      )}
    </section>
  );
}
