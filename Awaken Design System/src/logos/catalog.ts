import type {
  AwakenLogoOpticalShape,
  AwakenLogoOpticalSize,
  AwakenLogoStyle,
} from "./construction";

export const AWAKEN_LOGO_KINDS = ["product", "provider"] as const;
export type AwakenLogoKind = (typeof AWAKEN_LOGO_KINDS)[number];

export type AwakenProductLogoId = "product-mark" | "product-wordmark" | "product-icon";

export type AwakenProviderLogoId =
  | "openai"
  | "anthropic"
  | "gemini"
  | "kimi"
  | "minimax"
  | "glm"
  | "qwen"
  | "doubao"
  | "deepseek"
  | "volcengine";

export type AwakenLogoId = AwakenProductLogoId | AwakenProviderLogoId;

export type AwakenLogoEntry = {
  id: AwakenLogoId;
  kind: AwakenLogoKind;
  name: string;
  concept: string;
  tags: string[];
  opticalShape: AwakenLogoOpticalShape;
  styles: AwakenLogoStyle[];
  opticalSizes: AwakenLogoOpticalSize[];
  usage: string;
};

export const AWAKEN_LOGO_CATALOG: readonly AwakenLogoEntry[] = [
  {
    id: "product-mark",
    kind: "product",
    name: "双柱标记",
    concept: "Awaken 产品标记",
    tags: ["awaken", "mark", "a", "双柱", "产品"],
    opticalShape: "vertical",
    styles: ["filled", "outline"],
    opticalSizes: [16, 24],
    usage: "应用内 chrome、连接器与品牌组合。颜色跟随 currentColor / --aw-text-primary。",
  },
  {
    id: "product-wordmark",
    kind: "product",
    name: "字标",
    concept: "Awaken 横向字标",
    tags: ["awaken", "wordmark", "字标", "欢迎"],
    opticalShape: "horizontal",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "欢迎空态、About 与安装封面。默认视觉宽度 220px，窄屏 200px。",
  },
  {
    id: "product-icon",
    kind: "product",
    name: "应用图标",
    concept: "Awaken App Icon",
    tags: ["awaken", "app icon", "dock", "squircle"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "Dock、开始菜单和安装包。不要裁切成应用内标记。",
  },
  {
    id: "openai",
    kind: "provider",
    name: "OpenAI",
    concept: "OpenAI 供应商",
    tags: ["openai", "gpt"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "anthropic",
    kind: "provider",
    name: "Anthropic",
    concept: "Anthropic 供应商",
    tags: ["anthropic", "claude"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "gemini",
    kind: "provider",
    name: "Gemini",
    concept: "Google Gemini 供应商",
    tags: ["gemini", "google"],
    opticalShape: "circle",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "kimi",
    kind: "provider",
    name: "Kimi",
    concept: "Kimi 供应商",
    tags: ["kimi", "moonshot"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "Kimi API 与 Kimi Coding Plan 共用。官方 App Icon，不裁切安全留白。",
  },
  {
    id: "minimax",
    kind: "provider",
    name: "MiniMax",
    concept: "MiniMax 供应商",
    tags: ["minimax"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "glm",
    kind: "provider",
    name: "GLM",
    concept: "GLM / Z.ai 供应商",
    tags: ["glm", "z.ai"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "qwen",
    kind: "provider",
    name: "Qwen",
    concept: "Qwen 供应商",
    tags: ["qwen", "通义"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "Qwen API 与 Qwen Token Plan 共用。官方标识，不重绘。",
  },
  {
    id: "doubao",
    kind: "provider",
    name: "豆包",
    concept: "Doubao 供应商",
    tags: ["doubao", "豆包"],
    opticalShape: "circle",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "Doubao Ark。官方标识，不重绘。",
  },
  {
    id: "deepseek",
    kind: "provider",
    name: "DeepSeek",
    concept: "DeepSeek 供应商",
    tags: ["deepseek"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "模型设置、Composer 与用量窗口。官方标识，不重绘。",
  },
  {
    id: "volcengine",
    kind: "provider",
    name: "火山引擎",
    concept: "火山引擎供应商",
    tags: ["volcengine", "火山"],
    opticalShape: "square",
    styles: ["filled"],
    opticalSizes: [24],
    usage: "火山 Coding Plan 与火山 Agent Plan。官方标识，不重绘。",
  },
] as const;

const CATALOG_BY_ID = new Map(AWAKEN_LOGO_CATALOG.map((entry) => [entry.id, entry]));

export function getAwakenLogo(id: AwakenLogoId): AwakenLogoEntry {
  const entry = CATALOG_BY_ID.get(id);
  if (!entry) {
    throw new Error(`Unknown Awaken logo id: ${id}`);
  }
  return entry;
}

export function findAwakenLogoByConcept(concept: string): AwakenLogoEntry | undefined {
  return AWAKEN_LOGO_CATALOG.find((entry) => entry.concept === concept);
}

export function searchAwakenLogos(query: string): AwakenLogoEntry[] {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return [...AWAKEN_LOGO_CATALOG];
  }
  return AWAKEN_LOGO_CATALOG.filter((entry) => {
    const haystack = [entry.id, entry.name, entry.concept, ...entry.tags]
      .join(" ")
      .toLowerCase();
    return haystack.includes(needle);
  });
}

export function assertAwakenLogoCatalog(): void {
  const ids = new Set<string>();
  const concepts = new Set<string>();
  for (const entry of AWAKEN_LOGO_CATALOG) {
    if (ids.has(entry.id)) {
      throw new Error(`Duplicate logo id: ${entry.id}`);
    }
    if (concepts.has(entry.concept)) {
      throw new Error(`Duplicate logo concept: ${entry.concept}`);
    }
    ids.add(entry.id);
    concepts.add(entry.concept);
  }
  for (const required of ["product-mark", "product-wordmark", "product-icon"] as const) {
    if (!ids.has(required)) {
      throw new Error(`Logo catalog is missing ${required}`);
    }
  }
}
