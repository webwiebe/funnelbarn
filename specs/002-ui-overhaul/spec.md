# Feature Specification: FunnelBarn UI Overhaul

**Feature Branch**: `002-ui-overhaul`
**Created**: 2026-04-28
**Status**: Planning
**Supersedes**: Portions of `001-analytics-funnel` web dashboard tasks (T-130 through T-147)

---

## Vision

FunnelBarn is a self-hosted analytics and funnel tracking server. As a self-hosted SaaS product aimed at developers, founders, and marketers, the UI needs to be more than functional — it needs to be visually compelling. A product that handles your analytics data should feel as polished as the commercial alternatives it replaces.

**Logged out**: A marketing site that makes a first impression — persuasive, fast, and honest about what the product is.

**Logged in**: Dashboards that are dense with information but visually clear, with live data, animated charts, and the kind of interface that makes you want to open it.

The design language is dark-first, amber-accented, and data-dense — built for developers and power users who appreciate information density done well.

---

## Design Principles

### 1. Dark Mode First

The primary color scheme is dark. This is a deliberate choice: FunnelBarn's target audience is developers who spend their days in dark-themed editors and terminals. Dark mode is not an afterthought; it is the baseline.

Background layers:
- Base: `#0f1117` — near-black, almost blue-black
- Surface: `#1a1d27` — card and panel backgrounds
- Elevated: `#23263a` — modals, dropdowns, popovers
- Border: `#2d3148` — subtle dividers

### 2. Amber Accent

Amber (`#f59e0b`) is the primary action color. It reads as warm and energetic against the cold dark backgrounds, and it differentiates FunnelBarn from the cold blue/purple palettes of commercial analytics tools.

Accent scale:
- `#f59e0b` — primary (buttons, links, active states, chart fills)
- `#fbbf24` — hover (10% lighter)
- `#d97706` — pressed (10% darker)
- `#78350f` — subtle backgrounds, badges

### 3. Data Density

The dashboard is for power users. Aim for a newspaper-style information hierarchy: headlines at a glance, detail on closer inspection. Never waste whitespace. Use compact spacing within data tables. Use sparklines instead of full charts where a trend is sufficient.

### 4. Animations: Subtle and Purposeful

Animations reinforce meaning. A number that increments live communicates real-time data. A chart that draws in on first load signals a fresh fetch. A pulsing dot next to "Live" tells you the connection is active.

Timing:
- Hover transitions: 150ms ease-out
- Page transitions: 300ms ease-in-out
- Chart draw animations: 500ms ease-out (cubic-bezier)
- Live counter ticks: 100ms (fast enough to feel alive, slow enough to read)
- Skeleton loading: 1500ms shimmer loop

### 5. Mobile: Responsive, Desktop-Optimized

The dashboard is designed for desktop-width viewports (1280px+). All pages must be usable on mobile, but the default layout is sidebar + content rather than stacked mobile-first. On screens below 768px, the sidebar collapses to a bottom navigation bar.

---

## Marketing Site (Logged Out)

When a user is not authenticated and visits the root URL (`/`), they see a marketing site. This is the public face of FunnelBarn for anyone evaluating the product.

### Hero Section

**Headline**: "Own your analytics. No subscriptions."

**Sub-headline**: "FunnelBarn is a self-hosted analytics and funnel tracking server. One binary, one SQLite file, all your data on your own infrastructure."

**CTAs**:
- Primary: "Get Started Free" → scrolls to Quick Start section
- Secondary: "View Demo" → opens a live demo embed in a modal or navigates to `/demo`

**Visual**: An animated funnel diagram that plays on load — wide bars representing awareness narrowing to conversion, with percentages appearing on each step. The animation loops slowly. This is not a screenshot; it is a real SVG animation using the actual funnel visualization component.

**Live counter strip** (below hero, above fold): A row of three animated counters:
- "10,000+ events tracked" (static or pulled from a public stats endpoint if available)
- "100% data ownership"
- "Free forever — self-hosted"

These counters count up on first load (starting from 0) to make them feel alive.

### Feature Highlights Grid

A 3-column grid of feature cards on a slightly lighter background section.

| Icon | Title | Description |
|------|-------|-------------|
| Funnel | Funnel Visualization | See exactly where users drop off. Define multi-step funnels by event name, analyze conversion rate per step with color-coded health indicators. |
| Activity | Session Tracking | Cookie-free session fingerprinting. No PII stored. Privacy-compliant by design — GDPR ready without a compliance lawyer. |
| Target | UTM Attribution | Every campaign tracked to its source. First-touch and last-touch models. Compare UTM campaigns side by side. |
| Flask | A/B Tests | Run experiments without a third-party service. Split by event property, compare conversion rates, see statistical significance. |
| Shield | Privacy-First | No IP addresses stored. User IDs one-way hashed with SHA256. No cookies. No cross-site tracking. No third-party scripts. |
| Zap | Real-Time | Watch events arrive in real time. Live session counter, rolling sparklines, event stream ticker — all via SSE, no polling. |
| Box | Single Binary | One Go binary. One SQLite file. No PostgreSQL, no Redis, no Kafka. Runs on a $5 VPS. |
| Globe | Open Source | MIT license. Audit the code. Run it anywhere. No usage limits, no seat pricing, no surprise bills. |

### Quick Start Section

**Headline**: "Up and running in 60 seconds"

Three tabs: Docker / Homebrew / APT

**Docker tab**:
```bash
docker run -d \
  -e FUNNELBARN_API_KEY=your-ingest-key \
  -e FUNNELBARN_JWT_SECRET=change-me \
  -p 8080:8080 \
  -v funnelbarn-data:/data \
  ghcr.io/webwiebe/funnelbarn/service:latest
```
Then visit `http://localhost:8080` to open the dashboard.

**Homebrew tab**:
```bash
brew tap webwiebe/funnelbarn
brew install funnelbarn
funnelbarn project create --name "My Project"
funnelbarn apikey create --project my-project --name "website"
funnelbarn serve
```

**APT tab**:
```bash
curl -fsSL https://funnelbarn.com/install.sh | sh
# or
echo "deb [signed-by=/usr/share/keyrings/funnelbarn.gpg] https://apt.funnelbarn.com stable main" \
  | sudo tee /etc/apt/sources.list.d/funnelbarn.list
sudo apt update && sudo apt install funnelbarn
sudo systemctl enable --now funnelbarn
```

Each code block has a one-click copy button (clipboard icon, top-right of block, amber on hover).

### Pricing Section

**Headline**: "Simple pricing: own it forever"

Two cards side by side:

**Self-Hosted** (highlighted with amber border):
- Price: **$0 / forever**
- All features included
- Unlimited events
- Unlimited projects
- Your infrastructure, your data
- MIT licensed
- CTA: "Download Free"

**Hosted** (dimmed, coming-soon state):
- Price: **Coming Soon**
- Same features, managed infrastructure
- Automatic backups
- CTA: "Join Waitlist" (disabled/grayed, or email capture form)

Below both cards: "No seat limits. No event limits. No upgrade pressure."

### Social Proof Section

A simple strip showing:
- GitHub stars badge (live badge from shields.io, or static number if shields.io is not desired)
- "Trusted by X self-hosted instances" (static initially, can be made dynamic)
- GitHub link: "View source on GitHub →"

Testimonial quotes (can be fictional for initial build, replace with real ones):
> "Finally an analytics tool I can run on my own server without a PhD in Kafka."
> — Developer, indie SaaS

> "GDPR compliance just became a lot easier. One binary, no data leaving our infrastructure."
> — CTO, European startup

### Footer

Four-column layout:
- **Product**: Dashboard, Docs, Changelog, GitHub
- **Install**: Docker, Homebrew, APT, Binary releases
- **Community**: GitHub Issues, Discussions, Twitter/X
- **Legal**: Privacy Policy, License (MIT)

Bottom bar: "FunnelBarn — own your analytics data" with current year copyright.

---

## Auth Flow

### Login Page (`/login`)

Not the default HTML form. A centered card on the dark background with:

- FunnelBarn logo + wordmark at top (amber funnel icon)
- Headline: "Sign in to FunnelBarn"
- Username and password fields with floating labels
- "Sign in" button (full width, amber)
- Error state: red border on fields + error message below CTA
- Subtle animated background: very low-opacity moving gradient or particle effect (optional, can be static initially)

The login page does not show when there is an active session — it redirects immediately to `/dashboard`.

### 401 Interceptor

Any `fetch` call to `/api/v1/*` that returns a 401 triggers a client-side redirect to `/login`. The current URL is preserved as a `?next=` query parameter so the user lands back where they were after logging in.

### First-Run Wizard

When a user logs in for the first time (no projects exist), redirect to `/setup` instead of `/dashboard`.

**Step 1: Welcome**
- Headline: "Welcome to FunnelBarn"
- Sub: "Let's set up your first project"
- CTA: "Get started"

**Step 2: Create Project**
- Project name input
- Auto-generated slug (editable)
- Timezone selector (default: browser timezone)
- CTA: "Create project"

**Step 3: Get Your API Key**
- API key is auto-created with `ingest` scope
- Key displayed once in a monospace code block with copy button
- Warning: "Save this key — it will not be shown again"
- CTA: "I've saved my key — go to dashboard"

**Step 4: Add the Snippet**
- Shows the JS snippet pre-filled with their API key and server URL
- Language tabs: Browser / Node.js / Go / Python
- CTA: "Done — open dashboard"

Progress indicator: four dots at the top (Steps 1–4), current step highlighted in amber.

---

## Dashboard (Logged In)

### Top Navigation Bar

Full-width top bar, `#1a1d27` background:

**Left side**:
- FunnelBarn logo (amber funnel icon + wordmark)
- Project switcher dropdown: shows current project name, clicking opens a dropdown of all projects with search, plus "Create project" at the bottom

**Center** (or right of project switcher):
- Live indicator: pulsing amber dot + "Live" text when events are being received in the last 30 seconds. Dot pulses with a CSS animation. Goes gray when no recent events.

**Right side**:
- Notification bell (future)
- User avatar/initials circle → dropdown (Profile, Settings, Sign out)

### Sidebar Navigation

Vertical sidebar, `#1a1d27` background, 220px wide on desktop:

- Overview (home icon)
- Live Stats (activity icon with pulsing dot when active)
- Funnels (funnel icon)
- A/B Tests (flask icon)
- Attribution (target icon)
- Events (list icon)
- Sessions (users icon)
- Settings (gear icon)

Active item: amber left border + amber text + `#23263a` background.

On mobile: sidebar collapses to bottom navigation bar with icons only.

---

## Overview Page (`/dashboard`)

The main landing page after login. High information density — a founder's morning coffee view.

### Stat Cards Row

Four cards in a row (2×2 on mobile):

| Metric | Value | Sparkline | Delta |
|--------|-------|-----------|-------|
| Events Today | 14,823 | 7-day sparkline | +12% vs yesterday |
| Active Sessions | 47 | 30-min rolling sparkline | Live |
| Top Funnel % | 34.2% | — | vs 31.1% last period |
| Bounce Rate | 58.3% | 7-day sparkline | -3.1% vs last period |

Delta indicators: green for improvement, red for regression, amber for neutral. Small arrow icon (up/down) next to the percentage.

### Time Range Picker

A pill-shaped selector, top-right of the main content area:
- Last 24h | Last 7d | Last 30d | Custom

"Custom" opens a date range popover with two calendar inputs (from / to).

The selected range persists in URL query params (`?from=2026-04-01&to=2026-04-28`) so the page is shareable and bookmarkable.

### Events Over Time Chart

A line chart spanning the full content width. Two series:
- Events (amber line, filled area below with 20% opacity amber)
- Unique sessions (blue-gray line, no fill)

Chart features:
- Hover tooltip showing exact values for that date
- Smooth curve (bezier interpolation)
- Draws in on first load (left-to-right animation, 500ms)
- X axis: dates; Y axis: auto-scaled, formatted with K/M suffixes

Library: Recharts (MIT, well-maintained, works well with React + dark themes).

### Real-Time Event Stream Ticker

A slim horizontal strip below the chart. Scrolls events left-to-right as they arrive via SSE. Each event is a small pill:
- Event name (e.g., "page_view")
- Page URL (truncated)
- Time ago ("2s ago")

Pills fade in from the right and disappear off the left edge after 30 seconds. When no events are arriving, the strip shows "Waiting for events..." in a muted color.

### Top Pages Table

A table with mini bar charts:

| Page | Views | Mini Bar |
|------|-------|----------|
| /dashboard | 3,241 | ████████ |
| /pricing | 1,822 | █████ |
| /docs | 987 | ███ |

The bar in the "Mini Bar" column is a proportional inline bar (CSS background-color trick, not SVG). Width is relative to the row with the most views.

Clicking a page row navigates to a page detail view (future feature — for now, no-op).

### Referrer Breakdown

A donut chart (right side of the row) + a legend table (left side). Shows top 6 referrers by session count. "Other" is a catch-all slice if more than 6 exist.

Colors: amber for top referrer, then a spectrum of muted colors (avoid full rainbow — use analogous hues centered on amber/orange).

### UTM Attribution Table

A compact table below referrers:

| Source | Medium | Campaign | Sessions | Events |
|--------|--------|----------|----------|--------|
| google | cpc | spring-sale | 412 | 1,823 |
| twitter | social | launch | 287 | 934 |

Empty state (no UTM data): "No UTM data for this period. Add `utm_source` to your campaign URLs to see attribution."

---

## Funnel Builder (`/funnels`)

### Funnel List Page

A grid of funnel cards. Each card shows:
- Funnel name
- Step count ("4 steps")
- Top-line conversion rate ("12.4% end-to-end")
- Last analyzed timestamp
- Quick actions: Edit, Analyze, Delete (icon buttons)

"New Funnel" button top-right, amber, with a plus icon.

Empty state: centered illustration (abstract funnel graphic) + "No funnels yet. Create your first funnel to start analyzing conversion."

### Funnel Builder (Create/Edit)

A two-panel layout:
- **Left panel**: Step list editor
- **Right panel**: Live preview of the funnel visualization

**Step List Editor**:
- Each step is a draggable card (drag handle on left, using `@dnd-kit/sortable`)
- Step card contains:
  - Event name input (typeahead that suggests event names seen in the project)
  - Optional property filter (key/value pair, adds a "+" button to add filter)
  - Delete button (trash icon)
- "Add step" button at the bottom (dashed border card, plus icon)
- Minimum 2 steps; maximum 10 steps
- Steps reorder by drag without committing (preview updates live)

**Live Preview**:
- Renders the funnel visualization in real-time as steps are added/edited
- Shows placeholder percentages (100% / ? / ?) until analysis is run
- "Run Analysis" button below the preview (amber, full width of panel)

### Funnel Visualization

The core visual element. A series of horizontal bars, wide-to-narrow, with drop-off annotations.

```
Step 1: Visited Pricing     ████████████████████ 100%   2,410 users
                                                   ↓ -38% dropped
Step 2: Started Signup      ████████████         61.8%   1,490 users
                                                   ↓ -52% dropped
Step 3: Verified Email      █████                29.8%     718 users
                                                   ↓ -18% dropped
Step 4: Completed Onboarding ████               24.4%     589 users
```

Color coding per step:
- > 50% conversion from previous step: green (`#10b981`)
- 20–50% conversion: amber (`#f59e0b`)
- < 20% conversion: red (`#ef4444`)

The bars animate in from left to right on first render (500ms staggered per step, 50ms delay between steps).

Clicking a step bar opens a detail popover with:
- Events in this step (count and percentage)
- Top pages where this event fired
- Time-to-convert from previous step (median, p95)

### Funnel Date Comparison Mode

A toggle ("Compare") in the top-right of the funnel detail page. When enabled, shows two date range pickers (Period A, Period B) and renders the funnel bars side by side in two colors:
- Period A: amber
- Period B: muted blue (`#60a5fa`)

The delta between the two periods is shown for each step: "+3.2pp" in green or "-1.8pp" in red.

### Public Funnel Share Link

A "Share" button in the funnel detail page. Generates a signed URL that renders the funnel in read-only mode without authentication. The shared view shows the funnel visualization and conversion rates but no raw counts (privacy: the share link is safe to send to stakeholders who should not have full dashboard access).

Implementation: a signed token in the URL (`?token=...`) that the API verifies without requiring a session cookie.

---

## Live Stats Page (`/live`)

A dedicated page for real-time data. The UI feels like a mission control display.

### Active Sessions Counter

A large number in the center-top: "47 active sessions right now"

Below it, a 30-second rolling sparkline (tiny, 100px tall line chart) showing how the session count has changed in the last 5 minutes.

The number increments and decrements live as sessions start and end. When a new session arrives, the number briefly flashes amber.

### Real-Time Event Stream

The left half of the page. A scrolling list of events as they arrive via SSE (`GET /api/v1/projects/{id}/stream`).

Each event row:
- Event name (amber badge)
- Page URL (truncated to 40 chars)
- Browser and OS icons (inferred from UA)
- Country flag emoji (if country_code is available)
- "X seconds ago" timestamp (updates live)

Events appear at the top with a slide-in animation and fade out after 60 seconds if the list is too long (keep max 50 visible rows). When no events arrive for 10 seconds, show a subtle "Waiting..." pulsing indicator.

### World Map

The right half of the page. A world map (SVG-based, using a lightweight library such as `react-simple-maps`) showing event origins based on `country_code`.

Countries with recent events (last 5 minutes) glow amber. The intensity of the amber correlates with event count (more events = more saturated amber). Countries with no events are muted dark gray.

When a new event arrives from a country, that country briefly pulses.

### Top Events (Last 5 Minutes)

A compact table below the map:

| Event | Count | Trend |
|-------|-------|-------|
| page_view | 142 | ↑ |
| button_click | 38 | → |
| signup | 7 | ↑ |

Updated every 10 seconds. Count changes animate with a brief highlight flash.

---

## A/B Tests (`/ab-tests`)

### Test List Page

A table of A/B tests:

| Name | Status | Variants | Significance | Start Date |
|------|--------|----------|--------------|------------|
| Pricing CTA color | Running | 2 | 94% | Apr 15 |
| Onboarding step order | Paused | 2 | 78% | Apr 1 |
| Hero headline | Draft | 2 | — | — |

Status badges:
- Running: pulsing green dot + "Running"
- Paused: gray dot + "Paused"
- Concluded: checkmark + "Concluded"
- Draft: pencil + "Draft"

"New A/B Test" button, top-right.

### Create A/B Test

A modal (not a new page) with:

- Test name input
- Description (optional textarea)
- **Variants section**:
  - Control: always present, named "Control"
  - Variant B (and optionally C, D): add via "Add Variant" button
  - Each variant defines a property filter: `event property [key] equals [value]`
    - Example: `variant = "control"` vs `variant = "new-cta"`
- **Goal event**: an event name that represents a conversion for this test (typeahead from known events)
- **Analysis window**: date range for analysis

CTA: "Create Test" (amber button)

### Test Detail Page

Three columns:

**Left: Variant Cards**

One card per variant showing:
- Variant name
- Conversion rate (large number, e.g., "12.4%")
- Total entries
- Total conversions
- Relative uplift vs Control: "+18% uplift" in green, or "-3% vs control" in red

**Center: Conversion Rate Chart**

A horizontal bar chart comparing conversion rates across variants. Bars are color-coded by variant. Confidence interval shown as a thin horizontal line through each bar.

**Right: Statistical Significance**

A gauge-style indicator (semicircle, 0–100%) showing current significance level. Above 95% is green ("Statistically significant"), 80–95% is amber ("Getting there"), below 80% is red ("More data needed").

Below the gauge: "You need approximately X more conversions to reach 95% significance."

### Start/Stop Controls

In the test detail header:
- "Pause Test" button (when running) — stops collecting new data but preserves existing results
- "Resume Test" button (when paused)
- "Conclude Test" button — marks the test as concluded, selects a winner variant (dropdown)
- "Declare Winner" — updates the test record with the winning variant

---

## Settings

### Project Settings (`/settings/project`)

A form with:
- Project name
- Project slug (read-only after creation, or editable with a warning)
- Timezone selector (affects how data is displayed in the dashboard)
- Data retention period (in days; blank = keep forever)
- "Save changes" button (amber)

### API Keys (`/settings/api-keys`)

A table of API keys:

| Name | Scope | Created | Last Used | Actions |
|------|-------|---------|-----------|---------|
| website | ingest | Apr 1 | 2 hours ago | Rotate / Delete |
| admin | full | Mar 15 | Never | Rotate / Delete |

"Create API Key" button opens an inline form:
- Key name
- Scope: `ingest` (track events only) or `full` (all API access)
- CTA: "Create Key"

After creation: the full key is shown once in a modal with a copy button and a warning. Clicking away or closing the modal acknowledges that the key has been saved.

Rotating a key: generates a new random key, shows it once in a modal, invalidates the old key immediately.

Deleting a key: confirmation dialog ("Are you sure? This cannot be undone.") before deletion.

### Data Export (`/settings/export`)

A simple form:
- Date range picker (from / to)
- Format: CSV (only option for now)
- "Export" button → triggers a download of `funnelbarn-export-[project]-[date].csv`

The export streams from the server so large exports do not time out.

### Danger Zone (`/settings/danger`)

A red-bordered section at the bottom of the settings page:
- "Delete all event data for this project" → opens a confirmation modal requiring the user to type the project name
- "Delete this project" → same confirmation, also deletes all associated API keys, events, sessions, and funnels

Both actions are irreversible and the UI makes this very clear.

---

## User Stories

### Story 1 — As a founder, I want to see my signup funnel conversion rate at a glance

**Acceptance criteria**:
- The Overview page shows the top funnel's end-to-end conversion rate in a stat card
- The stat card shows the delta vs the previous period
- Clicking the stat card navigates to the funnel detail page

### Story 2 — As a developer, I want to embed the JS snippet and see events arrive in real time

**Acceptance criteria**:
- The first-run wizard shows a pre-filled JS snippet with the project's API key and server URL
- After embedding, opening the Live Stats page shows events arriving in the event stream within 5 seconds of page actions
- The live indicator in the top nav pulses when events are arriving

### Story 3 — As a marketer, I want to compare UTM campaign performance side by side

**Acceptance criteria**:
- The Attribution page shows a table of UTM sources, mediums, and campaigns with event and session counts
- Selecting two rows (via checkbox) enables a "Compare" button
- The comparison view shows conversion rates for both campaigns side by side with a delta indicator

### Story 4 — As a developer, I want to understand drop-off in my onboarding flow

**Acceptance criteria**:
- I can create a funnel with 4 steps (visited pricing, started signup, verified email, completed onboarding)
- The funnel visualization shows proportional bars for each step
- Drop-off percentages are color-coded (green/amber/red) based on step conversion rate
- I can see time-to-convert median and p95 for each step

### Story 5 — As a founder, I want to see which marketing channels are driving the most signups

**Acceptance criteria**:
- The Attribution page shows top UTM sources sorted by conversion count
- The referrer breakdown donut chart on the Overview page shows the top 6 referring domains
- I can filter both views by date range

### Story 6 — As a developer, I want to set up FunnelBarn in under 2 minutes from the marketing site

**Acceptance criteria**:
- The marketing site Quick Start section has a copyable Docker one-liner
- Following the Docker one-liner and visiting the shown URL lands me on the FunnelBarn login page
- The login page has clear instructions for creating an admin user (link to CLI command)

### Story 7 — As a developer, I want to run an A/B test on my CTA button color

**Acceptance criteria**:
- I can create an A/B test with two variants defined by event property filter (`variant=control` vs `variant=new-cta`)
- The test tracks a goal event (`signup`)
- The test detail page shows conversion rate per variant and a statistical significance indicator
- I can pause and resume the test

### Story 8 — As a developer, I want to see events arriving live without refreshing the page

**Acceptance criteria**:
- The Live Stats page connects to the SSE endpoint on load
- Events appear in the stream within 1 second of being processed
- If the SSE connection drops, the page automatically reconnects with exponential backoff
- A connection status indicator shows "Connected" / "Reconnecting..." / "Disconnected"

### Story 9 — As a developer, I want to export my event data to CSV for offline analysis

**Acceptance criteria**:
- Settings > Data Export shows a date range picker and an Export button
- Clicking Export downloads a CSV file named with the project slug and date range
- Large exports (100k+ rows) do not time out or buffer in the browser

### Story 10 — As a developer, I want to manage API keys without restarting the server

**Acceptance criteria**:
- Settings > API Keys shows all keys for the current project
- I can create a new key and see the full key once immediately after creation
- I can rotate an existing key (generates new value, shows once, invalidates old key)
- I can delete a key with a confirmation step

### Story 11 — As a developer, I want the login page to redirect me back where I was after signing in

**Acceptance criteria**:
- If I visit `/funnels` without being logged in, I am redirected to `/login?next=/funnels`
- After successful login, I am redirected to `/funnels`
- If no `next` parameter exists, redirect goes to `/dashboard`

### Story 12 — As a founder evaluating FunnelBarn, I want to understand what it costs

**Acceptance criteria**:
- The marketing site pricing section makes it clear that self-hosting is free forever
- The hosted plan is shown as "Coming Soon" so I understand there will be a managed option
- There is no ambiguity about limits — "no seat limits, no event limits" is explicit

### Story 13 — As a developer, I want to see where in the world my users are coming from

**Acceptance criteria**:
- The Live Stats page shows a world map with countries highlighted based on recent event origin
- The country data is based on `country_code` in event records (requires GeoIP to be configured server-side)
- If no country data is available, the map shows a "Country data unavailable — enable GeoIP in server config" message

### Story 14 — As a developer, I want to copy the JS tracking snippet from the setup wizard

**Acceptance criteria**:
- The first-run wizard Step 4 shows the snippet with the correct `endpoint` and `apiKey` pre-filled
- A copy button copies the full snippet to clipboard
- Clicking "Done" after copying is the only way to proceed (or skip) — there is no implicit confirmation

### Story 15 — As a founder, I want to set a data retention period so old data is automatically purged

**Acceptance criteria**:
- Settings > Project shows a "Data retention" field (number of days, blank = keep forever)
- Saving the field stores the retention value for the project
- The server's nightly retention job respects this value and deletes events older than N days

### Story 16 — As a developer, I want to delete all data for a specific session (right to erasure)

**Acceptance criteria**:
- Settings > Danger Zone has no UI for individual session erasure (this is a power-user/developer operation done via API)
- The API endpoint `DELETE /api/v1/projects/{id}/sessions/{hash}` is documented in the settings page with a code example

### Story 17 — As a developer, I want to see a changelog on the marketing site

**Acceptance criteria**:
- The marketing site footer has a "Changelog" link
- The changelog page (`/changelog`) lists releases with dates and bullet-point descriptions
- The changelog is statically generated from a markdown file (not a CMS)

### Story 18 — As a developer, I want the dashboard to remember my last-selected project

**Acceptance criteria**:
- The currently selected project is stored in `localStorage` and restored on next visit
- The project is also reflected in the URL (e.g., `/dashboard?project=my-project`) for shareability
- If the URL project does not exist, fall back to localStorage, then to the first project alphabetically

### Story 19 — As a marketer, I want to share a funnel report with my CEO without giving them dashboard access

**Acceptance criteria**:
- The Funnel detail page has a "Share" button that generates a public link
- The public link shows the funnel visualization and conversion rates without requiring login
- The public link shows conversion rates but not raw event counts (protecting sensitive volume data)
- The public link can be revoked from the Funnel settings

### Story 20 — As a developer, I want to know if my analytics tracking has stopped working

**Acceptance criteria**:
- The live indicator in the top nav goes gray if no events have been received in the last 30 seconds
- The Live Stats page event stream shows "Waiting for events..." when idle
- (Future) An alert rule can be configured to send an email if event count drops to zero for N minutes

---

## Acceptance Criteria by Feature Area

### Marketing Site

- [ ] The hero section renders above the fold on a 1280px viewport without scrolling
- [ ] The live demo CTA opens either an inline demo embed or navigates to `/demo`
- [ ] All Quick Start code blocks are copyable (clipboard API, amber feedback on copy)
- [ ] The pricing section clearly distinguishes self-hosted (free) from hosted (coming soon)
- [ ] The footer contains working links to GitHub, Docs, and Changelog
- [ ] The marketing site passes Lighthouse accessibility audit score > 90
- [ ] The marketing site loads in under 2 seconds on a simulated 4G connection

### Auth Flow

- [ ] Login page renders correctly on mobile (320px viewport minimum)
- [ ] Invalid credentials show an error message without a page reload
- [ ] Successful login redirects to the `?next=` URL or `/dashboard`
- [ ] First-run wizard completes in 4 steps without requiring CLI knowledge
- [ ] API key generated in wizard is shown exactly once and has a visible "copy" affordance
- [ ] 401 from any API endpoint triggers a redirect to `/login` within 200ms

### Dashboard

- [ ] Stat cards show correct values matching the API response for the selected date range
- [ ] Changing the time range picker updates all charts and tables without a page reload
- [ ] The real-time ticker shows events arriving within 1 second of the SSE delivering them
- [ ] The top pages table renders with proportional mini bars
- [ ] The events over time chart animates on first load
- [ ] The referrer donut chart shows a max of 6 slices plus "Other"

### Funnels

- [ ] Creating a funnel with drag-and-drop reordering works correctly across all major browsers
- [ ] Running analysis updates the funnel visualization with correct conversion rates
- [ ] Color coding (green/amber/red) correctly reflects step conversion thresholds
- [ ] Date comparison mode renders two sets of bars side by side
- [ ] Public share link generates a URL that renders the funnel without authentication

### Live Stats

- [ ] SSE connection is established within 500ms of page load
- [ ] Events appear in the stream within 1 second of server processing
- [ ] Automatic reconnect works after a 5-second network interruption
- [ ] The world map renders and highlights countries within 100ms of receiving an event
- [ ] The active sessions counter updates within 1 second of session changes

### A/B Tests

- [ ] Creating a test with two variants saves correctly and appears in the test list
- [ ] Conversion rate per variant is calculated correctly from event data
- [ ] Statistical significance indicator updates on page refresh
- [ ] Start/Stop/Conclude controls update test status immediately

### Settings

- [ ] Creating an API key shows the full key in a modal exactly once
- [ ] Rotating a key invalidates the old key and shows the new key exactly once
- [ ] Deleting a key requires confirmation (type project name or "delete" to confirm)
- [ ] Data export downloads a valid CSV file for the selected date range
- [ ] "Delete all event data" requires typing the project name and cannot be undone

---

## Technical Notes

### Routing

React Router v6 (`createBrowserRouter`). Routes:

```
/                       → Landing (if logged out) or redirect to /dashboard (if logged in)
/login                  → Login page
/setup                  → First-run wizard (redirects to /dashboard if setup complete)
/dashboard              → Overview
/live                   → Live stats
/funnels                → Funnel list
/funnels/new            → Funnel builder (create)
/funnels/:id            → Funnel detail + visualization
/funnels/:id/edit       → Funnel builder (edit)
/ab-tests               → A/B test list
/ab-tests/new           → Create A/B test modal (opens from list)
/ab-tests/:id           → A/B test detail
/attribution            → UTM attribution report
/events                 → Event list (paginated)
/sessions               → Session list (paginated)
/settings/project       → Project settings
/settings/api-keys      → API key management
/settings/export        → Data export
/settings/danger        → Danger zone
/share/funnel/:token    → Public funnel share (no auth required)
```

### State Management

- Server state: `@tanstack/react-query` for all API calls (caching, background refetch, optimistic updates)
- Client state: React context for auth (session user + selected project)
- URL state: date range, selected project encoded in URL query params
- Local persistence: `localStorage` for project selection, user preferences

### API Communication

All API calls use a thin `fetch` wrapper that:
1. Attaches `credentials: 'include'` for session cookies
2. On 401 response, dispatches a global event that the auth context listens to (triggering redirect to login)
3. On 5xx response, shows a global toast notification ("Server error — please try again")

### SSE (Server-Sent Events)

Live Stats and the event ticker both use SSE via the native `EventSource` API. The `EventSource` is initialized in a React effect with cleanup on unmount. Reconnect logic uses exponential backoff (1s, 2s, 4s, max 30s) via a custom hook `useSSE(url)`.

### Chart Library

Recharts 2.x. Selected for:
- MIT license
- React-native (no imperative API)
- Good dark mode support (fully customizable colors)
- Active maintenance
- Acceptable bundle size (~140KB gzipped)

### Map Library

`react-simple-maps` for the world map on the Live Stats page. Lightweight (SVG-based), no tile server required, works offline.

### Drag and Drop

`@dnd-kit/sortable` for funnel step reordering. Accessible, supports keyboard navigation.

### Icons

Lucide React. MIT licensed, tree-shakeable, consistent stroke-based icon style that reads well at small sizes on dark backgrounds.

### Font

Inter (Google Fonts or self-hosted via `fontsource`). Alternatively Geist (Vercel's open font) for a more modern developer-tool aesthetic. Geist is preferred if the font hosting can be done locally (no Google CDN for privacy reasons — FunnelBarn is a privacy-first product and loading Google Fonts from the CDN would be inconsistent with that positioning).
