import { SegmentRule } from '../../lib/api'

// ─── Funnel templates ─────────────────────────────────────────────────────────

export interface FunnelTemplate {
  name: string
  scope: 'session' | 'page_view'
  steps: string[]
}

export const FUNNEL_TEMPLATES: FunnelTemplate[] = [
  {
    name: 'Page Engagement',
    scope: 'page_view',
    steps: ['page_view', 'page_engaged', 'your_goal'],
  },
  {
    name: 'Lead Capture',
    scope: 'session',
    steps: ['page_view', 'form_submit'],
  },
  {
    name: 'E-commerce',
    scope: 'session',
    steps: ['page_view', 'add_to_cart', 'purchase'],
  },
  {
    name: 'SaaS Signup',
    scope: 'session',
    steps: ['page_view', 'signup_started', 'signup_completed'],
  },
]

// ─── Segment rule fields / operators ──────────────────────────────────────────

export const SEGMENT_FIELD_META: Record<string, { label: string; placeholder: string; hint: string }> = {
  country_code:     { label: 'Country code',       placeholder: 'NL, US, DE, FR…',            hint: 'ISO 3166-1 alpha-2 code from visitor IP' },
  city:             { label: 'City',                placeholder: 'Amsterdam, London…',          hint: 'City resolved from visitor IP (needs geo DB)' },
  device_type:      { label: 'Device type',         placeholder: 'mobile, desktop, tablet',    hint: 'Detected from browser User-Agent' },
  browser:          { label: 'Browser',             placeholder: 'Chrome, Firefox, Safari…',   hint: 'Browser name from User-Agent' },
  os:               { label: 'OS',                  placeholder: 'Windows, macOS, iOS…',       hint: 'Operating system from User-Agent' },
  connection_class: { label: 'Connection class',    placeholder: 'residential, mobile, datacenter', hint: 'Inferred from ASN — datacenter often means bots or VPNs' },
  dark_mode:        { label: 'Dark mode',           placeholder: '1  (dark)  or  0  (light)',  hint: 'Collected by the SDK from prefers-color-scheme' },
  browser_timezone: { label: 'Browser timezone',    placeholder: 'Europe/Amsterdam, UTC…',     hint: 'IANA timezone from Intl.DateTimeFormat, collected by SDK' },
}

export const SEGMENT_FIELDS = Object.keys(SEGMENT_FIELD_META) as (keyof typeof SEGMENT_FIELD_META)[]

export const SEGMENT_OPERATORS = [
  { value: 'eq', label: '= equals' },
  { value: 'neq', label: '≠ not equals' },
  { value: 'contains', label: '⊇ contains' },
  { value: 'not_contains', label: '⊅ not contains' },
  { value: 'is_null', label: '∅ is empty' },
  { value: 'is_not_null', label: '∃ is present' },
] as const

// Starter segment templates — shown when the project has no segments yet.
export const SEGMENT_TEMPLATES: Array<{ name: string; description: string; rules: SegmentRule[] }> = [
  {
    name: 'Mobile visitors',
    description: 'Phones and tablets only',
    rules: [{ field: 'device_type', operator: 'eq', value: 'mobile' }],
  },
  {
    name: 'Desktop visitors',
    description: 'Desktop browsers',
    rules: [{ field: 'device_type', operator: 'eq', value: 'desktop' }],
  },
  {
    name: 'Dark mode users',
    description: 'Users with dark mode enabled',
    rules: [{ field: 'dark_mode', operator: 'eq', value: '1' }],
  },
  {
    name: 'Datacenter / bots',
    description: 'Traffic from cloud IPs — good to exclude',
    rules: [{ field: 'connection_class', operator: 'eq', value: 'datacenter' }],
  },
  {
    name: 'Netherlands',
    description: 'Visitors from NL',
    rules: [{ field: 'country_code', operator: 'eq', value: 'NL' }],
  },
  {
    name: 'Chrome users',
    description: 'Chrome browser only',
    rules: [{ field: 'browser', operator: 'eq', value: 'Chrome' }],
  },
]

export const PRESET_SEGMENTS = [
  { id: 'all', label: 'All', tip: 'All sessions, no filtering applied' },
  { id: 'logged_in', label: 'Logged in', tip: 'Sessions where identify() was called with a user ID. Requires: analytics.identify("user-123") in your tracking code.' },
  { id: 'not_logged_in', label: 'Not logged in', tip: 'Sessions with no user ID. If you never call identify(), all sessions appear here.' },
  { id: 'mobile', label: 'Mobile', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'desktop', label: 'Desktop', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'tablet', label: 'Tablet', tip: 'Detected automatically from the User-Agent header — no code changes needed.' },
  { id: 'new_visitor', label: 'New visitors', tip: 'First-time sessions (1 event only). Tracked automatically via the SDK session in localStorage.' },
  { id: 'returning', label: 'Returning', tip: 'Sessions with more than one event. Tracked automatically via the SDK session in localStorage.' },
]
