package automation

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Backlog search: an hourly job searches wanted items that are due, least
// recently searched first, within a global budget per run.
const (
	BacklogJob      = "automation.backlog"
	BacklogInterval = time.Hour
	BacklogBudget   = 30 // item searches per hour
	// RetryBase is the wait after a search that grabbed nothing; it doubles
	// with every further miss up to RetryMax.
	RetryBase = 6 * time.Hour
	RetryMax  = 7 * 24 * time.Hour
	// QueryLogKeep is how long the per-indexer query log is kept (the longest limit window).
	QueryLogKeep = 24 * time.Hour
)

// NextEligible is when an item searched at `at` may be searched again
// automatically: RetryBase after a grab, else doubling per miss up to RetryMax.
func NextEligible(at time.Time, misses int) time.Time {
	if misses <= 1 {
		return at.Add(RetryBase)
	}
	return at.Add(min(RetryBase<<min(misses-1, 10), RetryMax))
}

// Due picks up to budget wanted items that are eligible at now, never
// searched first, then least recently searched; ties by id (pure).
func Due(wanted []int64, states map[int64]domain.SearchState, now time.Time, budget int) []int64 {
	var due []int64
	for _, id := range wanted {
		st := states[id]
		if st.NextEligible == nil || !st.NextEligible.After(now) {
			due = append(due, id)
		}
	}
	last := func(id int64) int64 {
		if st := states[id]; st.LastSearched != nil {
			return st.LastSearched.UnixMilli()
		}
		return -1
	}
	sort.Slice(due, func(i, j int) bool {
		a, b := last(due[i]), last(due[j])
		return a < b || (a == b && due[i] < due[j])
	})
	return due[:min(len(due), budget)]
}

// Limit is an indexer's query limit: at most Queries per Per (0 = none).
type Limit struct {
	Queries int
	Per     time.Duration
}

// ParseLimit reads an indexer's queryLimit/queryLimitPer settings.
func ParseLimit(queries int, per string) Limit {
	if per == "hour" {
		return Limit{Queries: queries, Per: time.Hour}
	}
	return Limit{Queries: queries, Per: 24 * time.Hour}
}

// LimitError says an indexer was skipped because its query limit is used up.
type LimitError struct {
	Indexer domain.ObjectKey
	Resets  time.Time
}

func (e LimitError) Error() string {
	return fmt.Sprintf("query limit reached, resets at %s", e.Resets.UTC().Format(time.RFC3339))
}

// Searches persists search state and the query log (store.Searches).
type Searches interface {
	SaveState(ctx context.Context, st domain.SearchState) error
	State(ctx context.Context, item domain.ItemRef) (domain.SearchState, error)
	States(ctx context.Context, kind domain.MediaKind) (map[int64]domain.SearchState, error)
	RecordQuery(ctx context.Context, indexer domain.ObjectKey, at time.Time) error
	Queries(ctx context.Context, indexer domain.ObjectKey, since time.Time) (int, time.Time, error)
	PruneQueries(ctx context.Context, before time.Time) error
}

// allow checks an indexer's limit and, when there is room, logs the query.
func (s *Service) allow(ctx context.Context, ix Indexer) error {
	now := s.d.Now()
	if ix.Limit.Queries > 0 {
		n, oldest, err := s.d.Searches.Queries(ctx, ix.Key, now.Add(-ix.Limit.Per))
		if err != nil {
			return err
		}
		if n >= ix.Limit.Queries {
			return LimitError{Indexer: ix.Key, Resets: oldest.Add(ix.Limit.Per)}
		}
	}
	return s.d.Searches.RecordQuery(ctx, ix.Key, now)
}

// record stores the result of one search for the movie.
func (s *Service) record(ctx context.Context, m domain.Movie, res domain.SearchEventData) {
	st, err := s.d.Searches.State(ctx, m.Ref())
	s.warn("read search state", err, "movie", m.ID)
	at := s.d.Now().UTC()
	st.Misses++
	if res.Grabbed != "" {
		st.Misses = 0
	}
	next := NextEligible(at, st.Misses)
	st.LastSearched, st.NextEligible, st.LastResult = &at, &next, &res
	s.warn("save search state", s.d.Searches.SaveState(ctx, st), "movie", m.ID)
}

// Request queues an automatic search for a movie (on add, on availability,
// after a failure, backlog). Searches run one at a time in the background;
// a movie already waiting is not queued twice.
func (s *Service) Request(ctx context.Context, movieID int64) {
	s.queue.add(ctx, movieID, func(ctx context.Context, id int64) {
		_, err := s.Search(ctx, id, domain.OriginAutomatic)
		s.warn("search", err, "movie", id)
	})
}

// Wait blocks until the search queue is empty (shutdown and tests).
func (s *Service) Wait() { s.queue.wait() }

// Backlog queues the wanted movies that are due, within the budget, and
// prunes the query log.
func (s *Service) Backlog(ctx context.Context) error {
	if s.searchable() != nil {
		return nil
	}
	states, err := s.d.Searches.States(ctx, domain.KindMovie)
	if err != nil {
		return err
	}
	for _, id := range Due(s.index.IDs(), states, s.d.Now(), BacklogBudget) {
		s.Request(ctx, id)
	}
	return s.d.Searches.PruneQueries(ctx, s.d.Now().Add(-QueryLogKeep))
}

// searchQueue runs searches one at a time in FIFO order; an id waiting is
// queued once. Its goroutine lives only while there is work.
type searchQueue struct {
	mu      sync.Mutex
	pending []int64
	waiting map[int64]bool
	wg      sync.WaitGroup
}

func newSearchQueue() *searchQueue { return &searchQueue{waiting: map[int64]bool{}} }

func (q *searchQueue) add(ctx context.Context, id int64, run func(context.Context, int64)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.waiting[id] {
		return
	}
	q.waiting[id] = true
	q.pending = append(q.pending, id)
	if len(q.pending) == 1 {
		q.wg.Add(1)
		go q.drain(ctx, run)
	}
}

func (q *searchQueue) drain(ctx context.Context, run func(context.Context, int64)) {
	defer q.wg.Done()
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.mu.Unlock()
			return
		}
		id := q.pending[0]
		delete(q.waiting, id)
		q.mu.Unlock()
		if ctx.Err() == nil {
			run(ctx, id)
		}
		q.mu.Lock()
		q.pending = q.pending[1:]
		q.mu.Unlock()
	}
}

func (q *searchQueue) wait() { q.wg.Wait() }
