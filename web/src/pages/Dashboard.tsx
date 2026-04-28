import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'

interface DashboardData {
  total_events: number
  unique_sessions: number
  bounce_rate: number
  top_pages: { URL: string; Views: number }[]
  top_referrers: { Domain: string; Visits: number }[]
  top_event_names: { Name: string; Count: number }[]
  events_time_series: { Time: string; Count: number }[]
}

export default function Dashboard() {
  const { projectId } = useParams<{ projectId?: string }>()
  const [data, setData] = useState<DashboardData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!projectId) return
    setLoading(true)
    setError(null)
    fetch(`/api/v1/projects/${projectId}/dashboard`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then(setData)
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false))
  }, [projectId])

  return (
    <div style={{ fontFamily: 'sans-serif', padding: '2rem', maxWidth: 1200, margin: '0 auto' }}>
      <h1>Trailpost Analytics</h1>

      {!projectId && (
        <p style={{ color: '#6b7280' }}>
          Select a project to view its dashboard.{' '}
          <a href="/api/v1/projects">List projects</a>
        </p>
      )}

      {loading && <p>Loading…</p>}
      {error && <p style={{ color: 'red' }}>Error: {error}</p>}

      {data && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '1rem', marginBottom: '2rem' }}>
            <StatCard label="Total Events" value={data.total_events.toLocaleString()} />
            <StatCard label="Unique Sessions" value={data.unique_sessions.toLocaleString()} />
            <StatCard label="Bounce Rate" value={`${(data.bounce_rate * 100).toFixed(1)}%`} />
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2rem' }}>
            <section>
              <h2>Top Pages</h2>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left' }}>URL</th>
                    <th style={{ textAlign: 'right' }}>Views</th>
                  </tr>
                </thead>
                <tbody>
                  {data.top_pages?.map((p) => (
                    <tr key={p.URL}>
                      <td style={{ maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {p.URL}
                      </td>
                      <td style={{ textAlign: 'right' }}>{p.Views.toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>

            <section>
              <h2>Top Referrers</h2>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left' }}>Domain</th>
                    <th style={{ textAlign: 'right' }}>Visits</th>
                  </tr>
                </thead>
                <tbody>
                  {data.top_referrers?.map((r) => (
                    <tr key={r.Domain}>
                      <td>{r.Domain}</td>
                      <td style={{ textAlign: 'right' }}>{r.Visits.toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>

            <section>
              <h2>Top Events</h2>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left' }}>Event</th>
                    <th style={{ textAlign: 'right' }}>Count</th>
                  </tr>
                </thead>
                <tbody>
                  {data.top_event_names?.map((e) => (
                    <tr key={e.Name}>
                      <td>{e.Name}</td>
                      <td style={{ textAlign: 'right' }}>{e.Count.toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          </div>
        </>
      )}
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div
      style={{
        background: '#f9fafb',
        border: '1px solid #e5e7eb',
        borderRadius: 8,
        padding: '1.25rem',
      }}
    >
      <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 600 }}>{value}</div>
    </div>
  )
}
