// Command svc-mock is a tiny, dependency-free stand-in for a platform service
// listed in the Service Catalog. It lets the discovery tutorial run
// out-of-the-box: point the endpoint the catalog returns at a local svc-mock and
// the agent's generated code (or ppg-verify) has something real to call.
//
//	svc-mock -addr :9110 -name notify-svc        # POST /v1/messages -> 202 queued
//	svc-mock -addr :9120 -name payments-gateway  # POST /v1/charges  -> 201 authorized
//
// Any other -name serves a generic echo endpoint. GET /healthz is always 200.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/owulveryck/poc-agentic-platform/internal/version"
)

func main() {
	addr := flag.String("addr", ":9110", "listen address")
	name := flag.String("name", "notify-svc", "which catalog service to impersonate")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("svc-mock " + version.String())
		return
	}

	log.Printf("svc-mock %q listening on %s", *name, *addr)
	log.Fatal(http.ListenAndServe(*addr, newHandler(*name)))
}

// newHandler builds the routes for the impersonated service. Exposed for tests.
func newHandler(name string) http.Handler {
	mux := http.NewServeMux()
	var seq atomic.Int64

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": name})
	})

	switch name {
	case "notify-svc":
		mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusAccepted, map[string]any{
				"id":     fmt.Sprintf("msg_%06d", seq.Add(1)),
				"status": "queued",
			})
		})
	case "payments-gateway":
		mux.HandleFunc("POST /v1/charges", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Provider string `json:"provider"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Provider == "" {
				body.Provider = "stripe"
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"id":       fmt.Sprintf("chg_%06d", seq.Add(1)),
				"status":   "authorized",
				"provider": body.Provider,
			})
		})
	default:
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"service": name, "received": true})
		})
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
