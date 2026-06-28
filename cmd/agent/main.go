// Command agent is the macOS endpoint agent (TRD §3.1): every interval it
// enumerates the user's connections, attributes them to AI tools via the
// catalog, and posts one observation per active tool to the backend.
//
// No entitlement, no sudo, no content — the tier the spike validated.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"oximy/internal/catalog"
	"oximy/internal/collector"
	"oximy/internal/event"
)

const agentVersion = "0.1.0"

func main() {
	backend := flag.String("backend", "http://localhost:8080", "backend base URL")
	interval := flag.Duration("interval", 10*time.Second, "scan interval")
	once := flag.Bool("once", false, "run a single scan, send, and exit")
	print := flag.Bool("print", false, "also print the events as JSON")
	flag.Parse()

	device, _ := os.Hostname()
	person := os.Getenv("USER")
	if person == "" {
		person = "unknown"
	}
	intervalSec := int(interval.Seconds())
	if intervalSec <= 0 {
		intervalSec = 1
	}

	log.Printf("oximy-lite agent %s — device=%s person=%s interval=%s backend=%s",
		agentVersion, device, person, *interval, *backend)

	scan := func() {
		events := collect(device, person, intervalSec)
		if len(events) == 0 {
			log.Printf("scan: no AI tools active")
		}
		for _, e := range events {
			log.Printf("scan: %-18s conns=%d -> %s", e.ToolLabel, e.ConnectionCount, e.Destination)
		}
		if *print && len(events) > 0 {
			b, _ := json.MarshalIndent(events, "", "  ")
			fmt.Println(string(b))
		}
		if len(events) > 0 {
			if err := send(*backend, events); err != nil {
				log.Printf("send failed (will retry next scan): %v", err)
			}
		}
	}

	scan()
	if *once {
		return
	}
	t := time.NewTicker(*interval)
	defer t.Stop()
	for range t.C {
		scan()
	}
}

// collect enumerates connections and rolls them up into one event per active tool.
func collect(device, person string, intervalSec int) []event.Event {
	conns, err := collector.Collect()
	if err != nil {
		log.Printf("collect error: %v", err)
		return nil
	}
	now := time.Now().Unix()

	type agg struct {
		tool  catalog.Tool
		proc  string
		dest  string
		conns int
	}
	byTool := map[string]*agg{}
	for _, c := range conns {
		tool, ok := catalog.MatchProcess(c.Command)
		if !ok {
			// weak secondary axis: only when the process axis misses
			if t2, ok2 := catalog.MatchIP(c.RemoteHost); ok2 {
				tool, ok = t2, true
			}
		}
		if !ok {
			continue
		}
		a := byTool[tool.ID]
		if a == nil {
			a = &agg{tool: tool, proc: c.Command, dest: c.RemoteHost}
			byTool[tool.ID] = a
		}
		a.conns++
	}

	events := make([]event.Event, 0, len(byTool))
	for _, a := range byTool {
		events = append(events, event.Event{
			DeviceID:        device,
			PersonID:        person,
			Process:         a.proc,
			ToolID:          a.tool.ID,
			ToolLabel:       a.tool.Label,
			Destination:     a.dest,
			StartTS:         now,
			DurationSeconds: intervalSec,
			ConnectionCount: a.conns,
			AgentVersion:    agentVersion,
			CatalogVersion:  catalog.Version,
		})
	}
	return events
}

func send(backend string, events []event.Event) error {
	b, err := json.Marshal(events)
	if err != nil {
		return err
	}
	resp, err := http.Post(backend+"/events", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("backend returned %s", resp.Status)
	}
	return nil
}
