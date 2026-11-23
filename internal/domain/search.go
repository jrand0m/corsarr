package domain

import "time"

// SearchState is an item's search record: when it was last searched, what
// that search found, and when it may be searched automatically again.
type SearchState struct {
	Item         ItemRef          `json:"item"`
	LastSearched *time.Time       `json:"lastSearched,omitempty"`
	NextEligible *time.Time       `json:"nextEligible,omitempty"`
	Misses       int              `json:"misses"` // searches in a row that grabbed nothing
	LastResult   *SearchEventData `json:"lastResult,omitempty"`
}
