// Package catalog is the project's real asset (PRD §9): a mapping of
// (process signature + destination patterns) -> tool identity.
//
// The spike proved the PROCESS axis is the strong, no-entitlement signal and the
// destination axis is mushy (providers front through Cloudflare/GCP/AWS). So the
// process rules below are load-bearing; the IP rules are a weak, illustrative
// secondary axis only.
//
// For the slice the catalog is embedded. Serving it from the backend (so tool
// defs update without shipping a new agent) is the next step.
package catalog

import (
	"net"
	"strings"
)

// Version lets the backend reason about which catalog produced an event.
const Version = 1

type Tool struct {
	ID    string
	Label string
}

type processRule struct {
	needle string // lowercased substring of the process command
	tool   Tool
}

// Hand-coded starter set. Start small (~10), not 6,500 (PRD §9).
var processRules = []processRule{
	{"cursor", Tool{"cursor", "Cursor"}},
	{"claude", Tool{"claude", "Claude"}},
	{"chatgpt", Tool{"chatgpt", "ChatGPT"}},
	{"codex", Tool{"codex", "OpenAI Codex"}},
	{"copilot", Tool{"github-copilot", "GitHub Copilot"}},
	{"codeium", Tool{"codeium", "Codeium/Windsurf"}},
	{"windsurf", Tool{"codeium", "Codeium/Windsurf"}},
	{"ollama", Tool{"ollama", "Ollama (local)"}},
	{"perplexity", Tool{"perplexity", "Perplexity"}},
	{"warp", Tool{"warp", "Warp"}},
}

// MatchProcess attributes a process command to a tool by case-insensitive
// substring — the strong on-device signal the spike validated.
func MatchProcess(command string) (Tool, bool) {
	low := strings.ToLower(command)
	for _, r := range processRules {
		if strings.Contains(low, r.needle) {
			return r.tool, true
		}
	}
	return Tool{}, false
}

type ipRule struct {
	net  *net.IPNet
	tool Tool
}

// ILLUSTRATIVE ONLY. Anthropic announces 160.79.104.0/23; most providers front
// through cloud/CDN ranges that rotate, which is why this axis is weak. A real
// catalog would track each provider's published CIDRs.
var ipRules = func() []ipRule {
	raw := []struct {
		cidr string
		tool Tool
	}{
		{"160.79.104.0/23", Tool{"anthropic-ip", "Anthropic (by IP)"}},
	}
	out := make([]ipRule, 0, len(raw))
	for _, r := range raw {
		if _, n, err := net.ParseCIDR(r.cidr); err == nil {
			out = append(out, ipRule{n, r.tool})
		}
	}
	return out
}()

// MatchIP is the weak secondary axis: only used when the process axis misses.
func MatchIP(host string) (Tool, bool) {
	ip := net.ParseIP(host)
	if ip == nil {
		return Tool{}, false
	}
	for _, r := range ipRules {
		if r.net.Contains(ip) {
			return r.tool, true
		}
	}
	return Tool{}, false
}
