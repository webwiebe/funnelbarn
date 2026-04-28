import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { BarChart2, Shield, Zap, GitBranch, Server, TrendingUp } from 'lucide-react'

const C = {
  bg: '#0f1117',
  surface: '#1a1d27',
  border: '#2a2d3a',
  amber: '#f59e0b',
  text: '#e2e8f0',
  muted: '#94a3b8',
}

const features = [
  { icon: <BarChart2 size={22} color="#f59e0b" />, title: 'Funnel Analysis', desc: 'Visualize drop-off at every step. See exactly where users leave your conversion flow.' },
  { icon: <TrendingUp size={22} color="#f59e0b" />, title: 'Real-time Events', desc: 'Live event stream with sub-second latency. Watch traffic happen as it occurs.' },
  { icon: <Shield size={22} color="#f59e0b" />, title: 'Privacy First', desc: 'No third-party scripts. No data leaving your server. Full GDPR compliance by design.' },
  { icon: <GitBranch size={22} color="#f59e0b" />, title: 'UTM Attribution', desc: 'First and last-touch attribution. Track campaigns across the full customer journey.' },
  { icon: <Zap size={22} color="#f59e0b" />, title: 'Single Binary', desc: 'One Go binary, one SQLite file. Zero ops overhead. Runs on a $5 VPS.' },
  { icon: <Server size={22} color="#f59e0b" />, title: 'Self-Hosted', desc: 'Your infrastructure, your rules. No subscriptions, no lock-in, no vendor risk.' },
]

const installTabs = [
  {
    label: 'Docker',
    code: `docker run -d \\
  -p 8080:8080 \\
  -v ./data:/data \\
  -e FUNNELBARN_SECRET=changeme \\
  ghcr.io/funnelbarn/funnelbarn:latest`,
  },
  {
    label: 'apt',
    code: `curl -sL https://funnelbarn.io/install.sh | sudo bash
sudo systemctl enable --now funnelbarn`,
  },
  {
    label: 'brew',
    code: `brew install funnelbarn/tap/funnelbarn
funnelbarn serve`,
  },
]

function InstallTabs() {
  const [active, setActive] = useState(0)

  return (
    <div style={{
      background: C.surface,
      border: `1px solid ${C.border}`,
      borderRadius: 12,
      overflow: 'hidden',
    }}>
      <div style={{
        display: 'flex',
        borderBottom: `1px solid ${C.border}`,
        background: '#13151f',
      }}>
        {installTabs.map((tab, i) => (
          <button
            key={tab.label}
            onClick={() => setActive(i)}
            style={{
              background: 'transparent',
              border: 'none',
              borderBottom: `2px solid ${i === active ? C.amber : 'transparent'}`,
              color: i === active ? C.amber : C.muted,
              padding: '0.75rem 1.25rem',
              cursor: 'pointer',
              fontSize: 14,
              fontWeight: 600,
              transition: 'all 0.15s',
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <pre style={{
        margin: 0,
        padding: '1.5rem',
        color: '#e2e8f0',
        fontSize: 14,
        lineHeight: 1.7,
        overflowX: 'auto',
        fontFamily: '"SF Mono", "Fira Code", monospace',
      }}>
        <code>{installTabs[active].code}</code>
      </pre>
    </div>
  )
}

export default function Landing() {
  const installRef = useRef<HTMLDivElement>(null)

  const scrollToInstall = () => {
    installRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  return (
    <div style={{
      minHeight: '100vh',
      background: C.bg,
      color: C.text,
      fontFamily: 'system-ui, -apple-system, sans-serif',
    }}>
      {/* Nav */}
      <nav style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '1rem 2rem',
        borderBottom: `1px solid ${C.border}`,
        position: 'sticky',
        top: 0,
        background: 'rgba(15,17,23,0.85)',
        backdropFilter: 'blur(12px)',
        zIndex: 100,
      }}>
        <div style={{ fontWeight: 800, fontSize: 18, letterSpacing: '-0.3px' }}>
          Funnel<span style={{ color: C.amber }}>Barn</span>
        </div>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <a href="https://github.com/funnelbarn/funnelbarn" style={{ color: C.muted, fontSize: 14, textDecoration: 'none' }}>
            GitHub
          </a>
          <Link
            to="/login"
            style={{
              background: C.amber,
              color: '#0f1117',
              padding: '0.45rem 1rem',
              borderRadius: 7,
              textDecoration: 'none',
              fontWeight: 700,
              fontSize: 14,
            }}
          >
            Sign in
          </Link>
        </div>
      </nav>

      {/* Hero */}
      <section style={{
        position: 'relative',
        textAlign: 'center',
        padding: '6rem 2rem 5rem',
        overflow: 'hidden',
      }}>
        <div style={{
          position: 'absolute',
          inset: 0,
          backgroundImage: 'radial-gradient(circle, #2a2d3a 1px, transparent 1px)',
          backgroundSize: '32px 32px',
          opacity: 0.5,
          pointerEvents: 'none',
        }} />
        <div style={{
          position: 'absolute',
          top: '10%',
          left: '50%',
          transform: 'translateX(-50%)',
          width: 600,
          height: 300,
          background: 'radial-gradient(ellipse, rgba(245,158,11,0.12) 0%, transparent 70%)',
          pointerEvents: 'none',
        }} />

        <div style={{ position: 'relative' }}>
          <div style={{
            display: 'inline-block',
            background: 'rgba(245,158,11,0.1)',
            border: '1px solid rgba(245,158,11,0.3)',
            borderRadius: 99,
            padding: '0.3rem 0.9rem',
            fontSize: 13,
            color: C.amber,
            fontWeight: 600,
            marginBottom: '1.5rem',
          }}>
            Open Source · Self-Hosted · Privacy First
          </div>

          <h1 style={{
            fontSize: 'clamp(2.5rem, 6vw, 5rem)',
            fontWeight: 900,
            letterSpacing: '-2px',
            lineHeight: 1.05,
            margin: '0 auto 1.5rem',
            maxWidth: 800,
          }}>
            Own Your<br />
            <span style={{ color: C.amber }}>Analytics</span>
          </h1>

          <p style={{
            fontSize: 20,
            color: C.muted,
            maxWidth: 520,
            margin: '0 auto 2.5rem',
            lineHeight: 1.6,
          }}>
            Self-hosted funnel tracking. No subscriptions. Your data, your server.
          </p>

          <div style={{ display: 'flex', gap: 12, justifyContent: 'center', flexWrap: 'wrap' }}>
            <button
              onClick={scrollToInstall}
              style={{
                background: C.amber,
                color: '#0f1117',
                border: 'none',
                borderRadius: 9,
                padding: '0.8rem 1.75rem',
                fontSize: 16,
                fontWeight: 700,
                cursor: 'pointer',
              }}
            >
              Get Started
            </button>
            <Link
              to="/login"
              style={{
                background: 'transparent',
                color: C.text,
                border: `1px solid ${C.border}`,
                borderRadius: 9,
                padding: '0.8rem 1.75rem',
                fontSize: 16,
                fontWeight: 600,
                textDecoration: 'none',
                display: 'inline-block',
              }}
            >
              View Demo
            </Link>
          </div>
        </div>
      </section>

      {/* Features */}
      <section style={{ padding: '4rem 2rem', maxWidth: 1100, margin: '0 auto' }}>
        <h2 style={{
          textAlign: 'center',
          fontSize: 32,
          fontWeight: 800,
          letterSpacing: '-1px',
          marginBottom: '0.5rem',
        }}>
          Everything you need
        </h2>
        <p style={{ textAlign: 'center', color: C.muted, marginBottom: '3rem', fontSize: 16 }}>
          Powerful analytics without the complexity or the bill.
        </p>

        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))',
          gap: '1.25rem',
        }}>
          {features.map((f) => (
            <div
              key={f.title}
              style={{
                background: C.surface,
                border: `1px solid ${C.border}`,
                borderRadius: 12,
                padding: '1.5rem',
                transition: 'border-color 0.2s',
              }}
              onMouseEnter={(e) => ((e.currentTarget as HTMLDivElement).style.borderColor = 'rgba(245,158,11,0.4)')}
              onMouseLeave={(e) => ((e.currentTarget as HTMLDivElement).style.borderColor = C.border)}
            >
              <div style={{ marginBottom: 12 }}>{f.icon}</div>
              <div style={{ fontWeight: 700, fontSize: 16, marginBottom: 6 }}>{f.title}</div>
              <div style={{ color: C.muted, fontSize: 14, lineHeight: 1.6 }}>{f.desc}</div>
            </div>
          ))}
        </div>
      </section>

      {/* Install */}
      <section ref={installRef} style={{ padding: '4rem 2rem', maxWidth: 780, margin: '0 auto' }}>
        <h2 style={{
          textAlign: 'center',
          fontSize: 32,
          fontWeight: 800,
          letterSpacing: '-1px',
          marginBottom: '0.5rem',
        }}>
          Up in minutes
        </h2>
        <p style={{ textAlign: 'center', color: C.muted, marginBottom: '2rem', fontSize: 16 }}>
          Pick your install method and you're done.
        </p>

        <InstallTabs />
      </section>

      {/* Footer */}
      <footer style={{
        borderTop: `1px solid ${C.border}`,
        padding: '2rem',
        textAlign: 'center',
        color: C.muted,
        fontSize: 14,
      }}>
        <div style={{ display: 'flex', gap: 24, justifyContent: 'center', marginBottom: 8 }}>
          <a href="https://github.com/funnelbarn/funnelbarn" style={{ color: C.muted, textDecoration: 'none' }}>GitHub</a>
          <a href="#" style={{ color: C.muted, textDecoration: 'none' }}>Docs</a>
          <Link to="/login" style={{ color: C.muted, textDecoration: 'none' }}>Sign in</Link>
        </div>
        <div>© {new Date().getFullYear()} FunnelBarn — Open Source</div>
      </footer>
    </div>
  )
}
