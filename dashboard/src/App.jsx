import { useEffect, useState } from 'react'
import './App.css'

const API = 'http://localhost:8080/api/usage'
const POLL_MS = 3000

function fmtDur(s) {
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ${s % 60}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}
function ago(unix) {
  if (!unix) return '—'
  const d = Math.max(0, Math.floor(Date.now() / 1000) - unix)
  if (d < 60) return `${d}s ago`
  if (d < 3600) return `${Math.floor(d / 60)}m ago`
  return `${Math.floor(d / 3600)}h ago`
}
function clock(unix) {
  return unix ? new Date(unix * 1000).toLocaleTimeString() : '—'
}
function peopleCount(tools) {
  return tools.reduce((m, t) => Math.max(m, t.people), 0)
}

export default function App() {
  const [usage, setUsage] = useState(null)
  const [err, setErr] = useState(null)
  const [updated, setUpdated] = useState(null)

  useEffect(() => {
    let alive = true
    const tick = async () => {
      try {
        const r = await fetch(API)
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        const j = await r.json()
        if (!alive) return
        setUsage(j)
        setErr(null)
        setUpdated(new Date())
      } catch (e) {
        if (alive) setErr(e.message)
      }
    }
    tick()
    const id = setInterval(tick, POLL_MS)
    return () => { alive = false; clearInterval(id) }
  }, [])

  const tools = usage?.tools ?? []
  const recent = usage?.recent ?? []

  return (
    <div className="wrap">
      <header className="top">
        <div>
          <h1>Oximy <span className="lite">Lite</span></h1>
          <p className="sub">AI tool usage across your fleet — discovery &amp; intensity</p>
        </div>
        <div className="status">
          <span className={err ? 'dot bad' : 'dot live'} />
          {err ? <span>backend unreachable</span>
               : <span>live · {updated ? updated.toLocaleTimeString() : '…'}</span>}
        </div>
      </header>

      {err && <div className="banner">Can’t reach {API} — is the backend running? ({err})</div>}

      <section className="cards">
        <Card label="AI tools detected" value={tools.length} />
        <Card label="Observations" value={usage?.events ?? 0} />
        <Card label="People" value={peopleCount(tools)} />
      </section>

      <section className="panel">
        <h2>Tool leaderboard</h2>
        {tools.length === 0 ? (
          <Empty msg="No AI tools observed yet. Open Claude / Cursor / ChatGPT and send a prompt." />
        ) : (
          <table>
            <thead>
              <tr>
                <th>#</th><th>Tool</th><th>Active time</th><th>Observations</th>
                <th>Connections</th><th>People</th><th>Last seen</th>
              </tr>
            </thead>
            <tbody>
              {tools.map((t, i) => (
                <tr key={t.tool_id}>
                  <td className="rank">{i + 1}</td>
                  <td className="tool">{t.tool_label}</td>
                  <td>{fmtDur(t.active_seconds)}</td>
                  <td>{t.observations}</td>
                  <td>{t.total_connections}</td>
                  <td>{t.people}</td>
                  <td className="muted">{ago(t.last_seen)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="panel">
        <h2>Recent observations</h2>
        {recent.length === 0 ? (
          <Empty msg="Waiting for the agent’s next scan…" />
        ) : (
          <table>
            <thead>
              <tr>
                <th>Time</th><th>Person</th><th>Tool</th><th>Process</th>
                <th>Destination</th><th>Conns</th>
              </tr>
            </thead>
            <tbody>
              {recent.map((e, i) => (
                <tr key={i}>
                  <td className="muted">{clock(e.start_ts)}</td>
                  <td>{e.person_id}</td>
                  <td className="tool">{e.tool_label}</td>
                  <td className="mono">{e.process}</td>
                  <td className="mono muted ellipsis">{e.destination}</td>
                  <td>{e.connection_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <footer className="foot">
        v1 captures <strong>usage intensity only</strong> — which tool, how often, how heavily.
        No prompt content, no tokens, no dollars (by design — see PRD). Attribution is by
        on-device process + socket enumeration; no entitlement, no sudo.
      </footer>
    </div>
  )
}

function Card({ label, value }) {
  return (
    <div className="card">
      <div className="cardval">{value}</div>
      <div className="cardlabel">{label}</div>
    </div>
  )
}

function Empty({ msg }) {
  return <div className="empty">{msg}</div>
}
