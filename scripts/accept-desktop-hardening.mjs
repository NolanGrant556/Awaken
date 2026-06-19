import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, "..");
const npm = process.platform === "win32" ? "npm.cmd" : "npm";
const skipSmokePattern = /--- SKIP: Test.*Smoke|skipping .*smoke|LLM_API_KEY not set/i;

function loadDotEnv() {
  const envPath = join(root, ".env");
  if (!existsSync(envPath)) {
    return;
  }

  for (const line of readFileSync(envPath, "utf8").split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) {
      continue;
    }

    const [key, ...rest] = trimmed.split("=");
    const name = key.trim();
    let value = rest.join("=").trim();
    value = value.replace(/^["']|["']$/g, "");
    if (name && value && !process.env[name]) {
      process.env[name] = value;
    }
  }
}

function fail(message) {
  console.error(`\n[acceptance:desktop-hardening] ${message}`);
  process.exit(1);
}

function runStep({ name, command, args, mustNotMatch }) {
  console.log(`\n[acceptance:desktop-hardening] ${name}`);
  console.log(`$ ${[command, ...args].join(" ")}`);

  const result = spawnSync(command, args, {
    cwd: root,
    env: process.env,
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  });

  if (result.stdout) {
    process.stdout.write(result.stdout);
  }
  if (result.stderr) {
    process.stderr.write(result.stderr);
  }
  if (result.error) {
    fail(`${name} failed to start: ${result.error.message}`);
  }
  if (result.status !== 0) {
    fail(`${name} failed with exit code ${result.status ?? "unknown"}`);
  }

  const output = `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
  if (mustNotMatch?.test(output)) {
    fail(`${name} skipped a required real LLM smoke test`);
  }
}

loadDotEnv();

if (!process.env.LLM_API_KEY || process.env.LLM_API_KEY === "YOUR-API-KEY") {
  fail("LLM_API_KEY is required; real LLM smoke must not be skipped");
}

const steps = [
  {
    name: "ensure current-platform awaken-core binary",
    command: process.execPath,
    args: ["scripts/ensure-desktop-runtime.mjs"],
  },
  {
    name: "desktop typecheck",
    command: npm,
    args: ["run", "typecheck:desktop"],
  },
  {
    name: "desktop unit tests",
    command: npm,
    args: ["--workspace", "@awaken/desktop", "run", "test"],
  },
  {
    name: "runtime Go tests with direct LLM smoke",
    command: "go",
    args: ["test", "-v", "./src/llm/...", "./cmd/awaken-core/...", "./src/runtime/..."],
    mustNotMatch: skipSmokePattern,
  },
  {
    name: "server Go tests with real LLM run smokes",
    command: "go",
    args: ["test", "-v", "./src/server/..."],
    mustNotMatch: skipSmokePattern,
  },
  {
    name: "packaged Desktop build",
    command: npm,
    args: ["--workspace", "@awaken/desktop", "run", "dist"],
  },
];

for (const step of steps) {
  runStep(step);
}

console.log("\n[acceptance:desktop-hardening] PASS");
console.log("Manual release evidence still required: run Desktop dev/app once and confirm no residual awaken-core process.");
