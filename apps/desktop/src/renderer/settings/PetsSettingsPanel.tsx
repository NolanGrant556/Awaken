import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import {
  AwakenButton as Button,
  AwakenTooltip as Tooltip,
} from "@awaken/design-system";
import type { RuntimeClient } from "../runtime/client";
import type { PetImportReport, PetSettings, PetSummary } from "../../shared/pets";
import { PetSprite } from "../pet-overlay/PetSprite";

type PetsSettingsPanelProps = {
  runtimeClient: RuntimeClient;
};

export function PetsSettingsPanel({ runtimeClient }: PetsSettingsPanelProps) {
  const [pets, setPets] = useState<PetSummary[]>([]);
  const [settings, setSettings] = useState<PetSettings | null>(null);
  const [importReport, setImportReport] = useState<PetImportReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [nextPets, nextSettings] = await Promise.all([
        runtimeClient.listPets(),
        runtimeClient.getPetSettings(),
      ]);
      setPets(nextPets);
      setSettings(nextSettings);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to load pets");
    }
  }, [runtimeClient]);

  useEffect(() => {
    void load();
    const unsubscribe = runtimeClient.onPetCatalogChanged(() => {
      void load();
    });
    return unsubscribe;
  }, [load, runtimeClient]);

  async function handleSelect(petId: string) {
    try {
      const next = await runtimeClient.selectPet(petId);
      setSettings(next);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to select pet");
    }
  }

  async function handleWake() {
    if (!settings?.selectedPetId) return;
    try {
      const next = await runtimeClient.setPetOverlayOpen(true);
      setSettings(next);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to wake pet");
    }
  }

  async function handleTuckAway() {
    try {
      const next = await runtimeClient.setPetOverlayOpen(false);
      setSettings(next);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to tuck away pet");
    }
  }

  async function handleRefresh() {
    setLoading(true);
    setMessage(null);
    try {
      const report = await runtimeClient.refreshCodexPets();
      setImportReport(report);
      await load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to refresh pets");
    } finally {
      setLoading(false);
    }
  }

  async function handleToggleAutoImport() {
    try {
      const next = await runtimeClient.setPetImportPreferences({
        importCodexPets: !settings?.importCodexPets,
      });
      setSettings(next);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to update preferences");
    }
  }

  async function handleOpenAwakenPetsDir() {
    try {
      await runtimeClient.openAwakenPetsDirectory();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to open directory");
    }
  }

  async function handleOpenCodexPetsDir() {
    try {
      await runtimeClient.openCodexPetsDirectory();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to open directory");
    }
  }

  async function handleCreatePet() {
    try {
      await runtimeClient.createPet();
      setMessage("已创建本地宠物模板，请替换 spritesheet.png 后刷新");
      await load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to create pet");
    }
  }

  const selectedPet = pets.find((p) => p.id === settings?.selectedPetId);
  const hasSelection = Boolean(settings?.selectedPetId);

  return (
    <section className="pets-settings" aria-label="宠物">
      <div className="pets-settings-toolbar">
        <div className="pets-settings-actions">
          <Tooltip
            label={
              !hasSelection
                ? "请先选择一只宠物"
                : settings?.overlayOpen
                  ? "宠物浮窗已打开"
                  : "打开桌面宠物浮窗"
            }
            placement="top"
          >
            <Button
              variant="default"
              size="sm"
              onClick={handleWake}
              disabled={!hasSelection || settings?.overlayOpen}
            >
              唤醒宠物
            </Button>
          </Tooltip>
          <Tooltip
            label={
              settings?.overlayOpen
                ? "收起桌面宠物浮窗"
                : "宠物浮窗当前未打开"
            }
            placement="top"
          >
            <Button
              variant="ghost"
              size="sm"
              onClick={handleTuckAway}
              disabled={!settings?.overlayOpen}
            >
              收起宠物
            </Button>
          </Tooltip>
        </div>
      </div>

      {message ? <div className="pets-settings-message">{message}</div> : null}

      <div className="pets-settings-preview">
        {selectedPet?.spritesheetUrl ? (
          <div className="pets-settings-preview-sprite">
            <PetSprite
              spritesheetUrl={selectedPet.spritesheetUrl}
              state="waiting"
              widthPx={120}
            />
          </div>
        ) : (
          <div className="pets-settings-preview-placeholder">
            选择一只宠物以预览
          </div>
        )}
        <div className="pets-settings-preview-meta">
          <strong>{selectedPet?.displayName ?? "未选择"}</strong>
          <span>{selectedPet?.description}</span>
        </div>
      </div>

      <div className="pets-settings-section">
        <div className="pets-settings-section-header">
          <h2>已导入宠物</h2>
          <div className="pets-settings-section-actions">
            <Button
              variant="ghost"
              size="sm"
              onClick={handleRefresh}
              disabled={loading}
              icon={<RefreshCw size={14} />}
            >
              刷新 Codex 宠物
            </Button>
            <Button variant="ghost" size="sm" onClick={handleOpenAwakenPetsDir}>
              打开 Awaken 目录
            </Button>
            <Button variant="ghost" size="sm" onClick={handleOpenCodexPetsDir}>
              打开 Codex 目录
            </Button>
          </div>
        </div>

        <label className="pets-settings-toggle">
          <input
            type="checkbox"
            checked={settings?.importCodexPets ?? true}
            onChange={handleToggleAutoImport}
          />
          <span>启动时自动导入 Codex 自定义宠物</span>
        </label>

        <div className="pets-settings-list" role="radiogroup" aria-label="选择宠物">
          {pets.length === 0 ? (
            <div className="pets-settings-empty">暂无可用宠物</div>
          ) : (
            pets.map((pet) => (
              <label
                key={pet.id}
                className={`pets-settings-item${
                  pet.id === settings?.selectedPetId
                    ? " pets-settings-item-active"
                    : ""
                }${!pet.available ? " pets-settings-item-missing" : ""}`}
              >
                <input
                  type="radio"
                  name="selected-pet"
                  value={pet.id}
                  checked={pet.id === settings?.selectedPetId}
                  onChange={() => handleSelect(pet.id)}
                  disabled={!pet.available}
                />
                <span className="pets-settings-item-name">{pet.displayName}</span>
                <span className="pets-settings-item-source">{sourceLabel(pet.source)}</span>
                {pet.warning ? (
                  <span className="pets-settings-item-warning">{pet.warning}</span>
                ) : null}
              </label>
            ))
          )}
        </div>

        <div className="pets-settings-create">
          <Button variant="default" size="sm" onClick={handleCreatePet}>
            创建自己的宠物
          </Button>
        </div>
      </div>

      {importReport && importReport.failed.length > 0 ? (
        <div className="pets-settings-errors">
          <h3>导入错误</h3>
          <ul>
            {importReport.failed.map((item, index) => (
              <li key={index}>
                {item.sourceId ?? item.sourcePath}: {item.error}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  );
}

function sourceLabel(source: PetSummary["source"]): string {
  switch (source) {
    case "awaken-builtin":
      return "Awaken 内置";
    case "awaken-local":
      return "Awaken 本地";
    case "codex-custom":
      return "Codex 自定义";
    default:
      return source;
  }
}
