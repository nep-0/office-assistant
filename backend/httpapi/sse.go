package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type SSEEmitter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func NewSSEEmitter(w http.ResponseWriter) SSEEmitter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	return SSEEmitter{w: w, flusher: flusher}
}

func (e SSEEmitter) Emit(event string, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, data)
	if e.flusher != nil {
		e.flusher.Flush()
	}
}
