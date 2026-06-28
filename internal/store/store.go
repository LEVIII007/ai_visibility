// Package store is the persistence boundary (TRD §3.3). The slice ships a
// file-backed in-memory implementation so the pipe runs with zero external
// services. Swapping to Postgres = implement this same Store interface; nothing
// upstream changes.
package store

import (
	"encoding/json"
	"os"
	"sort"
	"sync"

	"oximy/internal/event"
)

type Store interface {
	Ingest(events []event.Event) (added int)
	Usage() Usage
}

type ToolUsage struct {
	ToolID        string `json:"tool_id"`
	ToolLabel     string `json:"tool_label"`
	Observations  int    `json:"observations"`
	ActiveSeconds int    `json:"active_seconds"`
	TotalConns    int    `json:"total_connections"`
	LastSeen      int64  `json:"last_seen"`
	People        int    `json:"people"`
}

type Usage struct {
	Tools  []ToolUsage   `json:"tools"`
	Recent []event.Event `json:"recent"`
	Events int           `json:"events"`
}

type fileStore struct {
	mu     sync.Mutex
	seen   map[string]bool // dedup keys
	events []event.Event
	path   string
}

// NewFileStore loads any existing events from path and returns a Store.
func NewFileStore(path string) Store {
	s := &fileStore{seen: map[string]bool{}, path: path}
	s.load()
	return s
}

func (s *fileStore) load() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var evs []event.Event
	if json.Unmarshal(b, &evs) == nil {
		s.events = evs
		for _, e := range evs {
			s.seen[e.Key()] = true
		}
	}
}

func (s *fileStore) persist() {
	if b, err := json.MarshalIndent(s.events, "", "  "); err == nil {
		_ = os.WriteFile(s.path, b, 0o644)
	}
}

func (s *fileStore) Ingest(events []event.Event) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	for _, e := range events {
		k := e.Key()
		if s.seen[k] {
			continue // deterministic dedup (TRD §3.3)
		}
		s.seen[k] = true
		s.events = append(s.events, e)
		added++
	}
	if added > 0 {
		s.persist()
	}
	return added
}

func (s *fileStore) Usage() Usage {
	s.mu.Lock()
	defer s.mu.Unlock()

	type acc struct {
		u      ToolUsage
		people map[string]bool
	}
	m := map[string]*acc{}
	for _, e := range s.events {
		a := m[e.ToolID]
		if a == nil {
			a = &acc{u: ToolUsage{ToolID: e.ToolID, ToolLabel: e.ToolLabel}, people: map[string]bool{}}
			m[e.ToolID] = a
		}
		a.u.Observations++
		a.u.ActiveSeconds += e.DurationSeconds
		a.u.TotalConns += e.ConnectionCount
		if e.StartTS > a.u.LastSeen {
			a.u.LastSeen = e.StartTS
		}
		a.people[e.PersonID] = true
	}
	tools := make([]ToolUsage, 0, len(m))
	for _, a := range m {
		a.u.People = len(a.people)
		tools = append(tools, a.u)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].ActiveSeconds > tools[j].ActiveSeconds })

	// recent: newest-first, capped
	n := len(s.events)
	start := n - 20
	if start < 0 {
		start = 0
	}
	recent := make([]event.Event, 0, n-start)
	for i := n - 1; i >= start; i-- {
		recent = append(recent, s.events[i])
	}
	return Usage{Tools: tools, Recent: recent, Events: n}
}
