# Task Backlog: FunnelBarn UI Overhaul

**Feature Branch**: `002-ui-overhaul`
**Created**: 2026-04-28
**Status**: Planning
**Spec**: [spec.md](./spec.md)
**Design System**: [design-system.md](./design-system.md)

> All UI tasks below supersede or extend T-130 through T-147 in `001-analytics-funnel/tasks.md`.
> The Go backend is unchanged. These tasks target the React frontend in `web/`.

---

## Phase 1 — Auth + Skeleton (Blocking)

These tasks must be done first. Nothing else can be built without session handling and a layout shell.

- [ ] UI-001: Replace `web/src/App.tsx` routing with `createBrowserRouter`, add auth-aware route guards (`<RequireAuth>` wrapper that redirects to `/login` if no session)
- [ ] UI-002: Implement `web/src/pages/Login.tsx` — branded login form (FunnelBarn logo, amber CTA button, floating label inputs, error state)
- [ ] UI-003: Implement global 401 interceptor in `web/src/lib/api.ts` — on any 401 response, redirect to `/login?next=<current-path>`
- [ ] UI-004: Implement `AuthContext` in `web/src/context/AuthContext.tsx` — stores session user (username, projects list), exposes `login()`, `logout()`, `isLoading` flag
- [ ] UI-005: Implement `ProjectContext` in `web/src/context/ProjectContext.tsx` — stores selected project slug (from URL or localStorage), exposes `setProject()`
- [ ] UI-006: Implement `web/src/pages/Setup.tsx` — 4-step first-run wizard (Welcome → Create Project → Get API Key → Add Snippet)
- [ ] UI-007: Redirect from `/` to `/setup` when logged in with no projects; redirect to `/dashboard` when logged in with projects
- [ ] UI-008: Implement `web/src/components/layout/TopNav.tsx` — logo, project switcher dropdown, live indicator (pulsing dot), user menu
- [ ] UI-009: Implement `web/src/components/layout/Sidebar.tsx` — nav links with active state (amber left border), icons from Lucide React
- [ ] UI-010: Implement `web/src/components/layout/AppShell.tsx` — composes TopNav + Sidebar + `<Outlet />`, handles mobile sidebar collapse to bottom nav
- [ ] UI-011: Write unit tests for `AuthContext` — login sets user, logout clears user, 401 triggers redirect
- [ ] UI-012: Write unit tests for `ProjectContext` — project persists in localStorage, URL param takes priority over localStorage

---

## Phase 2 — Marketing Site

The `/` route when logged out. This is the public face of FunnelBarn.

- [ ] UI-020: Implement `web/src/pages/Landing.tsx` — route renders marketing site when user is not authenticated
- [ ] UI-021: Implement `web/src/components/marketing/Hero.tsx` — headline, sub-headline, primary/secondary CTAs, animated funnel SVG graphic
- [ ] UI-022: Create animated funnel SVG for the hero — SVG bars animate width from 0 to final value on load (CSS animation or Framer Motion), loops slowly
- [ ] UI-023: Implement live counter strip in `Hero.tsx` — three counters that count up from 0 on first mount using a custom `useCountUp` hook
- [ ] UI-024: Implement `web/src/components/marketing/Features.tsx` — 3-column grid of feature cards with Lucide icons, title, description
- [ ] UI-025: Implement `web/src/components/marketing/QuickStart.tsx` — tabbed code blocks (Docker / Homebrew / APT), copy-to-clipboard button with amber feedback
- [ ] UI-026: Implement `web/src/components/marketing/Pricing.tsx` — two pricing cards (Self-Hosted free, Hosted coming-soon), feature list, CTAs
- [ ] UI-027: Implement `web/src/components/marketing/SocialProof.tsx` — GitHub stars badge, testimonial quotes, deployment counter
- [ ] UI-028: Implement `web/src/components/marketing/Footer.tsx` — 4-column links grid, bottom copyright bar
- [ ] UI-029: Implement `web/src/pages/Changelog.tsx` — static changelog page generated from a markdown array in `src/data/changelog.ts`
- [ ] UI-030: Write snapshot tests for Landing page — verify Hero, Features, QuickStart, Pricing, Footer all render without errors

---

## Phase 3 — Core Dashboard

The primary logged-in experience. Prioritize information density and chart quality.

- [ ] UI-040: Implement `web/src/pages/Dashboard.tsx` — full overview layout with stat cards, time range picker, chart, ticker, top pages, referrer donut, UTM table
- [ ] UI-041: Implement `web/src/components/dashboard/StatCard.tsx` — metric value (large), label, sparkline (optional), delta badge (green/red/amber with arrow icon)
- [ ] UI-042: Implement `web/src/components/dashboard/TimeRangePicker.tsx` — pill button group (24h / 7d / 30d / Custom), custom opens date range popover, selected range synced to URL params
- [ ] UI-043: Implement `web/src/components/dashboard/EventsChart.tsx` — Recharts `<ComposedChart>` with two series (events + sessions), amber line + area fill, bezier curves, animated draw-in on mount
- [ ] UI-044: Implement `web/src/components/dashboard/EventTicker.tsx` — connects to SSE, renders scrolling event pills, fade-in from right, auto-removes old events after 30s
- [ ] UI-045: Implement `web/src/components/dashboard/TopPagesTable.tsx` — table with inline proportional bar (CSS, no SVG), sorted by views desc, row click no-op (future: drill-down)
- [ ] UI-046: Implement `web/src/components/dashboard/ReferrerChart.tsx` — Recharts `<PieChart>` with donut mode, max 6 slices + "Other", amber-anchored color spectrum, legend table beside chart
- [ ] UI-047: Implement `web/src/components/dashboard/UTMTable.tsx` — source/medium/campaign columns, session/event counts, empty state message with UTM explainer
- [ ] UI-048: Implement custom hook `web/src/hooks/useDashboard.ts` — wraps react-query calls for all dashboard API endpoints, respects selected project and date range
- [ ] UI-049: Write unit tests for `TimeRangePicker` — selecting a range fires callback with correct from/to dates, custom range validates start < end
- [ ] UI-050: Write unit tests for `StatCard` — renders value, renders delta badge with correct color class, renders without sparkline prop

---

## Phase 4 — Funnels

The flagship feature. Funnel builder with drag-and-drop and visual analysis.

- [ ] UI-060: Implement `web/src/pages/Funnels.tsx` — funnel list page with grid of cards, "New Funnel" button, empty state illustration
- [ ] UI-061: Implement `web/src/components/funnels/FunnelCard.tsx` — name, step count, conversion rate, last-analyzed timestamp, quick action icons (edit, analyze, delete)
- [ ] UI-062: Implement `web/src/pages/FunnelBuilder.tsx` — two-panel layout (step editor left, live preview right), used for both create and edit
- [ ] UI-063: Implement `web/src/components/funnels/StepList.tsx` — draggable step cards using `@dnd-kit/sortable`, step card with event name input, property filter, delete button
- [ ] UI-064: Implement typeahead in step event name input — fetches known event names for the project from `GET /api/v1/projects/{id}/events/names` (or derives from existing event list)
- [ ] UI-065: Implement `web/src/components/funnels/FunnelVisualization.tsx` — horizontal bars, wide-to-narrow, step labels, conversion % with color coding, drop-off annotations, staggered draw-in animation
- [ ] UI-066: Implement color coding logic in FunnelVisualization — > 50% → green, 20–50% → amber, < 20% → red (both bar color and % text)
- [ ] UI-067: Implement step detail popover — click a step bar to open a popover with event count, top pages, time-to-convert (median, p95)
- [ ] UI-068: Implement funnel comparison mode toggle — two date range pickers, side-by-side bars (amber vs muted blue), delta annotations per step
- [ ] UI-069: Implement public funnel share — "Share" button generates a signed token URL, navigates to `/share/funnel/:token`
- [ ] UI-070: Implement `web/src/pages/FunnelShare.tsx` — renders funnel visualization in read-only mode, no auth required, shows conversion rates but hides raw counts
- [ ] UI-071: Write unit tests for `FunnelVisualization` — renders correct bar widths given input data, correct color class for each threshold, drop-off labels show correct percentages
- [ ] UI-072: Write unit tests for `StepList` drag-and-drop — simulating drag events produces correct reorder output (use `@dnd-kit` test utilities)

---

## Phase 5 — Live Stats

Real-time experience. SSE connection, world map, event stream.

- [ ] UI-080: Implement `web/src/pages/LiveStats.tsx` — page layout: active sessions counter (top), left/right split (event stream | world map), top events table (bottom)
- [ ] UI-081: Implement `web/src/hooks/useSSE.ts` — custom hook wrapping `EventSource`, handles connect/disconnect/reconnect with exponential backoff (1s → 2s → 4s → max 30s), exposes `status: 'connected' | 'reconnecting' | 'disconnected'`
- [ ] UI-082: Implement `web/src/components/live/ActiveSessionsCounter.tsx` — large number display, flash-amber animation on increment, 30-second rolling Recharts sparkline
- [ ] UI-083: Implement `web/src/components/live/EventStream.tsx` — scrollable event list, each row with event name badge, URL, browser/OS icons, country flag, time-ago; slide-in animation on new event, max 50 rows (oldest removed)
- [ ] UI-084: Implement `web/src/components/live/WorldMap.tsx` — `react-simple-maps` SVG world map, countries with recent events (last 5 min) colored in amber, intensity proportional to count, pulse animation on new event
- [ ] UI-085: Implement `web/src/components/live/TopEventsMiniTable.tsx` — top 5 events in last 5 minutes, count + trend arrow, updates every 10 seconds, count changes flash-highlight
- [ ] UI-086: Implement connection status indicator in `LiveStats.tsx` header — green dot "Connected" / amber spinning "Reconnecting..." / red dot "Disconnected"
- [ ] UI-087: Write unit tests for `useSSE` — mock `EventSource`, verify reconnect logic fires after disconnect, verify cleanup on unmount
- [ ] UI-088: Write unit tests for `EventStream` — renders incoming events in order, removes events after max count, time-ago displays correctly

---

## Phase 6 — A/B Tests

Experiment tracking and significance calculation.

- [ ] UI-090: Implement `web/src/pages/ABTests.tsx` — test list table with status badges (pulsing dot for Running, gray for Paused, etc.), "New A/B Test" button
- [ ] UI-091: Implement `web/src/components/abtests/CreateTestModal.tsx` — modal with test name, description, variant definitions (property key/value filter per variant), goal event selector, date range, "Create Test" CTA
- [ ] UI-092: Implement `web/src/pages/ABTestDetail.tsx` — three-column layout: variant cards (left), conversion rate bar chart (center), significance gauge (right)
- [ ] UI-093: Implement `web/src/components/abtests/VariantCard.tsx` — variant name, conversion rate (large), total entries, total conversions, relative uplift vs control (colored)
- [ ] UI-094: Implement `web/src/components/abtests/SignificanceGauge.tsx` — semicircle gauge (0–100%), color changes at 80% (amber → green), text showing required additional conversions
- [ ] UI-095: Implement statistical significance calculation in `web/src/lib/stats.ts` — two-proportion z-test, returns `{ significant: boolean, confidence: number, zScore: number }`
- [ ] UI-096: Implement test controls — "Pause", "Resume", "Conclude" buttons in test detail header, each calls the appropriate API endpoint and updates local state optimistically
- [ ] UI-097: Write unit tests for `stats.ts` — known input values (control: 100 conversions / 1000 entries, variant: 120 conversions / 1000 entries) produce expected z-score and significance level
- [ ] UI-098: Write unit tests for `VariantCard` — renders uplift in green when positive, red when negative, "—" when no data

---

## Phase 7 — Settings

Project configuration, API key management, data export, danger zone.

- [ ] UI-100: Implement `web/src/pages/settings/ProjectSettings.tsx` — project name, slug (read-only), timezone selector, retention period input, Save button, react-query mutation on submit
- [ ] UI-101: Implement `web/src/pages/settings/APIKeys.tsx` — table of keys (name, scope, created, last used), Create button, inline create form (name + scope), rotate/delete actions
- [ ] UI-102: Implement `web/src/components/settings/APIKeyRevealModal.tsx` — modal shown once after create or rotate, full key in monospace with copy button, "I've saved my key" CTA to close
- [ ] UI-103: Implement `web/src/pages/settings/DataExport.tsx` — date range picker, format selector (CSV only), Export button that triggers streaming download via `fetch` + blob URL
- [ ] UI-104: Implement `web/src/pages/settings/DangerZone.tsx` — red-bordered section, "Delete all event data" and "Delete project" buttons each open a confirmation modal requiring project name to be typed
- [ ] UI-105: Implement `web/src/components/ui/ConfirmModal.tsx` — reusable confirmation modal with required-text input, confirm button disabled until text matches, used by Danger Zone and API key delete
- [ ] UI-106: Write unit tests for `APIKeys` page — create key shows modal, deleting key requires confirmation, rotating key shows new key in modal

---

## Phase 8 — E2E Tests

Playwright test suite covering critical user flows.

- [ ] UI-110: Set up Playwright in `web/` — `playwright.config.ts` targeting localhost:5173 (Vite dev server), baseline screenshots config
- [ ] UI-111: Auth flow test — visit `/dashboard` unauthenticated → redirected to `/login` → login with valid credentials → redirected to `/dashboard` → logout → back at `/login`
- [ ] UI-112: First-run wizard test — fresh login with no projects → wizard appears → complete all 4 steps → arrive at dashboard with project selected
- [ ] UI-113: Dashboard test — verify stat cards render with non-zero values when seeded event data exists, time range picker updates chart
- [ ] UI-114: Funnel creation test — create 3-step funnel via builder → run analysis → verify visualization renders with 3 bars and color-coded percentages
- [ ] UI-115: Funnel drag-and-drop test — create 3-step funnel → drag step 3 above step 1 → verify order updated before submitting
- [ ] UI-116: Live stats test — open Live Stats page → verify SSE connection status shows "Connected" → verify event stream appears when a test event is sent via API
- [ ] UI-117: A/B test creation test — create test with 2 variants → verify test appears in list with "Draft" status → verify detail page renders variant cards
- [ ] UI-118: API key management test — create API key → verify reveal modal shows full key → close modal → key appears in table → delete key → key removed from table
- [ ] UI-119: Data export test — select date range → click Export → verify browser download starts and file has `.csv` extension

---

## Cross-Cutting Concerns

These are not tied to a specific phase but should be done before the corresponding phase ships.

- [ ] UI-200: Install and configure Recharts, `@dnd-kit/core`, `@dnd-kit/sortable`, `react-simple-maps`, `@tanstack/react-query`, Lucide React, `fontsource/geist` — update `web/package.json`
- [ ] UI-201: Implement design token CSS variables in `web/src/styles/tokens.css` — all color, typography, spacing, and animation values as CSS custom properties
- [ ] UI-202: Implement global reset + base styles in `web/src/styles/global.css` — dark background on `html`, Geist font loaded from fontsource, no default margins/paddings
- [ ] UI-203: Implement `web/src/components/ui/` base components: `Button`, `Input`, `Select`, `Badge`, `Card`, `Modal`, `Tooltip`, `Toast` — all styled with CSS variables from design tokens
- [ ] UI-204: Implement toast notification system — `useToast` hook + `<ToastContainer>` component, positioned top-right, auto-dismisses after 4 seconds
- [ ] UI-205: Implement skeleton loading states for all data-dependent components (stat cards, tables, charts) — shimmer animation using CSS `@keyframes`
- [ ] UI-206: Implement empty states for all list pages (funnels, A/B tests, events, sessions) — centered illustration + explanation text + CTA
- [ ] UI-207: Implement error boundaries — `<ErrorBoundary>` component wrapping each page route, shows "Something went wrong" with a retry button
- [ ] UI-208: Implement mobile responsiveness — test all pages at 375px, 768px, 1280px, 1920px; sidebar collapses to bottom nav at < 768px
