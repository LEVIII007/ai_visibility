// Package event defines the one canonical event shape shared by the agent and
// the backend (TRD §4). Everything maps to this; nothing else crosses the wire.
package event

import "fmt"

// Event is a single observation: "on this device, this person's process was
// talking to an AI tool during this window." v1 carries NO content, NO tokens,
// NO dollars — only usage intensity, exactly as scoped in the PRD.
type Event struct {
	DeviceID        string `json:"device_id"`
	PersonID        string `json:"person_id"`
	Process         string `json:"process"`
	ToolID          string `json:"tool_id"`
	ToolLabel       string `json:"tool_label"`
	Destination     string `json:"destination"`
	StartTS         int64  `json:"start_ts"` // unix seconds (window start)
	DurationSeconds int    `json:"duration_seconds"`
	ConnectionCount int    `json:"connection_count"`
	AgentVersion    string `json:"agent_version"`
	CatalogVersion  int    `json:"catalog_version"`
}

// Key is the deterministic dedup key (TRD: one observation must not double-count
// across agent retries). Same (device, tool, window) => same event.
func (e Event) Key() string {
	return fmt.Sprintf("%s|%s|%d", e.DeviceID, e.ToolID, e.StartTS)
}
