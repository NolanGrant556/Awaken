import type { AwakenIconConcept } from "./icons";

export function composerTokenIconConcept(type: string | undefined): AwakenIconConcept {
  switch (type) {
    case "capability":
      return "computer-use";
    case "skill":
      return "skill";
    case "plugin":
      return "plugin-default";
    case "workflow":
      return "search";
    default:
      return "capability-package";
  }
}
