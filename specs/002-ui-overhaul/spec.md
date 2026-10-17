# Feature Specification: FunnelBarn UI Overhaul

**Spec**: `specs/002-ui-overhaul/`
**Status**: Partial
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none
**Principles touched**: II, IV
**Created**: 2026-04-28
**Supersedes**: Web dashboard tasks T-130 to T-147 of `001-analytics-funnel`

`design-system.md` (visual tokens) and `tasks.md` (the original task list) sit next to this file. Both predate the tree; where they disagree with `web/`, the tree wins.

---

## Vision

FunnelBarn is a self-hosted analytics and funnel tracking server. Logged out, a visitor sees a marketing page. Logged in, the user sees dense dashboards with live data and charts. The design language is dark-first, amber-accented and data-dense, for developers and power users.

## Design Principles

1. **Dark mode first**: the dashboard is dark by default.
2. **Amber accent**: amber (`#f59e0b`) marks primary actions and active states.
3. **Data density**: compact tables, sparklines where a trend is enough.
4. **Animation with a purpose**: hover and draw-in transitions carry meaning; none decorate.
5. **Responsive, desktop-optimized**: the shell is a sidebar on wide screens and a bottom tab bar below 768 px.

---

## Functional Requirements

### Shell and auth

- **FR-001**: Logged-out visitors at `/` MUST see a marketing page with hero, feature cards, install tabs (Docker, apt, brew) and a footer; the page MUST link to `/login`. Issues: none. Tests: `web/src/pages/Landing.test.tsx::renders the hero headline`, `web/src/pages/Landing.test.tsx::switches install tabs when a different method is selected`, `web/src/pages/Landing.test.tsx::renders a Sign in link pointing to /login`.
- **FR-002**: The login page MUST post the username and password, show an error for invalid credentials, disable the submit button while submitting, and hide the form when SSO is configured. Issues: none. Tests: `web/src/pages/Login.test.tsx::calls login with username and password on submit`, `web/src/pages/Login.test.tsx::shows invalid credentials error on 401`, `web/src/pages/Login.test.tsx::disables the submit button while submitting`, `web/src/pages/Login.test.tsx::does not render the form when SSO is configured`.
- **FR-003**: A `401` from `/api/v1/*` MUST end the client session and send the user to `/login`. Issues: none. [inferred]
- **FR-004**: Protected routes MUST redirect unauthenticated users to `/login`. Issues: none. [inferred]
- **FR-005**: The shell MUST show the brand, project navigation and a bottom tab bar on viewports below 768 px. Issues: none. Tests: `web/src/components/shell/Shell.test.tsx::shows the FunnelBarn brand name`, `web/src/components/shell/Shell.test.tsx::uses projectId in nav link hrefs when provided`.
- **FR-006**: The selected project MUST persist in `localStorage` under `funnelbarn:lastProjectId` and be restored on the next visit. Issues: none. [inferred]

### Dashboard and live data

- **FR-007**: The dashboard page `/dashboard/:projectId` MUST show event, session and bounce statistics, an events-over-time chart (Recharts), top pages, referrers and UTM tables for the selected range. Issues: none. [inferred]
- **FR-008**: The live page `/live/:projectId` MUST show the active-session count and a stream of recent events, connect with `EventSource` first and fall back to polling, and show "Waiting for events" while idle. Issues: none. Tests: `web/src/pages/Live.test.tsx::renders the Live heading`, `web/src/pages/Live.test.tsx::shows the connecting state initially`, `web/src/pages/Live.test.tsx::renders the active sessions count once loaded`.
- **FR-009**: The overview pages `/overview`, `/overview/events` and `/overview/funnels` MUST aggregate across projects. Issues: none. [inferred]

### Funnels

- **FR-010**: The funnel page MUST list a project's funnels and create or edit them in modals with ordered steps and per-step property filters. Issues: none. [inferred]
- **FR-011**: The funnel chart MUST render each step name and user count with a "dropped off" annotation between steps, and a placeholder when the result is empty. Issues: none. Tests: `web/src/components/charts/FunnelChart.test.tsx::shows placeholder text when results array is empty`, `web/src/components/charts/FunnelChart.test.tsx::renders all step names for multiple steps`, `web/src/components/charts/FunnelChart.test.tsx::shows user counts for each step`, `web/src/components/charts/FunnelChart.test.tsx::shows "dropped off" annotation between steps`.

### Settings

- **FR-012**: The settings page MUST manage API keys (create with a scope, show once, delete), project settings, retention and recording settings. Issues: none. [inferred]

### Other pages in the tree

- **FR-013**: The web app MUST also serve the pages Flags, Flag usage, Insights, Flows (Sankey), Sessions (with replay), Event mapping, Integrations and Account. Issues: none. [inferred]

---

## Non-Goals

- Hosted pricing and waitlist pages beyond the landing page.
- Real-time collaboration.

## Drift

| Claim | Source | Tree |
|---|---|---|
| React Router `createBrowserRouter` | Technical Notes, Routing | `App.tsx` uses `BrowserRouter` with `Routes` |
| Route set `/ab-tests`, `/attribution`, `/events`, `/setup`, `/settings/{project,api-keys,export,danger}`, `/share/funnel/:token` | Technical Notes, Routing | None exists. `App.tsx` serves `/overview*`, `/event-mapping`, `/dashboard`, `/funnels`, `/flags`, `/insights`, `/flows`, `/live`, `/sessions`, `/integrations`, `/settings` and `/account` |
| A/B test UI (list, create modal, significance gauge) | A/B Tests section, Stories 7 | No A/B test code in `web/src`; the API has `/api/v1/projects/{id}/abtests` (`internal/api/server.go`, `internal/api/abtests.go`) |
| First-run wizard `/setup` with four steps | Auth Flow | `web/src/components/wizards` exists; a `/setup` route does not |
| Login redirects to `?next=` | Auth Flow, Story 11 | not established |
| Server-sent events at `GET /api/v1/projects/{id}/stream` | Live Stats, Technical Notes | The page opens `EventSource('/api/v1/projects/${projectId}/events?stream=true')` and falls back to polling; `internal/api` has no handler that streams events |
| World map with `react-simple-maps` | Live Stats | Not in `web/package.json` |
| Drag-and-drop with `@dnd-kit/sortable`, `@tanstack/react-query` | Technical Notes | Neither is in `web/package.json` |
| Recharts 2.x | Chart Library | `recharts` ^3.8.1; `d3-sankey` is added for flows |
| Lucide React icons | Icons | `lucide-react` ^1.11.0 |
| Geist or Inter font, self-hosted | Font | `index.css` sets a system font stack |
| Funnel date comparison, step detail popover with median and p95, public share link | Funnel sections, Story 19 | No share route or comparison code found |
| Attribution page and UTM campaign comparison | Story 3 | UTM tables sit in the dashboard; no attribution page |
| Data export `/settings/export` and CSV | Settings, Story 9 | No export code in `web/src` or `internal/api` (spec 001 Drift) |
| Danger zone with delete-all-events | Settings | not established |
| Colour tokens `#0f1117`, `#1a1d27`, `#23263a`, `#2d3148` | Design Principles | not established; `ShellStyles.tsx` and `index.css` hold the tokens |
| Landing has a pricing section, testimonials and changelog | Marketing Site, Story 17 | `Landing.tsx` renders hero, six feature cards, install tabs and footer |
| Install snippet `brew tap webwiebe/funnelbarn && brew install funnelbarn` | Quick Start | The landing page shows `brew install funnelbarn/tap/funnelbarn` |
| Pages list (A/B tests, Attribution, Events) | Sidebar Navigation | The tree has Flags, Insights, Flows, Event mapping, Integrations and Account instead |

## Proof contract

- [ ] `pnpm --filter web test` passes for the Landing, Login, Live, Dashboard and FunnelChart test files.
- [ ] A browser at 767 px width shows the bottom tab bar and at 768 px the sidebar (`web/src/components/shell/ShellStyles.tsx` media queries).
- [ ] The live page keeps updating when `EventSource` fails (polling fallback in `web/src/pages/Live.tsx`).
- [ ] A request answered with `401` lands the user on `/login`.

## Sources

- `web/src/App.tsx`, `web/src/pages/`, `web/src/components/`, `web/src/lib/api.ts`, `web/package.json`
- `internal/api/server.go`
- `specs/002-ui-overhaul/design-system.md`, `specs/002-ui-overhaul/tasks.md`
