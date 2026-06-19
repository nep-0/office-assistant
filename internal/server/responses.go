package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeSSE(w http.ResponseWriter, event string, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		payload = []byte(`{"message":"failed to encode event"}`)
		event = "error"
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
}
