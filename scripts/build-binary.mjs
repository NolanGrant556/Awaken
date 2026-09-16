import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");

// Release resources are independently built and verified. A Go overlay embeds
// their catalog without overwriting the checkout's development catalog.
let dependencyOverlay;
if (process.env.AWAKEN_DEPENDENCY_CATALOG) {
  const catalog = resolve(process.env.AWAKEN_DEPENDENCY_CATALOG);
  execFileSync(process.execPath, [join(root, "scripts/check-managedenv-catalog.mjs"), catalog, "--release"], { stdio: "inherit" });
  const temporary = mkdtempSync(join(tmpdir(), "awaken-dependency-overlay-"));
  dependencyOverlay = join(temporary, "overlay.json");
  writeFileSync(dependencyOverlay, JSON.stringify({ Replace: { [join(root, "src/managedenv/assets/catalog.json")]: catalog } }));
  process.on("exit", () => rmSync(temporary, { recursive: true, force: true }));
}

const currentTarget = `${process.platform}-${process.arch}`;
const targets = process.env.AWAKEN_BINARY_TARGETS
  ? process.env.AWAKEN_BINARY_TARGETS.split(",").map((target) => target.trim()).filter(Boolean)
  : process.platform === "darwin"
    ? ["darwin-arm64", "darwin-x64"]
    : [currentTarget];
// Microsoft native/desktop OAuth client identifiers are public application
// metadata, not credentials. Keep Awaken's production identity in the release
// build so every packaged Desktop can start Outlook PKCE without asking the
// end user for configuration. Development and private distributions may still
// override it explicitly.
const defaultMicrosoftOutlookClientId = "1bdaadb2-a9d3-44a9-99cf-806172ca83af";
const microsoftOutlookClientId =
  (process.env.AWAKEN_MICROSOFT_CLIENT_ID ?? "").trim() || defaultMicrosoftOutlookClientId;
if (microsoftOutlookClientId && !/^[A-Za-z0-9._-]+$/.test(microsoftOutlookClientId)) {
  throw new Error("AWAKEN_MICROSOFT_CLIENT_ID contains unsupported characters");
}

function coreBuildArgs(outputPath) {
  const args = ["build"];
  if (dependencyOverlay) args.push("-overlay", dependencyOverlay);
  if (microsoftOutlookClientId) {
    args.push("-ldflags", `-X=main.microsoftOutlookClientID=${microsoftOutlookClientId}`);
  }
  args.push("-o", outputPath, "./cmd/awaken-core");
  return args;
}

function parseTarget(target) {
  const [platform, arch] = target.split("-");
  if (!platform || !arch) {
    throw new Error(`invalid target ${target}; expected <platform>-<arch>`);
  }
  return { platform, arch };
}

function goArch(nodeArch) {
  switch (nodeArch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      return nodeArch;
  }
}

function goOS(nodePlatform) {
  return nodePlatform === "win32" ? "windows" : nodePlatform;
}

for (const target of targets) {
  const { platform, arch } = parseTarget(target);
  const binName = platform === "win32" ? "awaken-core.exe" : "awaken-core";
  const outputDir = join(root, "apps", "desktop", "resources", "bin", target);
  const outputPath = join(outputDir, binName);

  mkdirSync(outputDir, { recursive: true });

  console.log(`[build-binary] building awaken-core for ${target} -> ${outputPath}`);
  try {
    execFileSync("go", coreBuildArgs(outputPath), {
      cwd: root,
      env: {
        ...process.env,
        GOOS: goOS(platform),
        GOARCH: goArch(arch),
      },
      stdio: "inherit",
    });
  } catch (err) {
    console.error("[build-binary] failed to build awaken-core:", err.message);
    process.exit(1);
  }
  console.log(`[build-binary] done: ${outputPath}`);

  const acpBinName = platform === "win32" ? "awaken-acp.exe" : "awaken-acp";
  const acpOutputPath = join(outputDir, acpBinName);
  console.log(`[build-binary] building awaken-acp for ${target} -> ${acpOutputPath}`);
  try {
    execFileSync("go", ["build", "-o", acpOutputPath, "./cmd/awaken-acp"], {
      cwd: root,
      env: {
        ...process.env,
        GOOS: goOS(platform),
        GOARCH: goArch(arch),
      },
      stdio: "inherit",
    });
  } catch (err) {
    console.error("[build-binary] failed to build awaken-acp:", err.message);
    process.exit(1);
  }
  console.log(`[build-binary] done: ${acpOutputPath}`);

  if (platform === "win32") {
    const networkWatchOutputPath = join(outputDir, "awaken-network-watch.exe");
    console.log(`[build-binary] building awaken-network-watch for ${target} -> ${networkWatchOutputPath}`);
    try {
      execFileSync("go", ["build", "-o", networkWatchOutputPath, "./cmd/awaken-network-watch"], {
        cwd: root,
        env: {
          ...process.env,
          GOOS: goOS(platform),
          GOARCH: goArch(arch),
        },
        stdio: "inherit",
      });
    } catch (err) {
      console.error("[build-binary] failed to build awaken-network-watch:", err.message);
      process.exit(1);
    }
    console.log(`[build-binary] done: ${networkWatchOutputPath}`);
  }
}
