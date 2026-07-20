import { useCallback, useEffect, useState } from "react";
import { AwakenPetsSettings } from "@awaken/design-system";
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
    <AwakenPetsSettings
      overlayOpen={settings?.overlayOpen}
      hasSelection={hasSelection}
      selectedDisplayName={selectedPet?.displayName}
      selectedDescription={selectedPet?.description}
      selectedPreview={selectedPet?.spritesheetUrl ? <PetSprite spritesheetUrl={selectedPet.spritesheetUrl} spriteVersionNumber={selectedPet.spriteVersionNumber} state="waiting" widthPx={80} /> : undefined}
      autoImport={settings?.importCodexPets ?? true}
      loading={loading}
      message={message}
      pets={pets.map((pet) => ({
        id: pet.id,
        displayName: pet.displayName,
        available: pet.available,
        selected: pet.id === settings?.selectedPetId,
        warning: pet.warning,
        preview: pet.spritesheetUrl ? <PetStaticPreview spritesheetUrl={pet.spritesheetUrl} spriteVersionNumber={pet.spriteVersionNumber} /> : undefined,
      }))}
      importErrors={importReport?.failed.map((item, index) => ({ key: index, source: item.sourceId ?? item.sourcePath, error: item.error }))}
      onToggleOverlay={() => void handleToggleOverlay()}
      onRefresh={() => void handleRefresh()}
      onOpenAwakenDirectory={() => void handleOpenAwakenPetsDir()}
      onOpenCodexDirectory={() => void handleOpenCodexPetsDir()}
      onToggleAutoImport={() => void handleToggleAutoImport()}
      onSelectPet={(petId) => void handleSelect(petId)}
    />
  );
}
