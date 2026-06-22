# AwakenPrimaryNav

This folder contains a standalone Awaken Design System component, its observable demo states, and its Gallery entry.

## Files

- `AwakenPrimaryNav.tsx` - component implementation.
- `AwakenPrimaryNav.css` - component-local style entry.
- `AwakenPrimaryNav.states.ts` - observable states and mock props.
- `AwakenPrimaryNav.demo.tsx` - standalone Gallery demo.
- `index.ts` - component export.

## Rules

- UI-only: no runtime, Electron, AG-UI, A2UI, or file-system side effects.
- Props in, callbacks out.
- Keep observable states updated when the public API changes.
