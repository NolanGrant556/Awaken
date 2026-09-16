// Isolated Main tests use this adapter for the Core boundary. Product binary
// preparation, migration, leases and deletion are tested in src/managedenv.
import fs from "node:fs/promises";
import path from "node:path";
import { CliConnectorManager as ProductManager } from "./manager.js";
import { extractSingleExecutable } from "./archive.js";
import type { CliProviderId } from "../../shared/runtime.js";
type Options = ConstructorParameters<typeof ProductManager>[0];
const roots = new Map<string, Map<CliProviderId, { executablePath: string; version: string; healthy: boolean }>>();
export class CliConnectorManager extends ProductManager {
  constructor(options: Options) {
    let installed = roots.get(options.homeDir);
    if (!installed) { installed = new Map(); roots.set(options.homeDir, installed); }
    const records = installed;
    super({
      inspectSharedComponent: async (id) => records.get(id) ?? null,
      reserveSharedComponentRemoval: async () => ({ token: "fixture", release: async () => {} }),
      holdSharedExecutable: async () => async () => {},
      removeSharedComponent: async (id) => {
        const record = records.get(id);
        if (record) await fs.rm(record.executablePath, { force: true });
        records.delete(id);
      },
      prepareSharedComponent: async (id, signal, progress) => {
        const manifest = JSON.parse(await fs.readFile(options.manifestPath, "utf8"));
        const asset = manifest.assets.find((value: { providerId: string }) => value.providerId === id);
        progress("正在下载官方 CLI");
        const response = await options.fetchAsset(asset.url, signal);
        const directory = path.join(options.homeDir, ".awaken/runtime/cli", id, asset.version, `${asset.platform}-${asset.architecture}`);
        await fs.mkdir(directory, { recursive: true });
        const archive = path.join(directory, "fixture.tar.gz");
        await fs.writeFile(archive, Buffer.from(await response.arrayBuffer()));
        const executablePath = path.join(directory, asset.executableName);
        await extractSingleExecutable(archive, asset.archiveType, asset.archiveExecutablePath, executablePath);
        await fs.chmod(executablePath, 0o700);
        await fs.rm(archive);
        records.set(id, { executablePath, version: asset.version, healthy: true });
        return executablePath;
      },
      ...options,
    });
  }
}
