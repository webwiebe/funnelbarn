import { describe, expect, it } from 'vitest'
import type { Project } from '../../lib/api'
import { filterProjects, groupProjects, menuOrder, sortProjects } from './projectPickerModel'

const p = (id: string, name: string, slug = name.toLowerCase()): Project => ({ id, name, slug, status: 'active' })
const list = [p('3', 'zebra'), p('1', 'Alpha'), p('2', 'beta', 'the-b-site'), p('4', 'Project 10'), p('5', 'Project 2')]

describe('projectPickerModel', () => {
  it('sorts case-insensitively with digits in natural order', () => {
    expect(sortProjects(list).map((x) => x.name)).toEqual(['Alpha', 'beta', 'Project 2', 'Project 10', 'zebra'])
  })

  it('filters on name and slug, ignoring case', () => {
    expect(filterProjects(list, 'ALP').map((x) => x.id)).toEqual(['1'])
    expect(filterProjects(list, 'the-b').map((x) => x.id)).toEqual(['2'])
    expect(filterProjects(list, '  ')).toHaveLength(5)
    expect(filterProjects(list, 'nothing')).toEqual([])
  })

  it('groups favourites first, both groups alphabetical', () => {
    const g = groupProjects(list, ['3', '2'])
    expect(g.favourites.map((x) => x.name)).toEqual(['beta', 'zebra'])
    expect(g.others.map((x) => x.name)).toEqual(['Alpha', 'Project 2', 'Project 10'])
  })

  it('orders the menu favourites first after filtering', () => {
    expect(menuOrder(list, ['3'], 'a').map((x) => x.name)).toEqual(['zebra', 'Alpha', 'beta'])
  })
})
