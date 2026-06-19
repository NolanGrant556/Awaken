import { app } from "electron";
import { access, mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

const DEFAULT_CONFIG_TOML = `[llm]
base_url = "https://api.deepseek.com/anthropic"
model_id = "deepseek-v4-pro"
protocol = "anthropic_messages"
auth_scheme = "auto"

[model]
temperature = 0.7
max_output_tokens = 8192
`;

const DEFAULT_ENV = `# 在此填写你的 LLM API Key
LLM_API_KEY=
`;

async function fileExists(filePath: string): Promise<boolean> {
  try {
    await access(filePath);
    return true;
  } catch {
    return false;
  }
}

export async function ensureConfigDir(): Promise<string> {
  const dir = join(app.getPath("userData"), "config");
  await mkdir(dir, { recursive: true });

  const configTomlPath = join(dir, "config.toml");
  if (!(await fileExists(configTomlPath))) {
    await writeFile(configTomlPath, DEFAULT_CONFIG_TOML, "utf8");
  }

  const envPath = join(dir, ".env");
  if (!(await fileExists(envPath))) {
    await writeFile(envPath, DEFAULT_ENV, "utf8");
  }

  return dir;
}
