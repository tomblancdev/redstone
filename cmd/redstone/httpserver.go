package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// The HTTP+JSON transport — humans, curl, healthchecks, browsers. Same JSON
// field names are the proto field names (snake_case).
//
// The kernel's own services are CORE capabilities, each with a proper
// single-capability base under /svc/<name> — so the kernel appears in its
// own catalog and is cross-checked and conformance-tested like any instance:
//
//	/svc/registry     bind, instances          (+ /manifest)
//	/svc/graph        edges                    (+ /manifest)
//	/svc/conformance  verdicts                 (+ /manifest)
//
// Root routes (/bind /instances /graph /health) are kept as aliases.
func serveHTTP(k *Kernel, addr string) {
	mux := http.NewServeMux()

	send := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(status)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(body)
	}

	health := func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]bool{"ok": true})
	}
	instances := func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"instances": k.Catalog()})
	}
	graph := func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"edges": k.GraphEdges()})
	}
	verdicts := func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, k.VerdictsDocument())
	}
	bind := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		labels := map[string]string{}
		for key, values := range q {
			if strings.HasPrefix(key, "label.") && len(values) > 0 {
				labels[key[len("label."):]] = values[0]
			}
		}
		ok, refusal, bad := k.SelectInstance(BindParams{
			Capability: q.Get("capability"),
			Level:      q.Get("level"),
			Name:       q.Get("name"),
			Labels:     labels,
			Consumer:   q.Get("consumer"),
			As:         q.Get("as"),
			Stack:      q.Get("stack"),
		})
		switch {
		case bad != "":
			send(w, 400, map[string]string{"error": bad})
		case refusal != nil:
			send(w, 409, refusal)
		default:
			send(w, 200, ok)
		}
	}
	manifest := func(capability string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			send(w, 200, map[string]any{
				"name":           "redstone-" + capability,
				"version":        "0.2.0",
				"kind":           "core",
				"capability":     capability,
				"level":          "core",
				"implementation": "redstone",
				"flags":          map[string]any{},
			})
		}
	}

	// The `edge` operation: adapters pull a consumer's {with, wire, identity}
	// by the edge path they see in the X-Edge header (stack/app/task).
	edge := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		e := k.EdgeFor(q.Get("stack"), q.Get("app"), q.Get("task"))
		if e == nil {
			send(w, 404, map[string]string{"error": "no such edge (was it ever bound?)"})
			return
		}
		send(w, 200, e)
	}

	// Core capability bases.
	mux.HandleFunc("/svc/registry/manifest", manifest("registry"))
	mux.HandleFunc("/svc/registry/bind", bind)
	mux.HandleFunc("/svc/registry/instances", instances)
	mux.HandleFunc("/svc/registry/edge", edge)
	mux.HandleFunc("/svc/graph/manifest", manifest("graph"))
	mux.HandleFunc("/svc/graph/edges", graph)
	mux.HandleFunc("/svc/conformance/manifest", manifest("conformance"))
	mux.HandleFunc("/svc/conformance/verdicts", verdicts)

	// Root aliases (back-compat for early consumers).
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/instances", instances)
	mux.HandleFunc("/graph", graph)
	mux.HandleFunc("/bind", bind)

	log.Printf("⚡ redstone http on %s (state: %s, capabilities: %s)", addr, k.Dir, k.Caps)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
