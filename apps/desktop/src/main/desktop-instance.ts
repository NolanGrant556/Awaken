export const desktopInstanceApplicationId = "com.awaken.desktop";
// Electron derives the macOS Safe Storage service from app.name. This value is
// therefore persistent credential identity, not mutable display copy.
export const desktopCredentialApplicationName = "Awaken";
export const desktopUserDataPathSegments = ["@awaken", "desktop"] as const;

export interface DesktopInstanceIdentity {
  schemaVersion: 1;
  applicationId: typeof desktopInstanceApplicationId;
  launchMode: "development" | "packaged";
}

export function desktopInstanceIdentity(
  isDevelopment: boolean,
): DesktopInstanceIdentity {
  return {
    schemaVersion: 1,
    applicationId: desktopInstanceApplicationId,
    launchMode: isDevelopment ? "development" : "packaged",
  };
}
