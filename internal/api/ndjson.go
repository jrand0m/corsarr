package api

import (
	"encoding/json"
	"net/http"
)

// NDJSON streams progressive per-request results: one JSON record
// per line, flushed as produced, then one summary line. It never touches the
// event bus.
type NDJSON struct {
	w      http.ResponseWriter
	enc    *json.Encoder
	flush  func()
	count  int
	errors []string
}

// NewNDJSON starts an application/x-ndjson response.
func NewNDJSON(w http.ResponseWriter) *NDJSON {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flush := func() {}
	if f, ok := w.(http.Flusher); ok {
		flush = f.Flush
	}
	return &NDJSON{w: w, enc: json.NewEncoder(w), flush: flush}
}

// Record writes one result line; an error means the client went away.
func (s *NDJSON) Record(v any) error {
	if err := s.enc.Encode(map[string]any{"record": v}); err != nil {
		return err
	}
	s.count++
	s.flush()
	return nil
}

// Error notes a partial failure (e.g. one indexer timed out); it is written
// immediately and counted in the summary.
func (s *NDJSON) Error(source string, err error) error {
	s.errors = append(s.errors, source+": "+err.Error())
	if e := s.enc.Encode(map[string]any{"error": map[string]string{"source": source, "message": err.Error()}}); e != nil {
		return e
	}
	s.flush()
	return nil
}

// Done writes the final summary line: counts, errors, done=true, plus extra.
func (s *NDJSON) Done(extra map[string]any) error {
	sum := map[string]any{"done": true, "count": s.count, "errors": len(s.errors)}
	for k, v := range extra {
		sum[k] = v
	}
	err := s.enc.Encode(map[string]any{"summary": sum})
	s.flush()
	return err
}
