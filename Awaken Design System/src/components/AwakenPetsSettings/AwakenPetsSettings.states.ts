import { createElement } from "react";
import type { AwakenPetsSettingsProps } from "./AwakenPetsSettings";
export const AwakenPetsSettingsSample: AwakenPetsSettingsProps = {
  hasSelection: true,
  selectedDisplayName: "Awaken",
  selectedDescription: "Awaken 内置桌面宠物",
  selectedPreview: createElement("div", { className: "pets-settings-preview-placeholder" }, "AW"),
  pets: [{
    id: "awaken",
    displayName: "Awaken",
    available: true,
    selected: true,
    preview: createElement("div", { className: "pet-static-preview pet-static-preview-placeholder" }),
  }],
};
