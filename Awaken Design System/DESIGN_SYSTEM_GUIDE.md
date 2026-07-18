# Awaken Design System Guide

Awaken owns its visual language through its own tokens and components. Keep Awaken component APIs, runtime behavior, and brand assets intact.

## Visual Direction

- Light mode is the first supported target.
- Awaken logo and wordmark are the only primary brand assets.
- Use neutral surfaces, restrained borders, and typography for hierarchy.
- Use black or near-black for the primary action; brand and accent colors must come from Awaken semantic tokens.
- Avoid decorative backgrounds, glow effects, nested cards, and ad hoc color palettes.

## Token Rules

- Preserve existing `--aw-*` variables for compatibility.
- Tokens are split into two classes, kept in separate labeled sections of `tokens/tokens.css`:
  - **Skin tokens** (pure visual: colors and shadow color values) are geometry-safe because they do not participate in layout measurement, but every change still requires visual, light/dark theme, and accessibility acceptance.
  - **Skeleton tokens** (layout-measured: spacing, icon sizes, radius, font metrics, layout widths) are frozen; changing any of them is treated as an interaction change and must go through the middle-region review checklist.
- Each status family (success, warning, error, running) has exactly one base hue; soft bg/border, status bg/text, and diff variants must derive from that base instead of introducing new hues.
- Product text defaults to `14px / 20px`.
- Default icon size is `16px`; compact icons may use `14px`.
- Default control radius is `8px`; default card, popover, and panel radius is `12px`.
- Interactive neutral states must move through `--aw-bg-fill-light`, `--aw-bg-fill-main`, and `--aw-bg-fill-dark`.
- Status colors are semantic only: success, warning, error, running, and permission states.

## Component Rules

- Expose only Awaken `aw-*` classes and `Awaken*` component APIs; do not introduce external design-system class names as public API.
- Do not replace React component contracts to chase visual polish.
- Prefer changing tokens and existing `aw-*` classes before editing component behavior.
- Hover, active, focus, selected, and disabled states must preserve layout size.
- Text must not overflow controls; use truncation only where the component already implies a single line.

## Validation

Before shipping visual changes, verify:

- `npm run build:ui`
- `npm --workspace @awaken/design-system run smoke:render`
- `npm --workspace @awaken/desktop run typecheck`
- `npm --workspace @awaken/desktop run test`

For Desktop visual QA on macOS, use the official root commands `npm run open:desktop` and `npm run close:desktop`. Windows currently uses the foreground `npm run dev:desktop` entry. The root `README.md` is the sole authority for current platform entry details.
