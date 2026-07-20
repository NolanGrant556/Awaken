import "./AwakenPetsSettings.css";
import type { ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import { AwakenButton } from "../AwakenButton";
import { AwakenTooltip } from "../AwakenTooltip";

export type AwakenPetSettingsItem = {
  id: string;
  displayName: string;
  available: boolean;
  selected?: boolean;
  warning?: string;
  preview?: ReactNode;
};

export type AwakenPetsSettingsProps = {
  overlayOpen?: boolean;
  hasSelection?: boolean;
  selectedDisplayName?: string;
  selectedDescription?: string;
  selectedPreview?: ReactNode;
  autoImport?: boolean;
  loading?: boolean;
  message?: string | null;
  pets: AwakenPetSettingsItem[];
  importErrors?: Array<{ key: string | number; source: string; error: string }>;
  onToggleOverlay?: () => void;
  onRefresh?: () => void;
  onOpenAwakenDirectory?: () => void;
  onOpenCodexDirectory?: () => void;
  onToggleAutoImport?: () => void;
  onSelectPet?: (id: string) => void;
};

export function AwakenPetsSettings({
  overlayOpen = false,
  hasSelection = false,
  selectedDisplayName = "未选择",
  selectedDescription,
  selectedPreview,
  autoImport = true,
  loading = false,
  message,
  pets,
  importErrors = [],
  onToggleOverlay,
  onRefresh,
  onOpenAwakenDirectory,
  onOpenCodexDirectory,
  onToggleAutoImport,
  onSelectPet,
}: AwakenPetsSettingsProps) {
  return (
    <section className="pets-settings" aria-label="宠物">
      <div className="pets-settings-toolbar">
        <div className="pets-settings-actions">
          <AwakenTooltip label={!hasSelection ? "请先选择一只宠物" : overlayOpen ? "收起桌面宠物浮窗" : "打开桌面宠物浮窗"} placement="top">
            <AwakenButton variant="default" size="md" onClick={onToggleOverlay} disabled={!hasSelection}>{overlayOpen ? "收起宠物" : "唤醒宠物"}</AwakenButton>
          </AwakenTooltip>
        </div>
      </div>
      {message ? <div className="pets-settings-message">{message}</div> : null}
      <div className="pets-settings-preview">
        <div className="pets-settings-preview-sprite">{selectedPreview ?? <div className="pets-settings-preview-placeholder">?</div>}</div>
        <div className="pets-settings-preview-meta"><span className="pets-settings-preview-label">当前选择的宠物</span><strong>{selectedDisplayName}</strong><span>{selectedDescription}</span></div>
      </div>
      <div className="pets-settings-section">
        <div className="pets-settings-section-header">
          <h2>已导入宠物</h2>
          <div className="pets-settings-section-actions">
            <AwakenButton variant="ghost" size="sm" onClick={onRefresh} disabled={loading} icon={<RefreshCw size={14} />}>刷新 Codex 宠物</AwakenButton>
            <AwakenButton variant="ghost" size="sm" onClick={onOpenAwakenDirectory}>打开 Awaken 目录</AwakenButton>
            <AwakenButton variant="ghost" size="sm" onClick={onOpenCodexDirectory}>打开 Codex 目录</AwakenButton>
          </div>
        </div>
        <label className="pets-settings-toggle"><input type="checkbox" checked={autoImport} readOnly={!onToggleAutoImport} onChange={onToggleAutoImport} /><span>启动时自动导入 Codex 自定义宠物</span></label>
        <div className="pets-settings-list" role="radiogroup" aria-label="选择宠物">
          {pets.length === 0 ? <div className="pets-settings-empty">暂无可用宠物</div> : pets.map((pet) => (
            <label key={pet.id} className={`pets-settings-item${pet.selected ? " pets-settings-item-active" : ""}${!pet.available ? " pets-settings-item-missing" : ""}`}>
              <input type="radio" name="selected-pet" value={pet.id} checked={pet.selected ?? false} readOnly={!onSelectPet} onChange={() => onSelectPet?.(pet.id)} disabled={!pet.available} />
              {pet.preview ?? <div className="pet-static-preview pet-static-preview-placeholder" />}
              <span className="pets-settings-item-name">{pet.displayName}</span>
              {pet.warning ? <span className="pets-settings-item-warning">{pet.warning}</span> : null}
            </label>
          ))}
        </div>
      </div>
      {importErrors.length ? <div className="pets-settings-errors"><h3>导入错误</h3><ul>{importErrors.map((item) => <li key={item.key}>{item.source}: {item.error}</li>)}</ul></div> : null}
    </section>
  );
}
