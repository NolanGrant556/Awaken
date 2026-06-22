# AwakenPromptCard

This folder contains a standalone Awaken Design System component, its observable demo states, and its Gallery entry.

## Files

- `AwakenPromptCard.tsx` - component implementation.
- `AwakenPromptCard.css` - component-local style entry.
- `AwakenPromptCard.states.ts` - observable states and mock props.
- `AwakenPromptCard.demo.tsx` - standalone Gallery demo.
- `index.ts` - component export.

## Rules

- UI-only: no runtime, Electron, AG-UI, A2UI, or file-system side effects.
- Props in, callbacks out.
- Keep observable states updated when the public API changes.
