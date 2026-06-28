# TRD — Lite AI-Usage Visibility (Mac v1)

> Status: Draft v1 · Companion to [PRD.md](PRD.md) · Scope: design-partner v1, Mac only

---

## 1. Scope

Build a macOS endpoint agent + ingest backend + admin dashboard that captures **AI-tool usage
intensity** using process attribution + destination signals only. No TLS interception, no content.

This TRD covers the component architecture, the data model, the classification logic, the team
split, and the build order. It deliberately parks the entire content/TLS/CASB tier in a labeled
**Phase 2** box that v1 does not touch.

## 2. Architecture overview

```
┌─────────────────────────────── macOS endpoint agent ───────────────────────────────┐
│                                                                                      │
│   process monitor ──► socket→PID mapper ──► catalog matcher ──► event buffer ──┐     │
│   (which apps,        (which process        (process+dest →      (batch +       │     │
│    when)              owns each conn,        tool identity)      retry)         │     │
│                       to which host)                                            │     │
└─────────────────────────────────────────────────────────────────────────────┼──────┘
                                                                                │
                                                          batched events (HTTPS)│
                                                                                ▼
                                            ┌──────────────── backend ──────────────────┐
                                            │  ingest API ──► dedup ──► canonical event  │
                                            │                          store (Postgres)  │
                                            │  catalog service ◄── served to agent       │
                                            │  aggregation queries                       │
                                            └──────────────────────┬─────────────────────┘
                                                                   ▼
                                                          ┌──── dashboard ────┐
                                                          │ usage views,      │
                                                          │ leaderboard,      │
                                                          │ shadow discovery, │
                                                          │ enrollment + auth │
                                                          └───────────────────┘

   [ Phase 2 — NOT in v1: TLS interception · content · tokens · $ · DLP · browser ext · MDM/CASB ]
```

## 3. Components

### 3.1 Endpoint agent (Mac) — the only hard piece

Three small jobs plus a buffer:

1. **Process monitor** — which apps are running, when they start/stop.
2. **Socket→PID mapper** — for each open network connection, resolve the owning PID and the
   remote host it's talking to.
3. **Catalog matcher** — turn "process `Cursor` talking to an Anthropic IP range" into a tool
   identity (`tool_id`, vendor, category) using the catalog.
4. **Buffer** — batch events, retry on failure (agent must tolerate offline / backend down).

**v1 capture mechanism (scrappy, no entitlement):** plain process + socket enumeration — the same
data `lsof -i` / `nettop` read, via `proc_pidinfo` / `libproc`. This needs **no Apple
entitlement**, which is why it's the v1 path.

**Phase 2 upgrades (each needs an Apple-granted entitlement — do not apply before the scrappy
version is proven):**
- `NEDNSProxyProvider` — real hostnames (SNI/DNS), cheap destination signal.
- `NETransparentProxyProvider` / NetworkExtension — true per-flow capture.
- `EndpointSecurity` framework — precise process lifecycle (exec/fork).

> Runs as a background process. v1 does not need to survive MDM-grade hardening; it needs to run
> reliably on a developer/design-partner Mac.

### 3.2 Catalog service

- Versioned mapping `(process signature + destination patterns) → tool identity`.
- Served from backend; agent fetches/refreshes so tool defs update without a new binary.
- Start ~40 tools. Curating it is an ongoing shared chore (one owner, everyone contributes).

### 3.3 Ingest pipeline (deliberately boring)

- **Ingest API** — receives batched events from agents (authenticated by device token).
- **Dedup step** — deterministic; one session must not double-count.
- **Canonical event store** — **Postgres** (not ClickHouse — at design-partner scale the
  billion-row problem does not exist; reaching for it is premature scaling).
- **Aggregation queries** — per-person, per-team, per-tool rollups for the dashboard.

### 3.4 Enrollment + auth (lite)

- Admin invites people; each install carries a token mapping **device → person**.
- Manual team tagging is fine for v1. No SSO, no SCIM, no org-chart inference.

### 3.5 Dashboard (the product the customer sees)

- Per-person and per-team tool usage.
- Tool leaderboard.
- Trends over time.
- Shadow-discovery view: "tools in use you may not have approved."

## 4. Canonical event shape

One event shape, everything maps to it:

```json
{
  "device_id":        "uuid",
  "person_id":        "uuid",
  "process":          "Cursor",
  "tool_id":          "cursor",
  "destination":      "api.anthropic.com / <ip-range>",
  "start_ts":         "ISO-8601",
  "duration":         "seconds",
  "connection_count": 412,
  "agent_version":    "semver",
  "catalog_version":  "int"
}
```

`agent_version` and `catalog_version` are first-class so we can reason about drift and reprocess.

## 5. Classification logic — "which request is AI work"

Two **orthogonal** axes; need both (one host serves many tools; one browser hits many hosts):

- **Who sent it (process attribution):** resolve flow → owning PID → binary path / bundle ID /
  code signature / argv → "Cursor" vs "node running claude-code" vs "some python script."
  *This is the on-device superpower a cloud proxy physically cannot do.*
- **Where it went (destination):** remote host / IP range matched against the AI-provider catalog.

**v1 stays deterministic and local — no LLM in this path.** Pipeline: known AI process? known AI
destination? → emit event. An LLM only appears in Phase 2, on *content* of confirmed AI requests,
to classify what the work was about.

> Note: distinguishing a *generation* from auth/telemetry (URL path, body schema, SSE
> `text/event-stream` token deltas) requires reading the body → that's Phase 2. v1 attributes at
> the **tool/session** level, not the per-request-intent level.

## 6. Degraded modes (expected, designed-for)

- Cert-pinned apps, QUIC/HTTP3, opaque/protobuf bodies, shared upstreams (internal AI gateway):
  v1 already can't read bodies, so these mostly don't regress v1 — but they collapse destination
  signal in some cases. **Principle: always emit the event (even contentless / lower-confidence)
  rather than break the machine.** Keep the event, lose the content.

## 7. Team split (3 engineers)

| Dev | Owns |
|-----|------|
| Dev 1 (strongest systems person) | The Mac agent end-to-end — the make-or-break piece |
| Dev 2 | Backend: ingest, dedup, canonical event, aggregation queries, catalog service |
| Dev 3 | Dashboard, enrollment, auth — the demoable surface |
| Shared | Catalog content (tool definitions) — one curator, everyone contributes |

## 8. Build order (schedule protector)

1. **Day-one spike** — validate assumption #1 (clean process→socket→tool attribution without
   entitlement). Gate: green → proceed; mushy → rethink capture before committing the team.
2. **Vertical slice** — one tool (Cursor → Anthropic), one Mac → one event → stored → dashboard
   shows "1 Cursor session." Forces the inter-dev interfaces to be real on day two.
3. **Breadth** — more tools via the catalog (turning a crank, not new architecture).
4. **Polish** — dashboard views, shadow discovery, demo readiness.

Do **not** build the three components in parallel and integrate at the end — interface drift turns
a 6-week project into a 12-week one.

## 9. Risks & open questions

| Risk | Mitigation |
|------|------------|
| Process→socket attribution is mushy without entitlement (R1, gates everything) | Day-one spike before committing team |
| Forced into NetworkExtension entitlement → +weeks + Apple approval cycle we don't control | Treat as a hard fork; only after scrappy version fails |
| "No-dollars" product isn't compelling enough to install | One design-partner conversation in parallel with the spike |
| Catalog rot as vendors change IPs/processes | Backend-served, versioned catalog; patch without redeploy |
| Scope creep back toward Phase 2 (content/TLS/DLP) | Non-goals stated explicitly; >4-month estimate = re-cut |

## 10. Phase 2 (explicitly out of v1 — listed so it stays out)

TLS interception (root CA, per-flow termination, pinning detect-and-passthrough), content
capture, token/dollar metrics, generation-vs-telemetry intent classification, LLM-based content
classification, DLP/leak detection, browser extension, MDM (Jamf/Intune) + CASB integrations,
Windows, SSO/SCIM. Each is a deliberate later tier, not a v1 stretch goal.
