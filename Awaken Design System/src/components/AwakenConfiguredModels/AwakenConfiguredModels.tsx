import "./AwakenConfiguredModels.css";
import type { CSSProperties } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenSettingsSwitch } from "../AwakenSettingsSwitch";

export type AwakenConfiguredModelItem = {
  id: string;
  displayName: string;
  modelId: string;
  credentialStatus?: string | null;
  credentialTone?: "default" | "unreadable";
  enabled: boolean;
  contextWindow: number;
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
  onReset,
  onSave,
  onUpdateApiKey,
  onDelete,
}: AwakenConfiguredModelsProps) {
  return (
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
        {models.length === 0 ? (
          <div className="configured-models-empty">暂无已配置模型</div>
        ) : models.map((model) => {
          const contextStyle = {
            "--context-fill": `${model.contextFill}%`,
            "--context-anchor": `${model.contextAnchor}%`,
          } as CSSProperties;
          const anyDeleteBusy = Boolean(deletingModelId);
          return (
            <div className="configured-models-row" key={model.id}>
              <div className="configured-model-name">
                <span className="configured-model-icon">{model.displayName.slice(0, 1)}</span>
                <span className="configured-model-copy">
                  <strong>{model.modelId}</strong>
                  {model.credentialStatus ? (
                    <span className={`configured-model-credential-status${model.credentialTone === "unreadable" ? " configured-model-credential-status-unreadable" : ""}`}>
                      {model.credentialStatus}
                    </span>
                  ) : null}
                </span>
              </div>
              <AwakenSettingsSwitch variant="control" label={`${model.modelId} 启用状态`} checked={model.enabled} onCheckedChange={(checked) => onEnabledChange?.(model.id, checked)} />
              <div className="configured-context" style={contextStyle}>
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
              </div>
              <span className="configured-compression-value">{model.compressionValue}</span>
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
              <div className="configured-models-actions">
                <div className="configured-models-save-actions">
                  <AwakenButton variant="default" size="md" disabled={!hasChanges || saving || anyDeleteBusy} onClick={onReset}>取消</AwakenButton>
                  <AwakenButton variant="primary" size="md" loading={saving} disabled={!hasChanges || anyDeleteBusy} onClick={onSave}>保存</AwakenButton>
                </div>
                <AwakenButton variant="default" size="md" disabled={saving || anyDeleteBusy} onClick={() => onUpdateApiKey?.(model.id)}>更新 API key</AwakenButton>
                <AwakenButton className="configured-models-delete" variant="default" size="md" loading={deletingModelId === model.id} disabled={saving || anyDeleteBusy} onClick={() => onDelete?.(model.id)}>删除</AwakenButton>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
