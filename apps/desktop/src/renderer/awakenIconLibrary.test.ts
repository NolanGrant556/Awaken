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
  ICON_DEFINITIONS,
  getAwakenIcon,
  isAwakenIconName,
  opticalVisual,
} from "@awaken/design-system";

const REPO_ROOT = path.resolve(__dirname, "../../../..");
const DS_SRC = path.join(REPO_ROOT, "Awaken Design System", "src");
const DESKTOP_SRC = path.join(REPO_ROOT, "apps", "desktop", "src");

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
  it("每个线性图标贴合它声明的光学形状", () => {
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      if (icon.mode !== "line") continue;
      const [targetW, targetH] = opticalVisual(ICON_DEFINITIONS[name]);
      const [w, h] = icon.measured;
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

  it("描边字形共用同一个描边宽度，实心字形描边为 0", () => {
    for (const name of AWAKEN_LINE_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      if (icon.mode !== "line") continue;
      // 实心字形的视觉包围盒就是几何包围盒，不为描边留半个描边宽度。
      expect(icon.strokeWidth, name).toBe(icon.filled ? 0 : AWAKEN_ICON_STROKE);
    }
  });

  it("实心字形只给产品真正需要的概念做", () => {
    const filled = AWAKEN_LINE_ICON_NAMES.filter((name) => {
      const icon = getAwakenIcon(name);
      return icon.mode === "line" && icon.filled;
    });
    // 停止是唯一需要实心的概念：描边版在 16px 下读起来像一个空框，
    // 与「发送」的视觉重量不成对比。新增实心字形必须在这里显式登记。
    expect(filled).toEqual(["stop"]);
  });

  it("品牌标记保留 authored geometry，并声明可读下限", () => {
    for (const name of AWAKEN_BRAND_ICON_NAMES) {
      const icon = getAwakenIcon(name);
      expect(icon.mode).not.toBe("line");
      if (icon.mode === "line") continue;
      // 品牌标记不使用 16 网格 viewBox —— 它们不参与光学归一化。
      expect(icon.viewBox).not.toBe(`0 0 ${AWAKEN_ICON_GRID} ${AWAKEN_ICON_GRID}`);
      expect(icon.minSize).toBeGreaterThanOrEqual(20);
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
});

describe("越层守卫", () => {
  it("图标层不依赖 lucide-react", () => {
    const files = [...walk(path.join(DS_SRC, "icons")), ...walk(path.join(DS_SRC, "components", "AwakenIcon"))];
    expect(files.length).toBeGreaterThan(0);
    const offenders = files.filter((f) => /from\s+["']lucide-react["']/.test(readFileSync(f, "utf8")));
    expect(offenders.map((f) => path.relative(REPO_ROOT, f))).toEqual([]);
  });

  it("图标层不依赖 Electron、runtime 或产品 store", () => {
    const files = walk(path.join(DS_SRC, "icons"));
    const banned = /from\s+["'](electron|\.\.\/\.\.\/\.\.\/|@awaken\/desktop)/;
    const offenders = files.filter((f) => banned.test(readFileSync(f, "utf8")));
    expect(offenders.map((f) => path.relative(REPO_ROOT, f))).toEqual([]);
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
