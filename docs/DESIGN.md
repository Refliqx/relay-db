# RelayDB Design System

This document explains **why** the RelayDB interface looks the way it does, not
merely **what** its colours are. Its single purpose: a teammate (or a new
contributor) does not have to guess, and does not silently drift the appearance
back to a "generic AI template" style when adding a new screen.

In short: RelayDB is an internal data tool. It should feel like a work tool used
eight hours a day (calm, dense, precise), not like a marketing page.

---

## 1. Principles

1. **Calm by default.** Colour, motion, and shadow are *signals*, not
   decoration. The majority of cells, rows, and panels must be flat. Colour is
   only used once something needs attention (error, FK cycle, unmasked PII).
2. **One accent only.** The accent (teal) is reserved for: primary actions, the
   active step, selection, and the focus ring. No decorative gradients, no
   second accent.
3. **Depth from lines, not shadows.** The `canvas → surface → raised → overlay`
   stack is separated by 1px hairline borders. Shadows are only for elements
   that genuinely float (command palette, dialogs).
4. **Radius hierarchy, not uniform radius.** Controls 4px, panels 6px, overlays
   10px. No `rounded-full` on buttons/badges (pill spam is a template tell).
5. **Dense but deliberately airy.** Table rows ~30px, text 12–13px, numbers use
   `tabular-nums` so columns line up. Density is a feature, not a problem to be
   patched with whitespace.
6. **Colour is always semantic.** `ok` = success, `warn` = warning, `danger` =
   failure, `info` = link/relationship. Colour is never used "to liven things
   up".
7. **Keyboard-first.** `⌘/Ctrl+K` opens all commands, `1–4` moves between steps,
   `?` opens the shortcut list, `Esc` closes. Every button has a clear
   `:focus-visible`.
8. **Real icons, not emoji.** All icons come from one set (Lucide, 15px, default
   `stroke-width`). Emoji are never a functional icon.
9. **Status must be answerable.** Every state (loading, empty, failed, done)
   has its own appearance, and every failure includes the next step, not just a
   red message.

The nine points above are the deliberate result of rejecting the patterns in
section 7.

---

## 2. Tokens

All colours are defined as CSS variables in `src/app.css` and then mapped to
Tailwind via `@theme inline`. Because that mapping uses `var()`, switching themes
does **not** require a class rebuild: a single `data-theme` attribute on `<html>`
is enough.

> Hard rule: **never write hex/oklch directly in a component.** Use tokens. If you
> need a new nuance, add a token; do not add a stray colour.

### Surfaces & lines

| Token | Utility | Purpose |
| --- | --- | --- |
| `--canvas` | `bg-canvas` | Page background & inner area of tables/logs |
| `--surface` | `bg-surface` | Panels, application header |
| `--raised` | `bg-raised` | Hover rows, table headers, controls |
| `--overlay` | `bg-overlay` | Dialogs/palette (the only shadowed element) |
| `--line` | `border-line` | Hairline separator |
| `--line-strong` | `border-line-strong` | Hover line, graph edge |

### Text

| Token | Utility | Purpose |
| --- | --- | --- |
| `--ink` | `text-ink` | Primary text |
| `--muted` | `text-muted` | Secondary text |
| `--faint` | `text-faint` | Metadata labels, placeholders, hints |

### Semantic

| Token | Utility | Meaning |
| --- | --- | --- |
| `--accent` | `bg-accent` / `text-accent` | Primary actions, active step, selection |
| `--accent-ink` | `text-accent-ink` | Text on top of `--accent` |
| `--ok` | `text-ok` | Success, active read-only, matching checksum |
| `--warn` | `text-warn` | Warning, not-yet-masked PII, FK cycle |
| `--danger` | `text-danger` | Failure, NOT NULL, destructive actions |
| `--info` | `text-info` | Relationships/FK, links |

The concrete values (dark & light mode) live in `:root` and `[data-theme="light"]`
in `src/app.css`. Both are chosen so that `--faint` text stays ≥ ~4.5:1 against
its panel background, because metadata labels are used everywhere.

### Other scales

| Scale | Value | Note |
| --- | --- | --- |
| Radius | `4px` / `6px` / `10px` | controls / panels / overlays, not uniform |
| Text size | `10.5px` label · `11–11.5px` mono data · `12–12.5px` UI · `13.5px` wordmark | mono for technical values, sans for chrome |
| Control height | buttons `32px` (`28px` small), fields `32px` (`26px` small) | consistent so form rows do not "ripple" |
| Shadow | only `var(--shadow-pop)` | for overlays; panels have no shadow |

---

## 3. Component classes

Components do not stack utilities until they are unreadable; recurring patterns
are extracted as classes in `src/app.css` (`@layer components`), so consistency
is guaranteed and Tailwind utilities can still override them when needed.

| Class | For |
| --- | --- |
| `.panel`, `.panel-head`, `.panel-body` | Standard container: surface + border + radius 6px |
| `.btn`, `.btn-primary`, `.btn-ghost`, `.btn-danger`, `.btn-sm`, `.btn-icon` | Buttons; the variant = meaning, not a random colour |
| `.field`, `.field-xs` | Inputs/selects/textareas (selects use a custom chevron) |
| `.chip`, `.chip-accent`, `.chip-ok`, `.chip-warn`, `.chip-danger` | Status/metadata badges |
| `.label`, `.hint` | Small uppercase labels & helper text |
| `.data-table` | Data tables: sticky header, right-aligned numbers, selectable cells |
| `.row-btn` | Clickable list rows (used by the table list) |
| `.kbd` | Shortcut key display |
| `.mono`, `.tnum`, `.selectable` | Mono, aligned numbers, and copyable text |

---

## 4. Density & readability

- Numbers are always `tabular-nums` (`.tnum`) so row/size/duration columns line up.
- Technical values (table names, columns, DSNs, checksums, SQL) use `.mono`.
- Text selection is **off** globally (it feels like a native application), but
  **on** in inputs, `pre`/`code`, `.data-table` cells, and anything with
  `.selectable`, because data must be copyable.
- Table headers are `sticky` so the column context is not lost while scrolling.

---

## 5. Accessibility

- Global `:focus-visible`: 2px `--focus` outline, 2px offset. Keyboard focus is
  always visible; click focus does not distract.
- `@media (prefers-reduced-motion: reduce)` disables animation almost entirely.
- Animation is only used for the "working" state (`animate-spin`,
  `animate-pulse`) and 100–200ms micro-transitions, no decorative motion.
- The command palette & dialogs use `role="dialog"`, `aria-modal`, and arrow
  navigation; the table list uses checkboxes with `aria-label`; graph nodes have
  `role="button"` and respond to `Enter`/`Space`.
- Contrast: primary text & `--muted` are comfortable; `--faint` is kept ≥ ~4.5:1
  because it is used for small labels.

---

## 6. Theme

- Dark theme is the default (matching developer-tool convention), light theme is
  fully supported.
- The choice is stored in `localStorage` (`relaydb.theme`), and an inline script
  in `src/app.html` applies it **before the first paint** so there is no flash.
- UI state (theme, palette, log, shortcuts) lives in `src/lib/stores/ui.svelte.ts`
  and is separate from the domain state (`flow`, `engine`).

---

## 7. Anti-patterns deliberately avoided

This list is the "slop taxonomy" relevant to tools like this. Breaking it makes
the application look like a template, not a product.

| Anti-pattern | Why it is rejected | What is used instead |
| --- | --- | --- |
| Purple→blue gradients / blur blobs | The most characteristic "AI-made" tell; carries no meaning | One flat accent (teal) |
| Glassmorphism (`backdrop-blur` everywhere) | Blurs the hierarchy, hard to read | Hairline borders + surface layers |
| `rounded-2xl` on every element | No sharp/soft contrast | A 4/6/10 radius scale |
| Large shadows on every card | Noisy, blurs elevation | Shadows only for overlays |
| Emoji as icons (✨🚀⚡) | Inconsistent, cannot be `stroke`d to match the theme | One line-icon set (Lucide) |
| Icons inside gradient boxes | Decorative without function | Bare icons, colour follows context |
| Pill buttons everywhere | Looks like a landing page | 4px radius, square badges |
| Glowing/pulsing status dots | Distracting, uninformative | Flat when idle, spinner only when working |
| Cards inside cards | Nested containers with no content | One panel level, `.divider` separators |
| Gradient text / centred hero | Marketing language, not tool language | Left-aligned panel titles sized to content |
| Colourful palette (5+ accents) | Signals become noise | Accent + 4 semantic colours only |
| All text the same size | No hierarchy, tiring on the eyes | A deliberate 10.5 → 13.5px scale |

---

## 8. Adding a new screen

1. Use `.panel` + `.panel-head` + `.panel-body` as the skeleton.
2. Take colour **only** from tokens (§2). If it feels insufficient, add a token,
   not a hex value.
3. For tabular data, use `.data-table` (sticky header, `.tnum` for numbers).
4. For actions, use the existing `.btn-*` variants; do not invent new buttons.
5. Make sure all four states exist: **loading**, **empty**, **failed**,
   **succeeded**.
6. Add an icon from Lucide (15–16px), not an emoji.
7. Test light **and** dark theme, plus keyboard navigation (Tab/Enter/Esc).
