# FunnelBarn Design System

**Version**: 1.0
**Created**: 2026-04-28
**Status**: Defined (not yet implemented)

This document defines the visual language for FunnelBarn's UI. It is the single source of truth for all color, typography, spacing, animation, and component decisions. When in doubt, check here first.

---

## Color Palette

All colors are defined as CSS custom properties on `:root` in `web/src/styles/tokens.css`. Use these tokens — never hardcode hex values in component styles.

### Background Layers

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-bg-base` | `#0f1117` | Page background, body |
| `--color-bg-surface` | `#1a1d27` | Cards, sidebar, top nav, panels |
| `--color-bg-elevated` | `#23263a` | Modals, dropdowns, popovers, context menus |
| `--color-bg-hover` | `#2a2d3e` | Row hover, button hover backgrounds |

### Borders

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-border-subtle` | `#2d3148` | Card borders, table dividers, input borders |
| `--color-border-default` | `#3d4166` | Stronger dividers, active input borders |
| `--color-border-emphasis` | `#5a5f8a` | Focus rings, emphasis borders |

### Amber Accent (Primary)

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-amber-50` | `#fffbeb` | Very light amber backgrounds (toasts) |
| `--color-amber-100` | `#fef3c7` | Light amber backgrounds |
| `--color-amber-200` | `#fde68a` | Light amber fills |
| `--color-amber-400` | `#fbbf24` | Hover state, lighter accent |
| `--color-amber-500` | `#f59e0b` | **Primary accent** — buttons, active states, links, chart fills, badges |
| `--color-amber-600` | `#d97706` | Pressed/active accent |
| `--color-amber-700` | `#b45309` | Dark accent variant |
| `--color-amber-900` | `#78350f` | Very dark amber — badge backgrounds, subtle highlights |

### Semantic Colors

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-success` | `#10b981` | Positive deltas, good conversion rates (>50%), success states |
| `--color-success-dim` | `#064e3b` | Success badge backgrounds |
| `--color-warning` | `#f59e0b` | Medium conversion rates (20–50%), warning states (same as amber-500) |
| `--color-warning-dim` | `#78350f` | Warning badge backgrounds |
| `--color-error` | `#ef4444` | Poor conversion rates (<20%), error states, negative deltas |
| `--color-error-dim` | `#7f1d1d` | Error badge backgrounds |
| `--color-info` | `#60a5fa` | Info states, comparison series B color |
| `--color-info-dim` | `#1e3a5f` | Info badge backgrounds |

### Text Colors

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-text-primary` | `#f1f5f9` | Primary text, headlines, values |
| `--color-text-secondary` | `#94a3b8` | Labels, captions, metadata |
| `--color-text-tertiary` | `#64748b` | Placeholder text, disabled, very subtle |
| `--color-text-inverse` | `#0f1117` | Text on amber/light backgrounds |
| `--color-text-accent` | `#f59e0b` | Amber-colored text, active nav links |

### Chart Colors

A sequence for multi-series charts, starting with amber and using analogous/harmonious hues:

| Token | Hex | Usage |
|-------|-----|-------|
| `--color-chart-1` | `#f59e0b` | Primary series, top referrer, first funnel step |
| `--color-chart-2` | `#60a5fa` | Secondary series, comparison B |
| `--color-chart-3` | `#34d399` | Third series |
| `--color-chart-4` | `#f87171` | Fourth series |
| `--color-chart-5` | `#a78bfa` | Fifth series |
| `--color-chart-6` | `#fb923c` | Sixth series |
| `--color-chart-other` | `#475569` | "Other" slice in pie/donut charts |

---

## Typography

### Font Family

**Primary font**: Geist (open source, by Vercel)
- Install via `fontsource`: `@fontsource/geist` and `@fontsource/geist-mono`
- Self-hosted — no Google CDN dependency
- If Geist is not available, fallback to Inter, then system-ui

**Monospace font**: Geist Mono
- Used for: API keys, code blocks, event names in stream, ingest IDs

```css
:root {
  --font-sans: 'Geist', Inter, system-ui, -apple-system, sans-serif;
  --font-mono: 'Geist Mono', 'JetBrains Mono', 'Fira Code', monospace;
}
```

### Type Scale

| Token | Size | Line Height | Weight | Usage |
|-------|------|-------------|--------|-------|
| `--text-xs` | 11px | 16px | 400 | Micro labels, badge text, time-ago |
| `--text-sm` | 13px | 20px | 400 | Table rows, secondary info, captions |
| `--text-base` | 14px | 22px | 400 | Body copy, form inputs, descriptions |
| `--text-md` | 16px | 24px | 500 | Nav items, card labels, sub-headings |
| `--text-lg` | 18px | 28px | 500 | Section headings, card titles |
| `--text-xl` | 24px | 32px | 600 | Page headings, stat card values |
| `--text-2xl` | 32px | 40px | 700 | Hero headline, large metric displays |
| `--text-3xl` | 48px | 56px | 700 | Marketing hero headline |
| `--text-4xl` | 64px | 72px | 800 | Very large display numbers |

### Font Weight Tokens

| Token | Value | Usage |
|-------|-------|-------|
| `--weight-regular` | 400 | Body text, descriptions |
| `--weight-medium` | 500 | Labels, nav items, table headers |
| `--weight-semibold` | 600 | Card titles, section headings |
| `--weight-bold` | 700 | Page titles, stat values |
| `--weight-extrabold` | 800 | Hero headline |

---

## Spacing Scale

Based on a 4px base unit. Use these tokens for all margin, padding, and gap values.

| Token | Value | Usage |
|-------|-------|-------|
| `--space-1` | 4px | Micro gaps (icon-to-label, badge padding) |
| `--space-2` | 8px | Tight spacing (table cell padding, input padding) |
| `--space-3` | 12px | Default inner padding (card padding small) |
| `--space-4` | 16px | Standard spacing (card padding default) |
| `--space-5` | 20px | Slightly loose (section inner padding) |
| `--space-6` | 24px | Medium gaps (between cards, between form fields) |
| `--space-8` | 32px | Large gaps (between sections, chart top margin) |
| `--space-10` | 40px | Extra large (page section vertical padding) |
| `--space-12` | 48px | Section padding |
| `--space-16` | 64px | Large section padding (marketing site) |
| `--space-24` | 96px | Marketing section vertical padding |

---

## Border Radius

| Token | Value | Usage |
|-------|-------|-------|
| `--radius-sm` | 4px | Badges, chips, small buttons |
| `--radius-md` | 6px | Cards, inputs, dropdowns |
| `--radius-lg` | 8px | Modals, panels, stat cards |
| `--radius-xl` | 12px | Large cards, marketing cards |
| `--radius-2xl` | 16px | Marketing hero card, large feature panels |
| `--radius-full` | 9999px | Pills, avatar circles, toggle switches |

---

## Shadows

For use on elevated surfaces in the dark theme. These shadows use dark tones (not white glow).

| Token | Value | Usage |
|-------|-------|-------|
| `--shadow-sm` | `0 1px 3px rgba(0,0,0,0.3)` | Subtle card lift |
| `--shadow-md` | `0 4px 12px rgba(0,0,0,0.4)` | Dropdown, tooltip |
| `--shadow-lg` | `0 8px 24px rgba(0,0,0,0.5)` | Modal, popover |
| `--shadow-amber` | `0 0 12px rgba(245,158,11,0.3)` | Amber accent glow on hover (used sparingly) |

---

## Animation Guidelines

### Timing Tokens

| Token | Duration | Easing | Usage |
|-------|----------|--------|-------|
| `--duration-fast` | 100ms | `ease-out` | Live counter tick, badge flash |
| `--duration-hover` | 150ms | `ease-out` | Hover transitions (color, background, border) |
| `--duration-interact` | 200ms | `ease-in-out` | Button press, dropdown open |
| `--duration-transition` | 300ms | `ease-in-out` | Page transitions, panel slide-in |
| `--duration-chart` | 500ms | `cubic-bezier(0.4, 0, 0.2, 1)` | Chart draw animation, funnel bar appear |
| `--duration-skeleton` | 1500ms | `ease-in-out` | Skeleton shimmer loop (infinite) |

### Animation Principles

**Use animation to communicate meaning, not for decoration.**

1. **Data freshness**: When new data arrives (SSE event, polled refresh), briefly highlight the changed element (flash-amber for 100ms). This tells the user "this value just changed."

2. **Load completion**: Charts animate in left-to-right when data loads. This signals "the data is here." Do not animate charts on every re-render — only on initial mount or when the time range changes.

3. **Live indicators**: The pulsing dot next to "Live" uses a `scale` + `opacity` pulse animation on a 1.5s infinite loop. This communicates "this is connected" without being distracting.

4. **Page transitions**: A subtle 300ms fade + slight upward translate (`translateY(-4px) → translateY(0)`) as pages enter. No full-page slide animations — they feel slow on data-dense dashboards.

5. **No gratuitous motion**: No parallax, no background animations on the dashboard (marketing site hero is the exception). Every animation must serve a purpose. When in doubt, remove it.

### Key Animation Implementations

**Skeleton shimmer** (for loading states):
```css
@keyframes shimmer {
  from { background-position: -200% 0; }
  to   { background-position: 200% 0; }
}
.skeleton {
  background: linear-gradient(
    90deg,
    var(--color-bg-surface) 25%,
    var(--color-bg-elevated) 50%,
    var(--color-bg-surface) 75%
  );
  background-size: 200% 100%;
  animation: shimmer var(--duration-skeleton) ease-in-out infinite;
}
```

**Live pulse** (for the pulsing dot):
```css
@keyframes pulse-dot {
  0%, 100% { transform: scale(1); opacity: 1; }
  50%       { transform: scale(1.5); opacity: 0.6; }
}
.live-dot {
  animation: pulse-dot 1.5s ease-in-out infinite;
}
```

**Chart draw-in** (for Recharts, applied via `isAnimationActive` and custom `animationDuration` prop):
```tsx
<Line isAnimationActive animationDuration={500} animationEasing="ease-out" />
```

**Flash highlight** (for live counter ticks and data updates):
```css
@keyframes flash-amber {
  0%   { background-color: rgba(245,158,11,0.2); }
  100% { background-color: transparent; }
}
.flash { animation: flash-amber var(--duration-fast) ease-out; }
```

---

## Component Inventory

### `Button`

Variants:
- `primary` — amber background (`--color-amber-500`), dark text, hover lightens to `--color-amber-400`
- `secondary` — transparent background, amber border, amber text, hover fills `--color-amber-900` background
- `ghost` — no background or border, amber text on hover, used for icon-only actions
- `danger` — red background (`--color-error`), used for destructive actions in danger zone

Sizes:
- `sm` — `--text-sm`, `--space-2` vertical / `--space-3` horizontal padding, `--radius-sm`
- `md` (default) — `--text-base`, `--space-2` vertical / `--space-4` horizontal, `--radius-md`
- `lg` — `--text-md`, `--space-3` vertical / `--space-6` horizontal, `--radius-md`

States: default, hover, active (pressed), disabled (50% opacity, no-pointer), loading (spinner replaces text, button disabled).

### `Input`

- Dark background (`--color-bg-base`), `--color-border-subtle` border
- Floating label pattern: label starts inside the field and floats above on focus or when filled
- Focus state: `--color-border-default` border + `--shadow-amber` glow on focus ring
- Error state: `--color-error` border, error message below field in `--text-sm` `--color-error`
- Disabled state: 50% opacity, no cursor

### `Select`

Styled to match `Input` (floating label, same focus/error states). Custom dropdown using a controlled popover (not native `<select>`) for full dark-mode control.

### `Badge`

Variants map to semantic colors:

| Variant | Background | Text | Border | Usage |
|---------|-----------|------|--------|-------|
| `amber` | `--color-amber-900` | `--color-amber-400` | `--color-amber-700` | Primary badge, "ingest" scope |
| `green` | `--color-success-dim` | `--color-success` | (none) | "Running" status, good conversion |
| `red` | `--color-error-dim` | `--color-error` | (none) | Error states, poor conversion |
| `blue` | `--color-info-dim` | `--color-info` | (none) | Info badges, "full" scope |
| `gray` | `--color-bg-elevated` | `--color-text-secondary` | `--color-border-subtle` | Neutral, "Paused" status, "Draft" |

Sizes: `sm` (11px, `--space-1` padding) and `md` (13px, `--space-1`/`--space-2` padding). Rounded with `--radius-sm`.

### `Card`

- Background: `--color-bg-surface`
- Border: `1px solid --color-border-subtle`
- Border radius: `--radius-lg`
- Padding: `--space-4` (default) or `--space-6` (comfortable)
- Shadow: `--shadow-sm`
- Hover variant: adds `--shadow-md` and border transitions to `--color-border-default`

### `StatCard`

Extends `Card`. Layout:
- Top row: metric label (`--text-sm`, `--color-text-secondary`) + optional icon
- Middle: metric value (`--text-2xl`, `--weight-bold`, `--color-text-primary`) + optional sparkline (right-aligned, 60px wide)
- Bottom row: delta badge (green/red/amber) + "vs last period" label

### `Chart`

Not a component itself — Recharts is used directly. But these defaults apply everywhere:

- Background: transparent (inherits `Card` background)
- Grid lines: `--color-border-subtle` at 30% opacity
- Axis labels: `--color-text-tertiary`, `--text-xs`
- Tooltip: `--color-bg-elevated` background, `--color-border-default` border, `--shadow-md`
- Primary series: `--color-chart-1` (amber)
- Secondary series: `--color-chart-2` (blue)
- Animation: `isAnimationActive={true}`, duration 500ms, easing `ease-out`

### `Table`

- Header row: `--color-bg-elevated` background, `--text-sm` `--weight-medium` `--color-text-secondary`
- Data rows: alternating (or flat — TBD based on density preference) with `--color-text-primary`
- Row hover: `--color-bg-hover` background, 150ms transition
- Row height: 40px (compact) or 48px (default)
- No outer border; subtle bottom border on each row (`1px solid --color-border-subtle`)

### `Modal`

- Overlay: `rgba(0,0,0,0.6)` backdrop with `backdrop-filter: blur(4px)`
- Modal container: `--color-bg-elevated` background, `--radius-xl`, `--shadow-lg`
- Max width: 480px (small), 640px (medium), 800px (large)
- Header: title (`--text-lg`, `--weight-semibold`) + close button (ghost, X icon)
- Footer: action buttons right-aligned
- Enter animation: fade-in + scale from 95% to 100% in 200ms

### `Tooltip`

- Background: `--color-bg-elevated`
- Border: `1px solid --color-border-default`
- Shadow: `--shadow-md`
- Font: `--text-sm`, `--color-text-primary`
- Max width: 240px with text wrapping
- Delay: 400ms show delay (prevents tooltips flashing on mouse-over)
- Animation: 150ms fade-in

### `Toast`

- Positioned: top-right, 16px from edges
- Width: 320px
- Background: `--color-bg-elevated`, left-border in semantic color (green for success, red for error, amber for warning, blue for info)
- Shadow: `--shadow-lg`
- Auto-dismiss: 4 seconds
- Dismiss on click
- Stacks vertically when multiple toasts are active (max 3 visible)
- Enter: slide in from right (300ms); exit: fade out + slide right (200ms)

---

## Icon Library

**Lucide React** (`lucide-react`, MIT license, tree-shakeable)

Usage guidelines:
- Default size: `16px` for inline icons, `20px` for nav icons, `24px` for feature cards
- Stroke width: `1.5` (the Lucide default) — do not change
- Color: inherit from parent (`currentColor`) — do not hardcode icon colors
- Never use icons larger than `32px` in the dashboard (marketing site feature section can use 32px)

Canonical icon assignments for the app:

| Icon name | Lucide component | Usage |
|-----------|-----------------|-------|
| Dashboard/overview | `LayoutDashboard` | Sidebar overview link |
| Live stats | `Activity` | Sidebar live link, live indicator |
| Funnels | `Filter` | Sidebar funnels link, funnel cards |
| A/B tests | `FlaskConical` | Sidebar A/B tests link |
| Attribution | `Target` | Sidebar attribution link |
| Events | `List` | Sidebar events link |
| Sessions | `Users` | Sidebar sessions link |
| Settings | `Settings` | Sidebar settings link |
| API keys | `Key` | API keys page, key badge |
| Copy | `Copy` | Copy-to-clipboard buttons |
| Check (copy success) | `Check` | Copy success feedback |
| Delete | `Trash2` | Delete actions |
| Edit | `Pencil` | Edit actions |
| Share | `Share2` | Share funnel button |
| Download | `Download` | Export button |
| Add | `Plus` | Create/add buttons |
| Close/dismiss | `X` | Modal close, toast dismiss |
| Chevron down | `ChevronDown` | Dropdown indicators |
| Arrow up/down | `ArrowUp` / `ArrowDown` | Delta indicators in stat cards |
| Warning | `AlertTriangle` | Warning states |
| Error | `XCircle` | Error states |
| Success | `CheckCircle2` | Success states |
| Info | `Info` | Info tooltips |
| Globe | `Globe` | World map section header |
| Zap | `Zap` | Real-time / performance features |
| Shield | `Shield` | Privacy features |
| Box | `Box` | Single binary feature |

---

## Layout Grid

### Dashboard Layout

```
┌────────────────────────────────────────────────────┐
│ TopNav (full width, 56px height)                    │
├────────┬───────────────────────────────────────────┤
│        │                                            │
│ Side   │  Main content area                         │
│ bar    │  max-width: 1400px, centered               │
│ 220px  │  padding: 24px 32px                        │
│        │                                            │
└────────┴───────────────────────────────────────────┘
```

Breakpoints:
- `sm`: 640px — mobile portrait
- `md`: 768px — mobile landscape / tablet (sidebar collapses)
- `lg`: 1024px — small laptop
- `xl`: 1280px — standard laptop (design target)
- `2xl`: 1536px — large desktop

### Marketing Site Layout

Full-width sections with centered content containers. Content max-width: `1200px`. Section padding: `96px 32px` on desktop, `64px 16px` on mobile.

---

## Accessibility

- All interactive elements have visible focus states (amber outline, `outline-offset: 2px`)
- Color is never the only means of conveying information (conversion color coding also includes text percentage)
- All charts have accessible descriptions via `aria-label` or `aria-describedby`
- Icons used as buttons always have `aria-label`
- Modal focus is trapped when open; focus returns to trigger on close
- All form inputs have associated `<label>` elements
- Minimum contrast ratio: 4.5:1 for normal text, 3:1 for large text (WCAG AA)

Note: amber on dark background — verify specific combinations:
- `#f59e0b` on `#1a1d27`: contrast ratio ~7.2:1 (passes AAA)
- `#f59e0b` on `#0f1117`: contrast ratio ~8.1:1 (passes AAA)
- `#94a3b8` on `#0f1117`: contrast ratio ~5.9:1 (passes AA)

---

## Implementation Order

When building components, follow this sequence to avoid blocking:

1. CSS tokens file (`tokens.css`) — all other components depend on this
2. Base reset + global styles (`global.css`)
3. `Button`, `Input`, `Badge`, `Card` — atomic components used everywhere
4. `Modal`, `Toast`, `Tooltip` — overlay components
5. `StatCard`, `Table` — dashboard-specific atoms
6. Chart wrapper conventions (Recharts config)
7. Page-level components (Dashboard, Funnels, etc.)
