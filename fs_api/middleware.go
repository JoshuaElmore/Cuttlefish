package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// debugMode enables per-request logging. It is empty in a normal build and set
// at link time by `make build-api-debug`:
//
//	go build -ldflags "-X main.debugMode=1" -o fs_api .
//
// This is deliberately a linker variable rather than a `//go:build debug` file
// pair. Mutually exclusive tagged files always leave one of the two outside the
// active build configuration, which gopls reports as "No packages found" on
// whichever file the editor isn't currently type-checking.
var debugMode string

func debugLog(format string, args ...any) {
	if debugMode == "" {
		return
	}
	log.Printf(format, args...)
}

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		debugLog("--> %s %s", r.Method, r.URL.String())
		next(w, r)
		debugLog("<-- %s %s took %v", r.Method, r.URL.String(), time.Since(start))
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encoding error: %v", err)
	}
}

func respondError(w http.ResponseWriter, code int, message string) {
	respondJSON(w, code, map[string]string{"error": message})
}
