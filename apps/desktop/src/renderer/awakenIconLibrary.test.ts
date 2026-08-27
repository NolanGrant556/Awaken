/**
 * Awaken 图标库的契约测试。
 *
 * 这些测试守的是「图标层是不是还成体系」，不是「某个图标好不好看」：
 *  - 几何快照：任何图形改动都在 review 里看得见；
 *  - 光学审计：每个图标仍然贴合它声明的光学形状；
 *  - 网格边界：坐标不越出 16 网格；
 *  - 描边不变量：全套线性图标同一个描边；
 *  - 概念表：一概念一图标，且目标都已注册；
 *  - 越层守卫：图标层不依赖 lucide-react；
 *  - 调用点守卫：AwakenIcon 只用五档尺寸，且不低于图标自己的可读下限。
 *
 * 测试放在 desktop workspace 是因为仓库当前只有这里配置了 vitest，
 * 并且它的 alias 直接指向 Design System 源码（见 apps/desktop/vitest.config.ts），
 * 所以这里断言的就是 Design System 的真实实现。
 */
import { readFileSync, readdirSync, statSync } from "node:fs";
import { createHash } from "node:crypto";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  AWAKEN_BRAND_ICON_NAMES,
  AWAKEN_ICON_CONCEPTS,
  AWAKEN_ICON_GRID,
  AWAKEN_ICON_NAMES,
  AWAKEN_ICON_SIZES,
  AWAKEN_ICON_STROKE,
  AWAKEN_LINE_ICON_NAMES,
  AWAKEN_OPTICAL_SHAPES,
  APPROVED_LUCIDE_GLYPHS,
  APPROVED_LUCIDE_ICON_NAMES,
  APPROVED_LUCIDE_LICENSE,
  APPROVED_LUCIDE_SOURCES,
  APPROVED_LUCIDE_STROKE,
  APPROVED_LUCIDE_VERSION,
  ICON_DEFINITIONS,
  getAwakenIcon,
  isAwakenIconName,
  opticalVisual,
} from "@awaken/design-system";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const DS_SRC = path.join(REPO_ROOT, "Awaken Design System", "src");
const DESKTOP_SRC = path.join(REPO_ROOT, "apps", "desktop", "src");

const PRODUCT_APPROVED_ICON_DIGESTS = {
  "agent-plugins-mark": "440d25a0fce1e7a0c22aa8af66ba7c6407b498b093ffdcf14e4f46b048491ac4",
  brain: "07158d2f25e925e3e36db7aee89b77d4f973ce758458e0d99a5d458c59783a4e",
  puzzle: "79a002f6eace9a0d74387aeb8a758759c0779fc56871d9d79a4e2cd525fd4d74",
  connector: "050aeec594e18aee075e2b302bf5af350a475b38e1c95f9262d60d3bcc066071",
  folder: "bc3ca3cdb88518eb7b3b8f60623627c23ba7bb9f64bb6adffa85614648262175",
  "folder-open": "74988aa27f3440fc2f92bed23b3acd81b1fef864aa18707602c73ad87123fdcc",
  "folder-plus": "d61abf5638dd4fcbb626cd0711c68d46dd72a16a381abc00f9dc424b6d40e32c",
  "mcp-mark": "98fd09c590a32b9f57ce02dbd2e8b00b5c5ad06b12ab3b31b56d063fafb69f5c",
  "book-open": "eec3bf9ff8717eb1d5dd3b5976310ec2fabfbf3e36f02f5e025f6afeedfd4232",
  terminal: "23704d14f851a1e3374751137118edbabd3520473f3470bd9a751864fc6cc20d",
} as const;

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "dist") continue;
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) {
      walk(full, out);
    } else if (/\.tsx?$/.test(entry)) {
      out.push(full);
    }
  }
  return out;
}

/** 从 path d 字符串里取出所有数值坐标。 */
function coordsOf(d: string): number[] {
  return (d.match(/-?\d+(?:\.\d+)?/g) ?? []).map(Number);
}

describe("图标注册表", () => {
  it("线性图标与品牌标记都已注册，且名字不重叠", () => {
    expect(AWAKEN_LINE_ICON_NAMES.length).toBeGreaterThan(0);
    expect(AWAKEN_BRAND_ICON_NAMES.length).toBeGreaterThan(0);
    const overlap = AWAKEN_LINE_ICON_NAMES.filter((n) =>
      (AWAKEN_BRAND_ICON_NAMES as readonly string[]).includes(n),
    );
    expect(overlap).toEqual([]);
    expect(AWAKEN_ICON_NAMES.length).toBe(
      AWAKEN_LINE_ICON_NAMES.length + AWAKEN_BRAND_ICON_NAMES.length,
    );
  });

  it("未注册的名字会抛错，而不是静默渲染空图形", () => {
    expect(isAwakenIconName("definitely-not-an-icon")).toBe(false);
    // @ts-expect-error 故意传入未注册名，验证运行时也会拦下
    expect(() => getAwakenIcon("definitely-not-an-icon")).toThrow(/未注册/);
  });

  it("每个图标都编译出至少一条 path", () => {
    for (const name of AWAKEN_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      expect(icon.paths.length, name).toBeGreaterThan(0);
      for (const d of icon.paths) {
        expect(d, `${name} 的 path 不能为空`).toMatch(/^M/);
      }
    }
  });
});

describe("几何快照", () => {
  it("全套线性图标的 path 数据保持稳定", () => {
    const snapshot: Record<string, string[]> = {};
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      snapshot[name] = [...getAwakenIcon(name).paths];
    }
    expect(snapshot).toMatchSnapshot();
  });
});

describe("光学与网格不变量", () => {
  it("Awaken 构造几何贴合光学盒，licensed authored 字形保留确认过的比例", () => {
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      if (icon.mode !== "line") continue;
      if (icon.provenance.kind === "licensed-authored") {
        expect(icon.measured, `${name} 不应被二次光学拉伸`).toBeUndefined();
        continue;
      }
      const [targetW, targetH] = opticalVisual(ICON_DEFINITIONS[name]);
      expect(icon.measured, `${name} 缺少光学测量`).toBeDefined();
      const [w, h] = icon.measured!;
      // 等比缩放只能让一条轴贴合目标，另一条轴不得超出。
      const touchesW = Math.abs(w - targetW) < 0.05;
      const touchesH = Math.abs(h - targetH) < 0.05;
      expect(touchesW || touchesH, `${name} 未贴合光学盒：实测 ${w}×${h}，目标 ${targetW}×${targetH}`).toBe(
        true,
      );
      expect(w, `${name} 宽度超出光学盒`).toBeLessThanOrEqual(targetW + 0.05);
      expect(h, `${name} 高度超出光学盒`).toBeLessThanOrEqual(targetH + 0.05);
    }
  });

  it("声明的光学形状必须是四种之一或显式 visual 覆盖", () => {
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const def = ICON_DEFINITIONS[name];
      expect(Object.keys(AWAKEN_OPTICAL_SHAPES)).toContain(def.shape);
    }
  });

  it("坐标不越出 16 网格（允许箭头按设计小幅溢出）", () => {
    const OVERSHOOT = 1.2;
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      for (const d of icon.paths) {
        for (const v of coordsOf(d)) {
          // 圆弧参数里也有半径等非坐标数值，这里只做宽松的上下界检查。
          expect(v, `${name} 坐标越界：${v}`).toBeGreaterThanOrEqual(-OVERSHOOT);
          expect(v, `${name} 坐标越界：${v}`).toBeLessThanOrEqual(AWAKEN_ICON_GRID + OVERSHOOT);
        }
      }
    }
  });

  it("描边字形只使用两套经确认的线宽，实心字形描边为 0", () => {
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      if (icon.mode !== "line") continue;
      // 实心字形的视觉包围盒就是几何包围盒，不为描边留半个描边宽度。
      const expectedStroke =
        icon.provenance.kind === "licensed-authored"
          ? APPROVED_LUCIDE_STROKE
          : AWAKEN_ICON_STROKE;
      expect(icon.strokeWidth, name).toBe(icon.filled ? 0 : expectedStroke);
    }
  });

  it("94 个确认候选全部静态固化并保留 Lucide 来源与许可", () => {
    expect(APPROVED_LUCIDE_ICON_NAMES).toHaveLength(94);
    expect(Object.keys(APPROVED_LUCIDE_SOURCES).sort()).toEqual(APPROVED_LUCIDE_ICON_NAMES);
    for (const name of APPROVED_LUCIDE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      expect(icon.mode, name).toBe("line");
      if (icon.mode !== "line") continue;
      expect(icon.provenance, name).toEqual({
        kind: "licensed-authored",
        source: `lucide-react v${APPROVED_LUCIDE_VERSION} · ${APPROVED_LUCIDE_SOURCES[name]}`,
        license: APPROVED_LUCIDE_LICENSE,
      });
      expect(icon.paths, name).toEqual(APPROVED_LUCIDE_GLYPHS[name].paths);
    }
  });

  it("KeyRound 的 authored 实心圆点不会在 path-only 固化时丢失", () => {
    const key = getAwakenIcon("key");
    expect(key.mode).toBe("line");
    if (key.mode !== "line") return;
    expect(key.filledPathIndexes).toEqual([1]);
  });

  it("实心字形只给产品真正需要的概念做", () => {
    const filled = AWAKEN_LINE_ICON_NAMES.filter((name) => {
      const icon = getAwakenIcon(name);
      return icon.mode === "line" && icon.filled;
    });
    // 只有两个概念需要实心：
    // - stop：描边版在 16px 下读起来像一个空框，与「发送」的视觉重量不成对比；
    // - play：播放三角是约定俗成的实心形，描边版会被读成「下一个」箭头。
    // 新增实心字形必须在这里显式登记，避免实心层悄悄长大。
    expect(filled).toEqual(["play", "stop"]);
  });

  it("品牌标记保留 authored paths，并声明可读下限", () => {
    for (const name of AWAKEN_BRAND_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      expect(icon.mode).not.toBe("line");
      if (icon.mode === "line") continue;
      // 品牌标记不使用 16 网格 viewBox：保留 authored paths，只允许对称透明留白。
      expect(icon.viewBox).not.toBe(`0 0 ${AWAKEN_ICON_GRID} ${AWAKEN_ICON_GRID}`);
      expect(icon.minSize).toBeGreaterThanOrEqual(14);
    }
  });
});

describe("概念表", () => {
  it("每个概念都指向一个已注册图标", () => {
    for (const [concept, name] of Object.entries(AWAKEN_ICON_CONCEPTS)) {
      expect(isAwakenIconName(name), `概念 ${concept} 指向未注册图标 ${name}`).toBe(true);
    }
  });

  it("一概念一图标：概念名不重复", () => {
    const keys = Object.keys(AWAKEN_ICON_CONCEPTS);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("连接器、Skill、Plugin、CLI 与 MCP 使用当前确定的图形", () => {
    expect(AWAKEN_ICON_CONCEPTS.connector).toBe("connector");
    expect(AWAKEN_ICON_CONCEPTS.skill).toBe("book-open");
    expect(AWAKEN_ICON_CONCEPTS["skill-active"]).toBe("book-open");
    expect(AWAKEN_ICON_CONCEPTS["tool-terminal"]).toBe("terminal");
    expect(AWAKEN_ICON_CONCEPTS.plugin).toBe("agent-plugins-mark");
    expect(AWAKEN_ICON_CONCEPTS["plugin-default"]).toBe("puzzle");
    expect(AWAKEN_ICON_CONCEPTS["mcp-brand"]).toBe("mcp-mark");
    expect(AWAKEN_ICON_CONCEPTS["mcp-source"]).toBe("mcp-mark");
  });

  it("记忆恢复 PR #36 前的 Brain 图形，并由 Design System 持有来源", () => {
    expect(AWAKEN_ICON_CONCEPTS["settings-memory"]).toBe("brain");
    const brain = ICON_DEFINITIONS.brain;
    expect(brain.build).toBeUndefined();
    expect(brain.authored?.source).toContain("lucide-react v0.468.0");
    expect(brain.authored?.license).toBe("ISC");
  });

  it("插件默认图标使用 Heroicons 拼图块，并由 Design System 持有来源", () => {
    const puzzle = ICON_DEFINITIONS.puzzle;
    expect(puzzle.build).toBeUndefined();
    expect(puzzle.authored?.source).toContain("heroicons v2");
    expect(puzzle.authored?.license).toBe("MIT");
    expect(getAwakenIcon("puzzle").minSize).toBe(12);
  });
});

describe("已确认产品字形保护", () => {
  it("文件夹、记忆、连接器、Plugin、MCP、Skill 与 CLI 字形保持已确认版本", () => {
    for (const [name, expected] of Object.entries(PRODUCT_APPROVED_ICON_DIGESTS)) {
      const icon = getAwakenIcon(name as keyof typeof PRODUCT_APPROVED_ICON_DIGESTS);
      const value = JSON.stringify({
        mode: icon.mode,
        viewBox: icon.viewBox,
        paths: icon.paths,
        strokeWidth: "strokeWidth" in icon ? icon.strokeWidth : undefined,
        filled: icon.mode === "line" ? icon.filled : undefined,
        minSize: icon.minSize,
      });
      expect(createHash("sha256").update(value).digest("hex"), name).toBe(expected);
    }
  });
});

describe("越层守卫", () => {
  it("Design System 与 Desktop 全树都不再依赖 lucide-react", () => {
    // 迁移已完成，因此守卫范围从图标层扩到全树：界面图形一律走 AwakenIcon。
    // `lucide-react` 也已从 package.json 移除，所以这条同时是「装不上就 import 不了」
    // 的双保险 —— 但保留断言，避免有人重新加回依赖。
    const files = [...walk(DS_SRC), ...walk(DESKTOP_SRC)].filter((f) => !f.endsWith("awakenIconLibrary.test.ts"));
    expect(files.length).toBeGreaterThan(0);
    const offenders = files.filter((f) => /from\s+["']lucide-react["']/.test(readFileSync(f, "utf8")));
    expect(offenders.map((f) => path.relative(REPO_ROOT, f))).toEqual([]);
    const designSystemPackage = JSON.parse(
      readFileSync(path.join(REPO_ROOT, "Awaken Design System", "package.json"), "utf8"),
    ) as { dependencies?: Record<string, string> };
    const desktopPackage = JSON.parse(
      readFileSync(path.join(REPO_ROOT, "apps", "desktop", "package.json"), "utf8"),
    ) as { dependencies?: Record<string, string> };
    expect(designSystemPackage.dependencies?.["lucide-react"]).toBeUndefined();
    expect(desktopPackage.dependencies?.["lucide-react"]).toBeUndefined();
  });

  it("图标层不依赖 Electron、runtime 或产品 store", () => {
    const files = walk(path.join(DS_SRC, "icons"));
    const banned = /from\s+["'](electron|\.\.\/\.\.\/\.\.\/|@awaken\/desktop)/;
    const offenders = files.filter((f) => banned.test(readFileSync(f, "utf8")));
    expect(offenders.map((f) => path.relative(REPO_ROOT, f))).toEqual([]);
  });
});

describe("CSS 不得覆盖图标描边", () => {
  it("没有任何 CSS 规则给图标 svg 设置 stroke-width", () => {
    // CSS 的 stroke-width 会盖掉 SVG 的同名呈现属性，因此一条
    // `.foo svg { stroke-width: ... }` 就能把全套 1.25 静默改成别的值，
    // 而注册表层的描边不变量测试完全看不到。迁移时真实踩过：
    // 九条规则用 --aw-icon-stroke(1.75) 覆盖了新图标。
    const cssFiles: string[] = [];
    for (const root of [DS_SRC, DESKTOP_SRC]) {
      for (const dir of [root]) {
        const stack = [dir];
        while (stack.length) {
          const cur = stack.pop()!;
          for (const entry of readdirSync(cur)) {
            if (entry === "node_modules" || entry === "dist") continue;
            const full = path.join(cur, entry);
            if (statSync(full).isDirectory()) stack.push(full);
            else if (entry.endsWith(".css")) cssFiles.push(full);
          }
        }
      }
    }
    expect(cssFiles.length).toBeGreaterThan(0);
    const offenders: string[] = [];
    for (const file of cssFiles) {
      const text = readFileSync(file, "utf8");
      for (const m of text.matchAll(/([^{}]*svg[^{}]*)\{([^}]*)\}/g)) {
        if (/\bstroke-width\s*:/.test(m[2])) {
          offenders.push(`${path.relative(REPO_ROOT, file)}: ${m[1].trim().slice(0, 60)}`);
        }
      }
    }
    expect(offenders).toEqual([]);
  });
});

describe("调用点守卫", () => {
  const sources = [...walk(DS_SRC), ...walk(DESKTOP_SRC)].filter((f) => !f.endsWith(".test.ts") && !f.endsWith(".test.tsx"));

  it("AwakenIcon 的 size 只使用五档尺寸档", () => {
    const bad: string[] = [];
    for (const file of sources) {
      const text = readFileSync(file, "utf8");
      if (!text.includes("AwakenIcon")) continue;
      // 匹配 <AwakenIcon ... size={N} ...>
      for (const m of text.matchAll(/<AwakenIcon\b[^>]*?\bsize=\{(\d+(?:\.\d+)?)\}/gs)) {
        const size = Number(m[1]);
        if (!(AWAKEN_ICON_SIZES as readonly number[]).includes(size)) {
          bad.push(`${path.relative(REPO_ROOT, file)} 使用了非档位尺寸 ${size}`);
        }
      }
    }
    expect(bad).toEqual([]);
  });

  it("AwakenIcon 的 size 不低于该图标声明的可读下限", () => {
    const bad: string[] = [];
    for (const file of sources) {
      const text = readFileSync(file, "utf8");
      if (!text.includes("AwakenIcon")) continue;
      for (const m of text.matchAll(
        /<AwakenIcon\b[^>]*?\bname=["']([a-z0-9-]+)["'][^>]*?\bsize=\{(\d+)\}/gs,
      )) {
        const [, name, rawSize] = m;
        if (!isAwakenIconName(name)) continue;
        const icon = getAwakenIcon(name);
        const minSize = icon.mode === "line" ? icon.minSize : icon.minSize;
        if (Number(rawSize) < minSize) {
          bad.push(
            `${path.relative(REPO_ROOT, file)}：${name} 的可读下限是 ${minSize}px，实际用了 ${rawSize}px`,
          );
        }
      }
    }
    expect(bad).toEqual([]);
  });
});
