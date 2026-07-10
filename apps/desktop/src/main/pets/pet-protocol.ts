export { PET_IPC_CHANNELS } from "../../shared/pets.js";

export const PET_ASSET_PROTOCOL = "awaken-pet:";
export const PET_ASSET_HOST = "asset";

export function isPetAssetUrl(url: string): boolean {
  return url.startsWith(`${PET_ASSET_PROTOCOL}//${PET_ASSET_HOST}/`);
}

export function parsePetAssetUrl(
  url: string,
): { petId: string; assetName: string } | null {
  if (!isPetAssetUrl(url)) return null;
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== PET_ASSET_PROTOCOL || parsed.host !== PET_ASSET_HOST) {
      return null;
    }
    const segments = parsed.pathname.split("/").filter(Boolean);
    if (segments.length !== 2) return null;
    const [petId, assetName] = segments;
    return { petId: decodeURIComponent(petId ?? ""), assetName: assetName ?? "" };
  } catch {
    return null;
  }
}
