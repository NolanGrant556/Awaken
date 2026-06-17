const SUPPORTED_COMPONENTS = new Set([
  "Badge",
  "Button",
  "Card",
  "Column",
  "KeyValue",
  "Row",
  "Text",
]);

export type A2UIAction = {
  event?: {
    name?: string;
  };
};

export type A2UIComponent = {
  id: string;
  component: string;
  text?: string;
  variant?: string;
  tone?: string;
  label?: string;
  value?: string;
  children?: string[];
  child?: string;
  disabled?: boolean;
  action?: A2UIAction;
};

export type A2UISurface = {
  version: string;
  metadata?: {
    source?: string;
    readOnly?: boolean;
  };
  updateComponents: {
    surfaceId: string;
    components: A2UIComponent[];
  };
};

export type A2UISurfaceParseResult =
  | { ok: true; surface: A2UISurface }
  | { ok: false; error: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringArray(value: unknown): string[] | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    return undefined;
  }
  return value;
}

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function optionalBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function parseAction(value: unknown): A2UIAction | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (!isRecord(value)) {
    return undefined;
  }
  const event = value.event;
  if (!isRecord(event)) {
    return undefined;
  }
  return {
    event: {
      name: optionalString(event.name),
    },
  };
}

function parseComponent(value: unknown): A2UIComponent | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  const id = optionalString(value.id);
  const component = optionalString(value.component);
  if (!id || !component || !SUPPORTED_COMPONENTS.has(component)) {
    return undefined;
  }

  const children = stringArray(value.children);
  if (value.children !== undefined && !children) {
    return undefined;
  }

  return {
    id,
    component,
    text: optionalString(value.text),
    variant: optionalString(value.variant),
    tone: optionalString(value.tone),
    label: optionalString(value.label),
    value: optionalString(value.value),
    children,
    child: optionalString(value.child),
    disabled: optionalBoolean(value.disabled),
    action: parseAction(value.action),
  };
}

export function parseA2UISurface(content?: string): A2UISurfaceParseResult {
  if (!content?.trim()) {
    return { ok: false, error: "A2UI artifact has no JSON content." };
  }
  if (content.length > 64_000) {
    return { ok: false, error: "A2UI artifact is too large for the Phase 5 viewer." };
  }

  let decoded: unknown;
  try {
    decoded = JSON.parse(content);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid JSON";
    return { ok: false, error: message };
  }

  if (!isRecord(decoded)) {
    return { ok: false, error: "A2UI payload must be an object." };
  }

  const version = optionalString(decoded.version);
  const updateComponents = decoded.updateComponents;
  if (!version || !isRecord(updateComponents)) {
    return { ok: false, error: "A2UI payload must include version and updateComponents." };
  }

  const surfaceId = optionalString(updateComponents.surfaceId);
  const rawComponents = updateComponents.components;
  if (!surfaceId || !Array.isArray(rawComponents)) {
    return { ok: false, error: "A2UI updateComponents must include surfaceId and components." };
  }
  if (rawComponents.length === 0 || rawComponents.length > 64) {
    return { ok: false, error: "A2UI surface must contain 1-64 components." };
  }

  const components = rawComponents.map(parseComponent);
  if (components.some((component) => !component)) {
    return { ok: false, error: "A2UI surface contains unsupported or invalid components." };
  }

  const metadata = isRecord(decoded.metadata)
    ? {
        source: optionalString(decoded.metadata.source),
        readOnly: optionalBoolean(decoded.metadata.readOnly),
      }
    : undefined;

  return {
    ok: true,
    surface: {
      version,
      metadata,
      updateComponents: {
        surfaceId,
        components: components as A2UIComponent[],
      },
    },
  };
}
