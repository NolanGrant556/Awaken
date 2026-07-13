export interface DesktopInstanceIdentity {
  schemaVersion: 1;
  applicationId: "com.awaken.desktop" | "com.awaken.desktop.dev";
  applicationName: "Awaken" | "Awaken Dev";
  userDataPathSegments: readonly ["@awaken", "desktop"] | readonly ["@awaken-dev", "desktop"];
  launchMode: "development" | "packaged";
}

export function desktopInstanceIdentity(
  isDevelopment: boolean,
): DesktopInstanceIdentity {
  return isDevelopment
    ? {
        schemaVersion: 1,
        applicationId: "com.awaken.desktop.dev",
        applicationName: "Awaken Dev",
        userDataPathSegments: ["@awaken-dev", "desktop"],
        launchMode: "development",
      }
    : {
        schemaVersion: 1,
        applicationId: "com.awaken.desktop",
        applicationName: "Awaken",
        userDataPathSegments: ["@awaken", "desktop"],
        launchMode: "packaged",
      };
}
