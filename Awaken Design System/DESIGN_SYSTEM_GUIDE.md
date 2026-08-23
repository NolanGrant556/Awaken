# Awaken Design System Guide

Awaken owns its visual language through its own tokens and components. Keep Awaken component APIs, runtime behavior, and brand assets intact.

## Visual Direction

Awaken is a local-first desktop office agent workbench, not an IDE chrome skin and not a marketing site.

- Light mode is the first supported target. Light surfaces stay cool near-white; dark surfaces stay warm black (`#11110f`). Both themes are the same product with two skins.
- Brand voltage is `--aw-accent` only. Do not introduce a second brand hue.
- `AwakenProductLogo` owns the square product mark and the optional full app-icon composition; `logo-wordmark.svg` owns the horizontal wordmark. They are the only primary brand assets and must not substitute for each other through cropping.
- Use the mark-only asset inside product chrome. The full app-icon PNG (black field + light squircle + mark) is for Dock, installer, and other OS icon surfaces.
- Use neutral surfaces, restrained borders, and typography for hierarchy.
- Use black or near-black for the primary action; brand and accent colors must come from Awaken semantic tokens.
- Avoid decorative backgrounds, glow effects, nested cards, and ad hoc color palettes.
- The thinking orb is the only living ornament. Settings, connectors, catalog, and inspector pages must not add orbit tracks, hero glows, or a second canvas decoration.
- Product pages use one of three templates: Workbench (left nav / center reading / right inspector), Settings (nav + titled content), or Catalog (header + filter + card grid).

## Brand assets

Three assets, never cropped into each other:

1. **Mark** — the two-pillar A only, transparent SVG. In-app sizes `16 / 24 / 32 / 40 / 64`. Color follows `--aw-text-primary`.
2. **App icon** — the authored 256 PNG composition. Dock, Start menu, and installer only.
3. **Wordmark** — welcome empty state, About, and installer covers. Default visual width is `220px` (`200px` at narrow widths).

Clear space around the mark is one quarter of its height. Do not stretch, rotate, recolor the mark to accent purple, add a stroke or shadow, or override the welcome wordmark with an arbitrary `logoSrc`. A full export matrix (favicon, DMG, ICO, document covers) is not a current capability.

## Token Rules

- Preserve existing `--aw-*` variables for compatibility.
- Tokens are split into two classes, kept in separate labeled sections of `tokens/tokens.css`:
  - **Skin tokens** (pure visual: colors and shadow color values) are geometry-safe because they do not participate in layout measurement, but every change still requires visual, light/dark theme, and accessibility acceptance.
  - **Skeleton tokens** (layout-measured: spacing, icon sizes, radius, font metrics, layout widths) are frozen; changing any of them is treated as an interaction change and must go through the middle-region review checklist.
- Each status family (success, warning, error, running) has exactly one base hue; soft bg/border, status bg/text, and diff variants must derive from that base instead of introducing new hues.
- Product text defaults to `14px / 20px`.
- Long-form assistant answers use the reading tokens: `15px / 24px` body, with `22px / 30px`, `18px / 26px`, and `16px / 24px` heading tiers.
- Chrome and helper copy must not go below `12px / 18px`.
- Weight roles are regular `400`, medium `500`, semibold `600`, bold `650`, and heavy `700`; use the role token instead of local `700` / `800` / `820` / `850` values. `650` is the SF Pro optical target and falls back to `600` on other platforms.
- Default icon size is `16px`; compact icons may use `14px`. Icons rendered through `AwakenIcon` accept only the five existing size tokens `12 / 14 / 16 / 20 / 24`.
- Awaken's own line icons are drawn on a 16-unit grid and carry an intrinsic `1.25` stroke, because the stroke participates in optical normalisation and cannot be re-skinned without changing every icon's visual size. All interface graphics go through `AwakenIcon`; `lucide-react` is no longer a dependency. The icon contract lives in `README.md` under 图标库契约.
- Brand marks keep their authored paths; they may add symmetric transparent viewBox padding for optical alignment but do not enter the 16-grid path normalisation pipeline. Filled status graphics keep their authored geometry.
- Compact controls use a `6px` radius, default controls use `8px`, cards / popovers / panels use `12px`, and the composer surface uses `16px`.
- Interactive neutral states must move through `--aw-bg-fill-light`, `--aw-bg-fill-main`, and `--aw-bg-fill-dark`.
- Status colors are semantic only: success, warning, error, running, and permission states.

## Component Rules

- Expose only Awaken `aw-*` classes and `Awaken*` component APIs; do not introduce external design-system class names as public API.
- Keep `*.demo.tsx` and `*.demo.css` behind the Gallery-only entry; production component barrels must not export Demo modules.
- Do not replace React component contracts to chase visual polish.
- Prefer changing tokens and existing `aw-*` classes before editing component behavior.
- Hover, active, focus, selected, and disabled states must preserve layout size.
- X-shaped close controls keep an accessible label but do not render a visible tooltip or native `title`; a visually hidden dialog title must leave the close control aligned to the top-right.
- Text must not overflow controls; use truncation only where the component already implies a single line.
- Model, tool, and other product-styled single-choice lists use `AwakenSelect`: the trigger, menu surface, option spacing, selected background, check icon, focus state, and disabled state stay consistent. Do not expose an operating-system native `<select>` menu for these product choices or add a page-private imitation of the list.

## Scrolling Rules

- Scrollable surfaces may show a narrow thumb as a position indicator, but never render a visible scrollbar track, track piece, corner, arrow button, or colored gutter. The Design System base layer keeps those parts transparent for Desktop and Gallery; a component may only tune the thumb width/contrast or hide the scrollbar entirely.
- New scroll containers must reuse the base scrollbar treatment. Do not add a filled rail, permanent slot background, border, or component-local track color.
- Removing the visible track must not remove wheel, touchpad, keyboard, or draggable-thumb scrolling, and must not change the content container's ownership of overflow.

## Validation

Before shipping visual changes, verify:

- `npm run build:ui`
- `npm --workspace @awaken/design-system run smoke:render`
- `npm --workspace @awaken/desktop run typecheck`
- `npm --workspace @awaken/desktop run test`

Icon changes additionally go through `apps/desktop/src/renderer/awakenIconLibrary.test.ts`. A geometry change shows up as a snapshot diff; update it only after checking the icon at real product sizes, because 12px legibility is the binding constraint and it is not visible in the path data.

For Desktop visual QA on macOS, use the official root commands `npm run open:desktop` and `npm run close:desktop`. Windows currently uses the foreground `npm run dev:desktop` entry. The root `README.md` is the sole authority for current platform entry details.
