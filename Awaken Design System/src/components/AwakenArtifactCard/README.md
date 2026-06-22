# AwakenArtifactCard

This folder contains a standalone Awaken Design System component, its observable demo states, and its Gallery entry.

## Files

- `AwakenArtifactCard.tsx` - component implementation.
- `AwakenArtifactCard.css` - component-local style entry.
- `AwakenArtifactCard.states.ts` - observable states and mock props.
- `AwakenArtifactCard.demo.tsx` - standalone Gallery demo.
- `index.ts` - component export.

## Rules

- UI-only: no runtime, Electron, AG-UI, A2UI, or file-system side effects.
- Props in, callbacks out.
- Keep observable states updated when the public API changes.
