import path from "node:path";

// Development acceptance keeps normal Desktop/Core/LLM code paths, while
// preventing subprocess tools from finding the real OS account's credentials.
// Packaged builds and ordinary development launches never use these overrides.
export function acceptanceChildEnvironment(options: { root?: string; dev: boolean; packaged: boolean }): NodeJS.ProcessEnv {
  if (!options.dev || options.packaged || !options.root) return {};
  if (!path.isAbsolute(options.root)) throw new Error("Desktop acceptance root must be absolute");
  const acceptanceHome = path.join(options.root, "home");
  return {
    HOME: acceptanceHome, USERPROFILE: acceptanceHome,
    APPDATA: path.join(acceptanceHome, "AppData", "Roaming"),
    LOCALAPPDATA: path.join(acceptanceHome, "AppData", "Local"),
    XDG_CONFIG_HOME: path.join(acceptanceHome, ".config"),
    XDG_DATA_HOME: path.join(acceptanceHome, ".local", "share"),
    XDG_CACHE_HOME: path.join(acceptanceHome, ".cache"),
  };
}
