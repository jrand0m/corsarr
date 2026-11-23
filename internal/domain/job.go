package domain

import (
	"encoding/json"
	"time"
)

// JobResult is the outcome of a job's last run.
type JobResult string

const (
	JobNeverRun JobResult = ""
	JobOK       JobResult = "ok"
	JobFailed   JobResult = "failed"
)

// JobState is a scheduled job's observable state: persisted by the store, served by /api/v1/jobs, carried by
// `job.updated` events.
type JobState struct {
	Name       string        `json:"name"`
	Enabled    bool          `json:"enabled"`
	Interval   time.Duration `json:"-"` // JSON: intervalSeconds
	Heavy      bool          `json:"heavy"`
	Running    bool          `json:"running"`
	LastStart  *time.Time    `json:"lastStart,omitempty"`
	LastFinish *time.Time    `json:"lastFinish,omitempty"`
	LastResult JobResult     `json:"lastResult"`
	LastError  string        `json:"lastError,omitempty"`
	Failures   int           `json:"failures"`
	NextRun    *time.Time    `json:"nextRun,omitempty"`
}

// MarshalJSON writes the interval as whole seconds (intervalSeconds).
func (j JobState) MarshalJSON() ([]byte, error) {
	type plain JobState
	return json.Marshal(struct {
		plain
		IntervalSeconds int64 `json:"intervalSeconds"`
	}{plain(j), int64(j.Interval / time.Second)})
}
