// Minimal PoC: verify that @trycua/cua-driver can be loaded and that a
// same-process CuaDriver can start a session and capture desktop state.
// This script is for local exploration only; it may fail if macOS TCC
// permissions (Screen Recording / Accessibility) are not granted.

import {
  CuaDriver,
  CaptureScope,
  StartSessionInput,
  GetDesktopStateInput,
  EndSessionInput,
  requestMacOsPermissions,
  currentMacOsPermissionStatus,
} from "@trycua/cua-driver";

async function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function main() {
  console.log("[poc] loading CUA driver...");
  let driver;
  try {
    driver = CuaDriver.create(undefined);
  } catch (err) {
    console.error("[poc] CuaDriver.create failed:", err);
    process.exit(1);
  }

  console.log("[poc] driver available:", driver.isAvailable());
  try {
    const metadata = await driver.metadata();
    console.log("[poc] driver metadata:", JSON.stringify(metadata, null, 2));
  } catch (err) {
    console.warn("[poc] metadata() failed:", err.message);
  }

  // Probe macOS permissions in the importing process. This only makes sense
  // on macOS; on other platforms the import is present but the call may no-op.
  try {
    const permissions = requestMacOsPermissions();
    console.log("[poc] macOS permissions:", JSON.stringify(permissions));
    const hasRequired = Boolean(permissions.accessibility && permissions.screenRecording);
    console.log("[poc] has required macOS permissions:", hasRequired);
    const current = currentMacOsPermissionStatus();
    console.log("[poc] current macOS permission status:", JSON.stringify(current));
  } catch (err) {
    console.warn("[poc] permission probe failed:", err.message);
  }

  const sessionId = `awaken-poc-${Date.now()}`;
  console.log(`[poc] starting window session ${sessionId}...`);
  try {
    const startOut = await driver.startSession(
      StartSessionInput.new({ session: sessionId, captureScope: CaptureScope.Window }),
    );
    console.log("[poc] startSession output:", JSON.stringify(startOut, null, 2));
  } catch (err) {
    console.error("[poc] startSession failed:", err);
    driver.shutdown();
    driver.uniffiDestroy();
    process.exit(1);
  }

  console.log("[poc] capturing desktop state in window session (expected to fail)...");
  try {
    const state = await driver.getDesktopState(
      GetDesktopStateInput.new({ session: sessionId }),
    );
    console.log("[poc] getDesktopState text:", state.text?.slice(0, 200));
    console.log("[poc] getDesktopState images:", state.images.length);
    console.log("[poc] getDesktopState isError:", state.isError);
  } catch (err) {
    console.error("[poc] getDesktopState failed:", err);
  }

  console.log("[poc] ending window session...");
  try {
    await driver.endSession(EndSessionInput.new({ session: sessionId }));
  } catch (err) {
    console.error("[poc] endSession failed:", err);
  }

  const desktopSessionId = `awaken-poc-desktop-${Date.now()}`;
  console.log(`[poc] starting desktop session ${desktopSessionId}...`);
  try {
    const startOut = await driver.startSession(
      StartSessionInput.new({ session: desktopSessionId, captureScope: CaptureScope.Desktop }),
    );
    console.log("[poc] startSession (desktop) output:", JSON.stringify(startOut, null, 2));
  } catch (err) {
    console.error("[poc] startSession (desktop) failed:", err);
    driver.shutdown();
    driver.uniffiDestroy();
    process.exit(1);
  }

  console.log("[poc] capturing desktop state in desktop session...");
  try {
    const state = await driver.getDesktopState(
      GetDesktopStateInput.new({ session: desktopSessionId }),
    );
    console.log("[poc] getDesktopState text:", state.text?.slice(0, 200));
    console.log("[poc] getDesktopState images:", state.images.length);
    console.log("[poc] getDesktopState isError:", state.isError);
    console.log("[poc] getDesktopState degraded:", state.degraded);
    if (state.images.length > 0) {
      console.log("[poc] first image mimeType:", state.images[0].mimeType);
      console.log("[poc] first image base64 length:", state.images[0].dataBase64?.length);
    }
    if (state.rawJson) {
      const preview = JSON.stringify(JSON.parse(state.rawJson), null, 2).slice(0, 1000);
      console.log("[poc] rawJson preview:", preview);
    }
  } catch (err) {
    console.error("[poc] getDesktopState (desktop) failed:", err);
  }

  console.log("[poc] ending desktop session...");
  try {
    const endOut = await driver.endSession(EndSessionInput.new({ session: desktopSessionId }));
    console.log("[poc] endSession output:", JSON.stringify(endOut, null, 2));
  } catch (err) {
    console.error("[poc] endSession failed:", err);
  }

  console.log("[poc] shutting down driver...");
  try {
    await driver.shutdown();
  } catch (err) {
    console.warn("[poc] shutdown failed:", err.message);
  }
  driver.uniffiDestroy();
  console.log("[poc] done");
}

main().catch((err) => {
  console.error("[poc] unexpected error:", err);
  process.exit(1);
});
