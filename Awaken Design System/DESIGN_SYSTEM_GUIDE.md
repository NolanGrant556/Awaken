# Awaken Design System Guide

Awaken uses the TraeWork package in `reference/traework/` as a visual reference, not as a runtime dependency. Keep Awaken component APIs, runtime behavior, and brand assets intact.

## Visual Direction

- Light mode is the first supported target.
- Awaken logo and wordmark are the only primary brand assets.
- Use neutral surfaces, restrained borders, and typography for hierarchy.
- Use black or near-black for the primary action; do not import the TraeWork purple brand system.
- Avoid decorative backgrounds, glow effects, nested cards, and ad hoc color palettes.

## Token Rules

- Preserve existing `--aw-*` variables for compatibility.
- Product text defaults to `14px / 20px`.
- Default icon size is `16px`; compact icons may use `14px`.
- Default control radius is `8px`; default card, popover, and panel radius is `12px`.
- Interactive neutral states must move through `--aw-bg-fill-light`, `--aw-bg-fill-main`, and `--aw-bg-fill-dark`.
- Status colors are semantic only: success, warning, error, running, and permission states.

## Component Rules

- Do not expose TraeWork `.ds-*` classes as Awaken public API.
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

For Desktop visual QA, use the official root command `npm run dev:desktop` through a single `awaken-dev-desktop` screen session.
