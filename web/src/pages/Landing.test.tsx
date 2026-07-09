import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Landing from './Landing'

// Landing is a largely static marketing page — it has no api/auth/projects
// dependencies, so it only needs a router for its <Link> elements.

function renderLanding() {
  return render(
    <MemoryRouter>
      <Landing />
    </MemoryRouter>,
  )
}

describe('Landing', () => {
  it('renders the hero headline', () => {
    renderLanding()
    expect(screen.getByRole('heading', { name: /own your/i })).toBeInTheDocument()
  })

  it('renders the "Everything you need" features heading', () => {
    renderLanding()
    expect(screen.getByRole('heading', { name: /everything you need/i })).toBeInTheDocument()
  })

  it('renders the install ("Up in minutes") heading', () => {
    renderLanding()
    expect(screen.getByRole('heading', { name: /up in minutes/i })).toBeInTheDocument()
  })

  it('renders all six feature cards', () => {
    renderLanding()
    expect(screen.getByText('Funnel Analysis')).toBeInTheDocument()
    expect(screen.getByText('Real-time Events')).toBeInTheDocument()
    expect(screen.getByText('Privacy First')).toBeInTheDocument()
    expect(screen.getByText('UTM Attribution')).toBeInTheDocument()
    expect(screen.getByText('Single Binary')).toBeInTheDocument()
    expect(screen.getByText('Self-Hosted')).toBeInTheDocument()
  })

  it('renders the Get Started CTA button', () => {
    renderLanding()
    expect(screen.getByRole('button', { name: /get started/i })).toBeInTheDocument()
  })

  it('renders a Sign in link pointing to /login', () => {
    renderLanding()
    const signInLinks = screen.getAllByRole('link', { name: /sign in/i })
    expect(signInLinks.length).toBeGreaterThan(0)
    expect(signInLinks[0]).toHaveAttribute('href', '/login')
  })

  it('renders GitHub links', () => {
    renderLanding()
    const githubLinks = screen.getAllByRole('link', { name: /github/i })
    expect(githubLinks.length).toBeGreaterThan(0)
    expect(githubLinks[0]).toHaveAttribute('href', 'https://github.com/funnelbarn/funnelbarn')
  })

  it('shows the Docker install snippet by default', () => {
    renderLanding()
    expect(screen.getByText(/ghcr\.io\/funnelbarn\/funnelbarn:latest/)).toBeInTheDocument()
  })

  it('switches install tabs when a different method is selected', () => {
    renderLanding()
    // Default tab is Docker; brew snippet should not be visible yet.
    expect(screen.queryByText(/brew install funnelbarn\/tap\/funnelbarn/)).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /^brew$/i }))
    expect(screen.getByText(/brew install funnelbarn\/tap\/funnelbarn/)).toBeInTheDocument()
    // Docker snippet is now hidden.
    expect(screen.queryByText(/ghcr\.io\/funnelbarn\/funnelbarn:latest/)).toBeNull()
  })

  it('switches to the apt install tab', () => {
    renderLanding()
    fireEvent.click(screen.getByRole('button', { name: /^apt$/i }))
    expect(screen.getByText(/curl -sL https:\/\/funnelbarn\.io\/install\.sh/)).toBeInTheDocument()
  })

  it('renders the current year in the footer', () => {
    renderLanding()
    const year = String(new Date().getFullYear())
    const footer = screen.getByText(new RegExp(`${year}.*FunnelBarn`))
    expect(footer).toBeInTheDocument()
  })

  it('renders the brand wordmark in the nav', () => {
    renderLanding()
    const nav = screen.getByRole('navigation')
    expect(within(nav).getByText('Barn')).toBeInTheDocument()
  })
})
