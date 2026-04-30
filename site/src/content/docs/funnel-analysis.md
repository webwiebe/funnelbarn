---
title: Funnel Analysis
description: Define conversion funnels, interpret step-by-step results, and segment by device or traffic source.
order: 8
---

# Funnel Analysis

Funnel analysis shows you how users progress through a sequence of steps — and where they drop off. Use it to measure signup flows, checkout sequences, onboarding steps, or any multi-event user journey.

## What is a funnel?

A funnel is an ordered list of event steps. FunnelBarn counts how many sessions completed each step in sequence. The result is a conversion rate from step N to step N+1, and a drop-off percentage at each step.

Example funnel — "Free trial to paid":

| Step | Event |
|---|---|
| 1 | `page_view` on `/pricing` |
| 2 | `trial_started` |
| 3 | `card_added` |
| 4 | `subscription_activated` |

## Creating a funnel

### Dashboard UI

1. Open your project in the dashboard.
2. Go to **Funnels** and click **New Funnel**.
3. Give it a name and add steps. Each step requires an event name and can have optional property filters.

### API

```bash
curl -X POST http://localhost:8080/api/v1/projects/my-project/funnels \
  -b cookies.txt \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Signup Flow",
    "description": "Landing page to account activated",
    "steps": [
      {"event_name": "page_view"},
      {"event_name": "signup_click"},
      {"event_name": "signup_completed"},
      {"event_name": "account_activated"}
    ]
  }'
```

## Step filters

Each step can have property filters to restrict which events count as matching that step:

```json
{
  "steps": [
    {
      "event_name": "page_view",
      "filters": {"url": "https://example.com/pricing"}
    },
    {
      "event_name": "plan_selected",
      "filters": {"plan": "pro"}
    }
  ]
}
```

Only events with matching property values will count toward that step.

## Running analysis

### Dashboard

Select a time range (last 7d, 30d, or custom) and optionally a segment, then click **Analyze**.

### API

```bash
curl "http://localhost:8080/api/v1/projects/my-project/funnels/funnel-id/analysis?range=30d&segment=mobile" \
  -b cookies.txt
```

**Response:**

```json
{
  "funnel": { "id": "...", "name": "Signup Flow", "steps": [...] },
  "results": [
    { "step": 1, "event_name": "page_view",         "count": 10000, "conversion_rate": 1.0, "drop_off_rate": 0.0 },
    { "step": 2, "event_name": "signup_click",       "count": 2500,  "conversion_rate": 0.25, "drop_off_rate": 0.75 },
    { "step": 3, "event_name": "signup_completed",   "count": 1800,  "conversion_rate": 0.72, "drop_off_rate": 0.28 },
    { "step": 4, "event_name": "account_activated",  "count": 1600,  "conversion_rate": 0.89, "drop_off_rate": 0.11 }
  ],
  "from": "2024-01-01T00:00:00Z",
  "to": "2024-01-31T23:59:59Z"
}
```

## Interpreting results

- **count** — number of unique sessions that reached this step.
- **conversion_rate** — fraction of sessions at step N that also completed step N (relative to the previous step).
- **drop_off_rate** — fraction of sessions from the previous step that did NOT reach this step.

The first step always has `conversion_rate: 1.0` and `drop_off_rate: 0.0` — it is the entry point.

A large drop-off at a particular step is the signal to investigate: is the page slow? Is the form confusing? Is there a bug in the tracking?

## Segmentation

Add a `segment` query parameter to filter the analysis to a subset of sessions:

| Segment | Description |
|---|---|
| `all` | No filter (default) |
| `mobile` | Sessions from mobile devices |
| `desktop` | Sessions from desktop browsers |
| `tablet` | Sessions from tablets |
| `logged_in` | Sessions with a non-null user ID |
| `not_logged_in` | Anonymous sessions |
| `new_visitor` | First-time sessions |
| `returning` | Returning visitor sessions |

Compare mobile vs desktop conversion to identify device-specific friction:

```bash
# Mobile
curl ".../analysis?segment=mobile" -b cookies.txt

# Desktop
curl ".../analysis?segment=desktop" -b cookies.txt
```

## Time range

Use `from` and `to` (RFC3339) for custom ranges:

```bash
curl ".../analysis?from=2024-01-01T00:00:00Z&to=2024-01-07T23:59:59Z" -b cookies.txt
```

Or use the `range` shorthand: `24h`, `7d`, or `30d`.

## Tips for accurate funnels

- Use consistent event names across your SDKs and curl calls.
- Track events as close to the user action as possible to avoid attribution issues.
- For checkout funnels, fire events server-side (Node.js or Python SDK) to avoid lost events from browser navigation.
- Add property filters when the same event name is used for multiple purposes.
