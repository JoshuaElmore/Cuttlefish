package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

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
