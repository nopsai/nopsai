# Modal Shell

Every dialog that creates or edits an object in NopsAI wears one skin. This
document describes that skin, where it lives, and the rules a feature has to
follow so a dialog opened from Pipelines looks identical to one opened from
Knowledge Context.

The skin lives in `services/ui/src/components/modalShell.css` and is imported
once, from `services/ui/src/main.tsx`, right after `styles.css`. Its React
counterparts are in `services/ui/src/components/WorkflowPrimitives.tsx` and
`services/ui/src/components/WorkflowFormDialog.tsx`.

## The three blocks

A dialog is not a box. It is three detached surfaces stacked on a blurred
overlay, and the card element between them paints nothing at all:

| Element | Class | What it is |
| --- | --- | --- |
| Overlay | `workflow-dialog-shell` | Dimmed, blurred backdrop with two slow ambient gradients behind the dialog. |
| Card | `pipelines-modal-card` | The stack itself: width, max height, entrance animation. No background, border, or shadow. |
| Title pill | `pipelines-modal-header` | A rounded bar carrying the kicker, the title, and the close icon. |
| Canvas | `pipelines-modal-body` | The rounded panel that owns the whole form and scrolls when the form is long. |
| Action bar | `pipelines-modal-footer` | Bare row of pill buttons under the canvas, sitting directly on the overlay. |

`WorkflowFormDialog` renders all five for you and is the preferred entry point.
A dialog that needs its own arrangement can use `WorkflowDialogFrame` directly,
but it must still use these class names, because the shell — not the feature —
paints them.

Because the action bar sits on the overlay rather than on a surface, its ghost
buttons are painted for a dark backdrop in both themes. The primary action is
the only lit surface in the dialog.

## Width

A dialog picks a width with a shell size class:

- (no class) — 640px, the default for a form
- `workflow-dialog--compact` — 30rem, for confirmations and single-field dialogs
- `workflow-form-dialog--wide` / `workflow-dialog--wide` — 52rem
- `workflow-form-dialog--xwide` / `workflow-dialog--xwide` — 68rem

A dialog that genuinely needs its own footprint sets `--modal-max-width` on its
own card class, the way `.kc-document-modal` does for the Knowledge Context
document editor. Sizing with a Tailwind `max-w-*` utility no longer works: the
shell's rule is more specific, and the utility is silently ignored.

## Controls

The shell also owns the in-dialog control set, so settings look the same
wherever they appear:

- `modal-hero` with `modal-hero-input` and `modal-hero-summary` — the object's
  name and summary, typed straight onto the canvas instead of into boxed fields,
  so a dialog opens on the decision that matters.
- `modal-divider` — the fading hairline under the hero.
- `modal-property-grid` with `WorkflowPropertyRow` — two aligned columns of
  label/hint on the left and control on the right, separated by row hairlines
  rather than nested boxes.
- `modal-section-heading` — the small uppercase section title with its icon and
  count badge.
- `WorkflowSegmentedControl` (`modal-segmented`) — a short closed set of
  choices. Real radios stay in the markup, so the group keeps arrow-key
  navigation and accessible names. A button-driven variant, keyed off
  `aria-pressed`, is available for groups that toggle a mode.
- `modal-toggle` — an on/off setting.
- `modal-chip-list` with `modal-chip` — a small multi-select.
- `pipelines-input` — inside the shell, inputs drop the app-wide glass treatment
  and read as recessed fields: one border, one radius, no blur stack.

## Colour and motion

Every colour is a theme token, so the shell holds up in light and dark. The dark
palette is the one that gets reviewed first, because that is where the product
is used. The ambient gradients and the entrance animation are disabled under
`prefers-reduced-motion: reduce`.

## What the tests guard

`services/ui/src/components/modalShell.styles.test.ts` fails the build if a
feature stylesheet repaints one of the three blocks, if the card stops being a
bare stack, if a size stops coming from `--modal-max-width`, or if the dark
palette stops restating the shell's tokens. `WorkflowPrimitives.component.test.tsx`
covers the close button, property rows, and the segmented control.
