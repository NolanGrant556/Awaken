export type AwakenMemoryLightFlowTheme = "light" | "dark";
export type AwakenMemoryLightFlowTone = "memory" | "effort" | "danger";
export type AwakenMemoryLightFlowVariant = "ambient" | "compact";

type LightFlowPalette = {
  backgroundTop: string;
  backgroundBottom: string;
  filament: readonly [number, number, number];
  accent: readonly [number, number, number];
  highlight: readonly [number, number, number];
  haze: readonly [number, number, number];
  filamentOpacity: number;
  particleOpacity: number;
  glowOpacity: number;
};

type WaveDefinition = {
  lanePhaseOffset: number;
  speed: number;
  primaryFrequency: number;
  primaryAmplitude: number;
  detailFrequency: number;
  detailAmplitude: number;
};

type FilamentDefinition = {
  wave: WaveDefinition;
  alpha: number;
  colorChoice: number;
  gradientAlpha: readonly [number, number, number, number, number];
  lineWidth: number;
};

type ParticleDefinition = {
  baseX: number;
  normalizedY: number;
  speed: number;
  wave: WaveDefinition;
  size: number;
  alpha: number;
  useHighlight: boolean;
  aspectRatio: number;
};

type LightFlowRenderCache = {
  theme: AwakenMemoryLightFlowTheme;
  tone: AwakenMemoryLightFlowTone;
  width: number;
  height: number;
  pixelWidth: number;
  pixelHeight: number;
  background: CanvasGradient;
  filamentGradients: CanvasGradient[];
};

const palettes: Record<AwakenMemoryLightFlowTheme, LightFlowPalette> = {
  light: {
    backgroundTop: "#f8fcfa",
    backgroundBottom: "#eaf7f0",
    filament: [24, 132, 80],
    accent: [22, 174, 103],
    highlight: [4, 105, 58],
    haze: [49, 183, 115],
    filamentOpacity: 0.62,
    particleOpacity: 0.38,
    glowOpacity: 0.44,
  },
  dark: {
    backgroundTop: "#11110f",
    backgroundBottom: "#111511",
    filament: [113, 224, 181],
    accent: [89, 236, 178],
    highlight: [225, 255, 243],
    haze: [83, 219, 168],
    filamentOpacity: 0.3,
    particleOpacity: 0.32,
    glowOpacity: 0.38,
  },
};

const effortPalettes: Record<Exclude<AwakenMemoryLightFlowTone, "memory">, LightFlowPalette> = {
  effort: {
    backgroundTop: "#18a66d",
    backgroundBottom: "#0b7654",
    filament: [184, 255, 226],
    accent: [104, 246, 188],
    highlight: [248, 255, 252],
    haze: [71, 229, 160],
    filamentOpacity: 0.9,
    particleOpacity: 0.78,
    glowOpacity: 0.82,
  },
  danger: {
    backgroundTop: "#ef625b",
    backgroundBottom: "#b92338",
    filament: [255, 207, 205],
    accent: [255, 142, 131],
    highlight: [255, 250, 248],
    haze: [255, 92, 87],
    filamentOpacity: 0.92,
    particleOpacity: 0.82,
    glowOpacity: 0.86,
  },
};

function hash(value: number): number {
  const seed = Math.sin(value * 12.9898 + 78.233) * 43758.5453;
  return seed - Math.floor(seed);
}

function createWaveDefinition(seed: number): WaveDefinition {
  return {
    lanePhaseOffset: (hash(seed + 1) - 0.5) * 0.9,
    speed: 0.86 + hash(seed + 2) * 0.62,
    primaryFrequency: 0.68 + hash(seed + 3) * 0.72,
    primaryAmplitude: 0.038 + hash(seed + 4) * 0.04,
    detailFrequency: 1.8 + hash(seed + 5) * 1.7,
    detailAmplitude: 0.006 + hash(seed + 6) * 0.012,
  };
}

const FILAMENT_DEFINITIONS = Array.from({ length: 112 }, (_, index): FilamentDefinition => {
  const seed = 90 + index * 3.17;
  return {
    wave: createWaveDefinition(seed),
    alpha: 0.038 + hash(seed + 7) * 0.064,
    colorChoice: hash(seed + 8),
    lineWidth: 0.2 + hash(seed + 11) * 0.36,
    gradientAlpha: [
      0.45 + hash(seed + 13) * 0.32,
      0.66 + hash(seed + 14) * 0.3,
      0.5 + hash(seed + 15) * 0.38,
      0.68 + hash(seed + 16) * 0.28,
      0.42 + hash(seed + 17) * 0.32,
    ],
  };
});

const PARTICLE_DEFINITIONS = Array.from({ length: 360 }, (_, index): ParticleDefinition => {
  const seed = index * 4.31 + 19;
  return {
    baseX: hash(seed),
    normalizedY: hash(seed + 3),
    speed: 0.022 + hash(seed + 1) * 0.024,
    wave: createWaveDefinition(seed + 40),
    size: 0.32 + Math.pow(hash(seed + 5), 2) * 1.15,
    alpha: 0.02 + hash(seed + 6) * 0.1,
    useHighlight: hash(seed + 7) > 0.82,
    aspectRatio: 0.48 + hash(seed + 8) * 0.7,
  };
});

const renderCaches = new WeakMap<CanvasRenderingContext2D, LightFlowRenderCache>();

function rgba(color: readonly [number, number, number], alpha: number): string {
  return `rgba(${color[0]}, ${color[1]}, ${color[2]}, ${Math.max(0, Math.min(1, alpha))})`;
}

function getRenderCache(
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  theme: AwakenMemoryLightFlowTheme,
  tone: AwakenMemoryLightFlowTone,
  palette: LightFlowPalette,
): LightFlowRenderCache {
  const cached = renderCaches.get(context);
  if (
    cached
    && cached.theme === theme
    && cached.tone === tone
    && cached.width === width
    && cached.height === height
    && cached.pixelWidth === context.canvas.width
    && cached.pixelHeight === context.canvas.height
  ) {
    return cached;
  }

  const background = context.createLinearGradient(0, 0, 0, height);
  background.addColorStop(0, palette.backgroundTop);
  background.addColorStop(1, palette.backgroundBottom);

  const filamentGradients = FILAMENT_DEFINITIONS.map((definition) => {
    const alpha = definition.alpha * palette.filamentOpacity / 0.3;
    const filamentColor = definition.colorChoice > 0.94
      ? palette.highlight
      : definition.colorChoice > 0.72
        ? palette.accent
        : palette.filament;
    const gradient = context.createLinearGradient(0, 0, width, 0);
    gradient.addColorStop(0, rgba(filamentColor, alpha * definition.gradientAlpha[0]));
    gradient.addColorStop(0.23, rgba(filamentColor, alpha * definition.gradientAlpha[1]));
    gradient.addColorStop(0.5, rgba(filamentColor, alpha * definition.gradientAlpha[2]));
    gradient.addColorStop(0.77, rgba(filamentColor, alpha * definition.gradientAlpha[3]));
    gradient.addColorStop(1, rgba(filamentColor, alpha * definition.gradientAlpha[4]));
    return gradient;
  });

  const nextCache = {
    theme,
    tone,
    width,
    height,
    pixelWidth: context.canvas.width,
    pixelHeight: context.canvas.height,
    background,
    filamentGradients,
  };
  renderCaches.set(context, nextCache);
  return nextCache;
}

function getFilamentY(
  normalizedY: number,
  x: number,
  time: number,
  definition: WaveDefinition,
): number {
  const lanePhase = normalizedY * 3.2 + definition.lanePhaseOffset;
  const primaryWave = Math.sin(
    x * Math.PI * 2 * definition.primaryFrequency - time * definition.speed + lanePhase,
  ) * definition.primaryAmplitude;
  const detailWave = Math.sin(
    x * Math.PI * 2 * definition.detailFrequency
      - time * definition.speed * 1.42
      + lanePhase * 1.7,
  ) * definition.detailAmplitude;
  const longCurrent = Math.sin(
    x * Math.PI * 1.24 - time * definition.speed * 0.54 + normalizedY * 4.2,
  ) * 0.018;

  return normalizedY + primaryWave + detailWave + longCurrent;
}

function drawEllipticalGlow(
  context: CanvasRenderingContext2D,
  x: number,
  y: number,
  radiusX: number,
  radiusY: number,
  color: readonly [number, number, number],
  opacity: number,
): void {
  context.save();
  context.translate(x, y);
  context.scale(radiusX, radiusY);
  const gradient = context.createRadialGradient(0, 0, 0, 0, 0, 1);
  gradient.addColorStop(0, rgba(color, opacity));
  gradient.addColorStop(0.42, rgba(color, opacity * 0.46));
  gradient.addColorStop(1, rgba(color, 0));
  context.fillStyle = gradient;
  context.fillRect(-1, -1, 2, 2);
  context.restore();
}

function drawFilamentField(
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  time: number,
  filamentGradients: readonly CanvasGradient[],
  variant: AwakenMemoryLightFlowVariant,
): void {
  const compact = variant === "compact";
  const count = compact
    ? Math.max(18, Math.min(34, Math.round(height * 0.9)))
    : Math.max(72, Math.min(FILAMENT_DEFINITIONS.length, Math.round(height * 0.62)));
  const steps = compact
    ? Math.max(48, Math.min(104, Math.round(width / 5)))
    : Math.max(96, Math.min(180, Math.round(width / 7)));

  for (let index = 0; index < count; index += 1) {
    const definition = FILAMENT_DEFINITIONS[index];
    const normalizedY = count <= 1 ? 0.5 : index / (count - 1);
    context.beginPath();
    for (let step = 0; step <= steps; step += 1) {
      const x = step / steps;
      const y = getFilamentY(normalizedY, x, time, definition.wave);
      if (step === 0) context.moveTo(0, y * height);
      else context.lineTo(x * width, y * height);
    }

    context.strokeStyle = filamentGradients[index];
    context.lineWidth = definition.lineWidth * (compact ? 3 : 1);
    context.stroke();
  }
}

function drawParticleField(
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  time: number,
  palette: LightFlowPalette,
  variant: AwakenMemoryLightFlowVariant,
): void {
  const particleCount = variant === "compact"
    ? Math.max(28, Math.min(84, Math.round((width * height) / 180)))
    : Math.max(
        150,
        Math.min(PARTICLE_DEFINITIONS.length, Math.round((width * height) / 920)),
      );

  for (let index = 0; index < particleCount; index += 1) {
    const definition = PARTICLE_DEFINITIONS[index];
    const x = ((definition.baseX + time * definition.speed) % 1.08) - 0.04;
    const y = getFilamentY(definition.normalizedY, x, time, definition.wave);
    if (y < 0 || y > 1) continue;

    const alpha = definition.alpha * palette.particleOpacity / 0.3;
    context.fillStyle = rgba(
      definition.useHighlight ? palette.highlight : palette.filament,
      alpha,
    );
    context.fillRect(
      x * width,
      y * height,
      definition.size,
      definition.size * definition.aspectRatio,
    );
  }
}

export function drawAwakenMemoryLightFlow(
  context: CanvasRenderingContext2D,
  width: number,
  height: number,
  elapsedMilliseconds: number,
  theme: AwakenMemoryLightFlowTheme,
  options: {
    tone?: AwakenMemoryLightFlowTone;
    variant?: AwakenMemoryLightFlowVariant;
  } = {},
): void {
  const tone = options.tone ?? "memory";
  const variant = options.variant ?? "ambient";
  const compactGlowScale = variant === "compact" ? 1.48 : 1;
  const palette = tone === "memory" ? palettes[theme] : effortPalettes[tone];
  const cache = getRenderCache(context, width, height, theme, tone, palette);
  const time = elapsedMilliseconds / 1000;
  context.clearRect(0, 0, width, height);

  context.fillStyle = cache.background;
  context.fillRect(0, 0, width, height);

  const ambientFlowX = (((time * 0.075) % 1.5) - 0.25) * width;
  drawEllipticalGlow(
    context,
    ambientFlowX,
    height * 0.46,
    width * 0.42,
    height * 0.7,
    palette.haze,
    palette.glowOpacity * 0.34 * compactGlowScale,
  );
  drawEllipticalGlow(
    context,
    width * 0.28,
    height * 0.5,
    width * 0.34,
    height * 0.78,
    palette.haze,
    palette.glowOpacity * 0.16 * compactGlowScale,
  );
  drawEllipticalGlow(
    context,
    width * 0.84,
    height * 0.42,
    width * 0.3,
    height * 0.68,
    palette.accent,
    palette.glowOpacity * 0.2 * compactGlowScale,
  );

  context.save();
  context.globalCompositeOperation = theme === "dark" ? "screen" : "source-over";
  drawFilamentField(context, width, height, time, cache.filamentGradients, variant);
  drawParticleField(context, width, height, time, palette, variant);
  context.restore();
}
