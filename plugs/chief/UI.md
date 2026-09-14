# Shared UI implementation

Follow [the design contract](../../docs/wiki-ui-contract.md).

`ui-styles.js` is the shared styling source for the host and shadow panel. It is an ES module embedded in the Go binary; it needs no separate stylesheet build. `ui-controls.js` supplies anchored action-menu keyboard, focus and dismissal behavior. `chief.js` composes the feature state and native SilverBullet integration.

Use `--iugum-reading-font`, `--iugum-reading-size`, `--iugum-reading-leading`, and `--iugum-code-font` for prose/code. Interface text uses `--ui-font` and `--iugum-ui-size`. `--iugum-space-4/8/12/16` provide shared spacing. Icons use `--iugum-icon-size` and `--iugum-control-size`; focus uses `--iugum-focus-color`. Define overrides in a Space Style. Set `--iugum-editor-font` to a monospace family to retain a monospace wiki editor independently of chat.

Static presentation belongs here, not in element.style. Runtime panel width and tick geometry remain calculated CSS values. Prompt colors retain their documented theme properties. The second theme and preference UI are separate work.

`mount()` retains one instance on ordinary navigation. Its `dispose()` removes its menu handlers, search dismissal listener, observers and mounted host elements. Closing chat is only a detach and does not dispose drafts.

The required frontend gate includes `shared-ui.e2e.mjs` for menu focus, arrows, dismissal and token inheritance, alongside existing feature tests. This stage does not claim the full future visual/accessibility gate or server conversation persistence is implemented.
