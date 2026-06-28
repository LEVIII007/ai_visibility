# PRD — Lite AI-Usage Visibility (working name: "lite Oximy")

> Status: Draft v1 · Owner: Shashank · Team: 3 engineers · Scope: design-partner v1

---

## 1. One-liner

An endpoint agent for company-owned Macs that tells a small company **which AI tools its
people use and how much** — using process + destination signals only. No content capture,
no tokens, no dollars, no MDM, no CASB.

## 2. Problem

Companies are handing out laptops to employees who use a sprawling, unknown set of AI tools
(ChatGPT desktop, Claude, Cursor, Copilot, Codex, and dozens more). Leadership has no idea:

- **Which** AI tools are actually in use across the org (shadow-AI discovery).
- **How heavily** each tool/team/person uses AI.
- Whether paid seats are **active or idle** (license waste).

Today this is invisible. The first time anyone finds out a team adopted an unapproved tool is
usually after something goes wrong.

## 3. Why now / why this shape

The full vision (per-person dollar spend, prompt content, leak/DLP detection, ROI attribution)
requires TLS interception — installing a root CA and MITM-ing encrypted traffic. That is the
hardest, most fragile, most adversarial capture path (cert pinning hard-fails it, QUIC defeats
it, every vendor update can silently break the parser) and it forces an enterprise sales motion
(MDM, SOC 2, legal review).

**This v1 deliberately does NOT do that.** It delivers *usage intensity* from cheap, reliable
OS-level telemetry — the part that is reliable to build and still produces a real
"holy shit, we have 14 AI tools and didn't know" moment.

## 4. Goals (v1)

| # | Goal | Success signal |
|---|------|----------------|
| G1 | Discover which AI tools are in use on a Mac | Agent reliably attributes connections to known AI tools |
| G2 | Quantify usage intensity per person and per team | Dashboard shows sessions, frequency, duration, connection counts |
| G3 | Surface shadow AI | "Tools in use you may not have approved" view |
| G4 | Be demoable to a design partner | End-to-end: install → data flows → dashboard within minutes |

## 5. Explicit non-goals (v1)

State these out loud to the team — drifting back into any of these is the main project risk.

- ❌ **No TLS interception / no content / no prompts / no tokens / no dollars** (deferred — Phase 2).
- ❌ **No Windows** — Mac only.
- ❌ **No browser extension** — accept the in-browser blind spot for now.
- ❌ **No MDM / enterprise CASB integration** — no Jamf/Intune/Zscaler/Netskope.
- ❌ **No SSO / SCIM** — lite invite-based enrollment only.
- ❌ **No Sidekick / coaching / ROI-Loop / 800M-profile dataset / cross-company benchmarking.**

> Why the dollar metric is out: tokens and cost live inside encrypted request bodies, and on
> flat-rate subscription tools (Cursor seat, Claude seat) per-person dollar cost is largely
> fiction anyway. Do **not** design the dashboard around a number we cannot produce yet.

## 6. Target user & buyer

- **Buyer / beneficiary:** the company (admin / eng lead / IT). The tool is installed on
  company-owned laptops at hand-out time. This is top-down, not bottoms-up — accept that framing.
- **Subject:** the employee whose AI-tool usage is captured.

## 7. Core user stories

- As an **admin**, I can see a leaderboard of AI tools in use across the company.
- As an **admin**, I can see per-person and per-team usage intensity (sessions, frequency,
  duration, connection count) over time.
- As an **admin**, I can spot AI tools in use that we never approved (shadow discovery).
- As an **admin**, I can invite employees and map a device to a person.

## 8. What v1 measures (and what it does not)

| Captured | Not captured |
|----------|--------------|
| Which app/process is running and when | Prompt / response content |
| Which AI host/provider it talked to | Token counts |
| Session start/stop, duration | Dollar cost |
| Connection counts per session | Anything inside the encrypted body |
| Tool identity (via catalog) | Browser-tab-level tool identity |

## 9. The asset: the tool catalog

The real product asset is **not** the interception code — it's a versioned mapping of
`(process signature + destination patterns) → tool identity`
(e.g. "process `Cursor` + Anthropic IP range → tool: Cursor / vendor: Anthropic / category: coding").

- Start with **~40 tools**, not 6,500.
- Serve it from the backend so tool definitions update without shipping a new agent.
- This is also the answer to drift: when a vendor changes IPs, patch the catalog, not the binary.

## 10. Make-or-break assumptions (verify before committing the team)

1. **(Technical, gates everything)** Plain process + socket enumeration on macOS
   (`proc_pidinfo`/`libproc`, the data `lsof -i`/`nettop` read) gives clean enough attribution
   to say which AI tool a connection belongs to — **without** an Apple NetworkExtension
   entitlement.
   - *Verify:* a ~50-line spike that lists processes, resolves each one's open connections to
     remote endpoints, and matches against a hand-coded list of ~5 AI providers. If it reliably
     prints "Cursor → Anthropic, ChatGPT desktop → OpenAI" → validated. If attribution is mushy
     (shared networking daemons, unattributable CDN IPs) → we learn it today, for free.
2. **(Demand)** "Which AI tools, how much" — *without* dollars/content — is valuable enough that
   a small company would install it.
   - *Verify:* one design-partner conversation.

Run both in parallel this week; they de-risk the two different ways this dies.

## 11. Rough timeline

- **One-day spike** to validate assumption #1.
- If validated: **~6 weeks full-time** (you + 2) to a design-partner-demoable v1, or **~3 months**
  part-time around day jobs.
- Long pole is the **macOS agent** (2–3 weeks) — bottlenecked by real-world systems debugging,
  the least AI-accelerable part. Backend ~1 week, dashboard ~1–1.5 weeks, integration/polish ~1–2 weeks.

## 12. Build principle

Build the **thinnest vertical slice first**: agent detects one tool (Cursor → Anthropic) on one
Mac → emits one event → backend stores it → dashboard shows "1 Cursor session." Get that single
thread end-to-end before adding a second tool. Do **not** build the three components in parallel
and integrate at the end.
