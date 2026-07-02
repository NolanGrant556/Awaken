import { app } from "electron";
import { mkdir } from "node:fs/promises";
import { join } from "node:path";

export async function ensureConfigDir(): Promise<string> {
  const dir = join(app.getPath("userData"), "config");
  await mkdir(dir, { recursive: true });
  return dir;
}
