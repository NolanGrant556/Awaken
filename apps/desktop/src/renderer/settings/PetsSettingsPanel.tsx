import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import {
  AwakenButton as Button,
  AwakenTooltip as Tooltip,
} from "@awaken/design-system";
import type { RuntimeClient } from "../runtime/client";
import {
  petSheetRowsForVersion,
  type PetImportReport,
  type PetSettings,
  type PetSummary,
} from "../../shared/pets";
import { PetSprite } from "../pet-overlay/PetSprite";

type PetsSettingsPanelProps = {
  runtimeClient: RuntimeClient;
};

function PetStaticPreview({
  spritesheetUrl,
  spriteVersionNumber,
  widthPx,
}: {
  spritesheetUrl: string;
  spriteVersionNumber: PetSummary["spriteVersionNumber"];
  widthPx?: number;
}) {
  const sheetRows = petSheetRowsForVersion(spriteVersionNumber);
  return (
    <div
      className="pet-static-preview"
      role="img"
      aria-label="宠物预览"
      style={{
        ["--pet-static-preview-width" as string]: `${widthPx ?? 56}px`,
        ["--pet-sheet-rows" as string]: sheetRows,
        backgroundImage: `url(${spritesheetUrl})`,
      }}
    />
  );
}

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

  async function handleToggleOverlay() {
    const nextOpen = !settings?.overlayOpen;
    if (nextOpen && !settings?.selectedPetId) return;
    try {
      const next = await runtimeClient.setPetOverlayOpen(nextOpen);
      setSettings(next);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Failed to toggle pet overlay");
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
                  ? "收起桌面宠物浮窗"
                  : "打开桌面宠物浮窗"
            }
            placement="top"
          >
            <Button
              variant="default"
              size="md"
              onClick={handleToggleOverlay}
              disabled={!hasSelection}
            >
              {settings?.overlayOpen ? "收起宠物" : "唤醒宠物"}
            </Button>
          </Tooltip>
        </div>
      </div>

      {message ? <div className="pets-settings-message">{message}</div> : null}

      <div className="pets-settings-preview">
        <div className="pets-settings-preview-sprite">
          {selectedPet?.spritesheetUrl ? (
            <PetSprite
              spritesheetUrl={selectedPet.spritesheetUrl}
              spriteVersionNumber={selectedPet.spriteVersionNumber}
              state="waiting"
              widthPx={80}
            />
          ) : (
            <div className="pets-settings-preview-placeholder">?</div>
          )}
        </div>
        <div className="pets-settings-preview-meta">
          <span className="pets-settings-preview-label">当前选择的宠物</span>
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
                {pet.spritesheetUrl ? (
                  <PetStaticPreview
                    spritesheetUrl={pet.spritesheetUrl}
                    spriteVersionNumber={pet.spriteVersionNumber}
                  />
                ) : (
                  <div className="pet-static-preview pet-static-preview-placeholder" />
                )}
                <span className="pets-settings-item-name">{pet.displayName}</span>
                {pet.warning ? (
                  <span className="pets-settings-item-warning">{pet.warning}</span>
                ) : null}
              </label>
            ))
          )}
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
