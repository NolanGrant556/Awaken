# AwakenUserPromptBlock

This folder contains a standalone Awaken Design System component, its observable demo states, and its Gallery entry.

## Files

- `AwakenUserPromptBlock.tsx` - component implementation.
- `AwakenUserPromptBlock.css` - component-local style entry.
- `AwakenUserPromptBlock.states.ts` - observable states and mock props.
- `AwakenUserPromptBlock.demo.tsx` - standalone Gallery demo.
- `index.ts` - component export.

## Rules

- UI-only: no runtime, Electron, AG-UI, A2UI, or file-system side effects.
- Props in, callbacks out.
- Keep observable states updated when the public API changes.
