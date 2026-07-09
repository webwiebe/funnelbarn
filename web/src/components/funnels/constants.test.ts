import { describe, it, expect } from 'vitest'
import {
  FUNNEL_TEMPLATES,
  SEGMENT_FIELD_META,
  SEGMENT_FIELDS,
  SEGMENT_OPERATORS,
  SEGMENT_TEMPLATES,
  PRESET_SEGMENTS,
} from './constants'

describe('FUNNEL_TEMPLATES', () => {
  it('exposes the four expected templates by name', () => {
    expect(FUNNEL_TEMPLATES.map((t) => t.name)).toEqual([
      'Page Engagement',
      'Lead Capture',
      'E-commerce',
      'SaaS Signup',
    ])
  })

  it('every template has a valid scope and at least two steps', () => {
    for (const t of FUNNEL_TEMPLATES) {
      expect(['session', 'page_view']).toContain(t.scope)
      expect(t.steps.length).toBeGreaterThanOrEqual(2)
      expect(t.steps.every((s) => typeof s === 'string' && s.length > 0)).toBe(true)
    }
  })

  it('Page Engagement is the only page_view scoped template', () => {
    const pageViewScoped = FUNNEL_TEMPLATES.filter((t) => t.scope === 'page_view')
    expect(pageViewScoped).toHaveLength(1)
    expect(pageViewScoped[0].name).toBe('Page Engagement')
  })
})

describe('SEGMENT_FIELD_META / SEGMENT_FIELDS', () => {
  it('derives SEGMENT_FIELDS from the meta keys in order', () => {
    expect(SEGMENT_FIELDS).toEqual(Object.keys(SEGMENT_FIELD_META))
  })

  it('every SEGMENT_FIELDS key exists in SEGMENT_FIELD_META', () => {
    for (const field of SEGMENT_FIELDS) {
      expect(SEGMENT_FIELD_META).toHaveProperty(field)
    }
  })

  it('every meta entry has non-empty label, placeholder and hint', () => {
    for (const meta of Object.values(SEGMENT_FIELD_META)) {
      expect(meta.label.length).toBeGreaterThan(0)
      expect(meta.placeholder.length).toBeGreaterThan(0)
      expect(meta.hint.length).toBeGreaterThan(0)
    }
  })

  it('includes the expected core fields', () => {
    expect(SEGMENT_FIELDS).toContain('country_code')
    expect(SEGMENT_FIELDS).toContain('device_type')
    expect(SEGMENT_FIELDS).toContain('dark_mode')
    expect(SEGMENT_FIELD_META.country_code.label).toBe('Country code')
  })
})

describe('SEGMENT_OPERATORS', () => {
  it('exposes the six expected operator values', () => {
    expect(SEGMENT_OPERATORS.map((o) => o.value)).toEqual([
      'eq',
      'neq',
      'contains',
      'not_contains',
      'is_null',
      'is_not_null',
    ])
  })

  it('every operator has a value and label', () => {
    for (const op of SEGMENT_OPERATORS) {
      expect(op.value.length).toBeGreaterThan(0)
      expect(op.label.length).toBeGreaterThan(0)
    }
  })
})

describe('SEGMENT_TEMPLATES', () => {
  it('has six starter templates with unique names', () => {
    expect(SEGMENT_TEMPLATES).toHaveLength(6)
    const names = SEGMENT_TEMPLATES.map((t) => t.name)
    expect(new Set(names).size).toBe(names.length)
  })

  it('every rule references a known field and a known operator', () => {
    const operatorValues = SEGMENT_OPERATORS.map((o) => o.value)
    for (const template of SEGMENT_TEMPLATES) {
      expect(template.description.length).toBeGreaterThan(0)
      expect(template.rules.length).toBeGreaterThan(0)
      for (const rule of template.rules) {
        expect(SEGMENT_FIELDS).toContain(rule.field)
        expect(operatorValues).toContain(rule.operator)
      }
    }
  })
})

describe('PRESET_SEGMENTS', () => {
  it('starts with the "all" preset', () => {
    expect(PRESET_SEGMENTS[0]).toMatchObject({ id: 'all', label: 'All' })
  })

  it('has unique ids and every entry carries a label and tip', () => {
    const ids = PRESET_SEGMENTS.map((p) => p.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const preset of PRESET_SEGMENTS) {
      expect(preset.label.length).toBeGreaterThan(0)
      expect(preset.tip.length).toBeGreaterThan(0)
    }
  })

  it('includes the device presets', () => {
    const ids = PRESET_SEGMENTS.map((p) => p.id)
    expect(ids).toEqual(
      expect.arrayContaining(['mobile', 'desktop', 'tablet', 'logged_in', 'returning']),
    )
  })
})
