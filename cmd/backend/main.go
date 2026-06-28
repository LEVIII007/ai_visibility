// Command backend is the ingest API + aggregation (TRD §3.3/§3.5).
//   POST /events     ingest a batch (dedup + store)
//   GET  /api/usage  aggregated usage for the dashboard
// Optionally serves the built dashboard via -static.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"oximy/internal/event"
	"oximy/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "oximy-events.json", "event store file")
	static := flag.String("static", "", "optional dir to serve as dashboard (e.g. dashboard/dist)")
	flag.Parse()

	st := store.NewFileStore(*data)
	mux := http.NewServeMux()

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var evs []event.Event
		if err := json.NewDecoder(r.Body).Decode(&evs); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		added := st.Ingest(evs)
		log.Printf("ingest: %d received, %d new", len(evs), added)
		writeJSON(w, map[string]int{"received": len(evs), "added": added})
	})

	mux.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // let the Vite dev server fetch
		writeJSON(w, st.Usage())
	})

	if *static != "" {
		mux.Handle("/", http.FileServer(http.Dir(*static)))
	}

	log.Printf("oximy-lite backend on %s (data=%s)", *addr, *data)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
