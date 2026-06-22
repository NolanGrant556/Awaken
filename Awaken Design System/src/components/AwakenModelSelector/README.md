# AwakenModelSelector

This folder contains a standalone Awaken Design System component, its observable demo states, and its Gallery entry.

## Files

- `AwakenModelSelector.tsx` - component implementation.
- `AwakenModelSelector.css` - component-local style entry.
- `AwakenModelSelector.states.ts` - observable states and mock props.
- `AwakenModelSelector.demo.tsx` - standalone Gallery demo.
- `index.ts` - component export.

## Rules

- UI-only: no runtime, Electron, AG-UI, A2UI, or file-system side effects.
- Props in, callbacks out.
- Keep observable states updated when the public API changes.
