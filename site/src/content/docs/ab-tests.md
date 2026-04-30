---
title: A/B Tests
description: Set up control and variant groups, track conversions, and read results with statistical significance.
order: 9
---

# A/B Tests

FunnelBarn's A/B test feature computes conversion rates for a control group and a variant group, then runs a two-proportion z-test to tell you whether the difference is statistically significant.

## How it works

An A/B test has three parts:

1. **Control filter** — an event property expression that identifies control-group events.
2. **Variant filter** — an event property expression that identifies variant-group events.
3. **Conversion event** — the event name that counts as a conversion.

FunnelBarn counts how many distinct sessions in each group fired the conversion event, then computes per-group conversion rates and a z-score.

## Instrumenting your experiment

The simplest approach is to send a `variant` property with every event that belongs to the experiment:

```javascript
// Control group
analytics.track('hero_viewed', { variant: 'control' });

// Variant group
analytics.track('hero_viewed', { variant: 'blue-cta' });
```

When users convert:

```javascript
analytics.track('signup_completed');
```

The conversion event does not need a variant property — FunnelBarn will attribute it to the session that started in each group.

## Creating an A/B test

### Dashboard

1. Go to **A/B Tests** in your project.
2. Click **New Test**.
3. Fill in:
   - **Name** — e.g. "Hero CTA colour"
   - **Control filter** — e.g. `variant=control`
   - **Variant filter** — e.g. `variant=blue-cta`
   - **Conversion event** — e.g. `signup_completed`

### API

```bash
curl -X POST http://localhost:8080/api/v1/projects/my-project/abtests \
  -b cookies.txt \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Hero CTA colour",
    "control_filter": "variant=control",
    "variant_filter": "variant=blue-cta",
    "conversion_event": "signup_completed"
  }'
```

## Reading results

### API

```bash
curl "http://localhost:8080/api/v1/projects/my-project/abtests/test-id/analysis?range=7d" \
  -b cookies.txt
```

**Response:**

```json
{
  "test": { "id": "...", "name": "Hero CTA colour", "status": "running" },
  "control_sample":      5000,
  "control_conversions": 250,
  "variant_sample":      4980,
  "variant_conversions": 310,
  "significant":         true,
  "z_score":             2.31,
  "from": "2024-01-24T00:00:00Z",
  "to":   "2024-01-31T23:59:59Z"
}
```

## Interpreting results

| Field | Description |
|---|---|
| `control_sample` | Sessions in the control group |
| `control_conversions` | Sessions in the control group that converted |
| `variant_sample` | Sessions in the variant group |
| `variant_conversions` | Sessions in the variant group that converted |
| `significant` | `true` if `|z_score| > 1.96` (95% confidence) |
| `z_score` | Two-proportion z-test score |

**Conversion rate** = conversions ÷ sample. Compute it client-side:

```javascript
const controlRate = control_conversions / control_sample;   // e.g. 0.05 = 5%
const variantRate = variant_conversions / variant_sample;   // e.g. 0.062 = 6.2%
const uplift = (variantRate - controlRate) / controlRate;   // e.g. +24%
```

**Statistical significance**: FunnelBarn performs a two-proportion z-test. `significant: true` means the result is unlikely to be random at the 95% confidence level (`z > 1.96`). You should also verify that you have enough sample size before declaring a winner — as a rule of thumb, aim for at least 1 000 sessions per group.

## Time range

The analysis endpoint accepts `range=24h|7d|30d` or custom `from`/`to` RFC3339 timestamps. Use a consistent time range when comparing results over time.

## Experiment design tips

- **Run only one experiment per page** at a time to avoid interaction effects.
- **Randomise on the server** — assign users to groups in your application code and persist the assignment (e.g. in a cookie or user record). This prevents users from switching groups between page loads.
- **Send the variant in every relevant event** — not just the view event. This ensures the filter can identify each session's group even if events are missing.
- **Wait for significance** — do not stop an experiment as soon as one group looks better. Let it run until you have a significant z-score and sufficient sample size.
- **Track the right conversion** — make sure the conversion event is the one that matters for your business goal, not a proxy metric.
