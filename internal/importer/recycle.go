package importer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/store"
)

// Rule ids registered by the importer with the kernel rule runner.
const (
	RuleUpgradeReplace = "upgradereplace" // recycle the file an upgrade replaced
	RuleItemDelete     = "itemdelete"     // recycle the files of a deleted item
	RuleRecyclePurge   = "recyclepurge"   // delete old recycled files
)

// recycling are the rules that move files into the recycle area.
var recycling = []string{RuleUpgradeReplace, RuleItemDelete}

// RecycleKeep is how long recycled files stay before the purge rule removes them.
const RecycleKeep = 7 * 24 * time.Hour

// moveRecord is the undo payload of a recycle: where the file went and came from.
type moveRecord struct {
	From string `json:"from"` // original path
	To   string `json:"to"`   // path inside the recycle area
}

// RegisterRules adds upgradereplace and itemdelete (live: moving to the
// recycle area is reversible) and recyclepurge (default purgeMode;
// destructive rules ship dry-run) to the runner.
func (s *Service) RegisterRules(r *rules.Runner, purgeMode rules.Mode) error {
	for id, what := range map[string]string{RuleUpgradeReplace: "the file an upgrade replaced", RuleItemDelete: "the files of a deleted item"} {
		if err := r.Register(s.recycleRule(id, "Move "+what+" to the recycle area.")); err != nil {
			return err
		}
	}
	return r.Register(rules.Rule{ID: RuleRecyclePurge, Mode: purgeMode, Cap: 100,
		Description: "Delete recycled files older than 7 days.",
		Find:        func(ctx context.Context) ([]rules.Candidate, error) { return s.purgeCandidates(ctx) },
		Recheck:     func(_ context.Context, c rules.Candidate) (bool, error) { return exists(c.Key), nil },
		Act: func(_ context.Context, c rules.Candidate) (string, error) {
			return "", os.RemoveAll(filepath.Dir(c.Key)) // one entry folder per recycled file
		}})
}

func (s *Service) recycleRule(id, description string) rules.Rule {
	return rules.Rule{ID: id, Mode: rules.ModeLive, Description: description,
		Recheck: func(_ context.Context, c rules.Candidate) (bool, error) { return exists(c.Key), nil },
		Act:     func(_ context.Context, c rules.Candidate) (string, error) { return s.recycle(c.Key) },
		Undo:    func(_ context.Context, p string) error { return restore(p) }}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// recycle moves path into <data>/recycle/<stamp>/<name> and returns the undo payload.
func (s *Service) recycle(path string) (string, error) {
	stamp := strconv.FormatInt(s.d.Now().UnixNano(), 10)
	to := filepath.Join(s.recycleDir(), stamp, filepath.Base(path))
	if err := move(path, to); err != nil {
		return "", err
	}
	b, _ := json.Marshal(moveRecord{From: path, To: to}) // plain strings
	return string(b), nil
}

func (s *Service) recycleDir() string { return filepath.Join(s.d.DataDir, "recycle") }

func restore(payload string) error {
	var m moveRecord
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return err
	}
	if exists(m.From) {
		return errors.New("a file already exists at " + m.From)
	}
	if err := move(m.To, m.From); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(m.To)) // the entry folder, now empty
	return nil
}

// purgeCandidates are recycled files older than RecycleKeep that still exist.
func (s *Service) purgeCandidates(ctx context.Context) ([]rules.Candidate, error) {
	var out []rules.Candidate
	for _, id := range recycling {
		err := s.d.RuleStore.ActedBefore(ctx, id, s.d.Now().Add(-RecycleKeep), func(a store.RuleAction) error {
			var m moveRecord
			if json.Unmarshal([]byte(a.Undo), &m) == nil && exists(m.To) {
				out = append(out, rules.Candidate{Key: m.To, Evidence: map[string]any{"recycledAt": a.CreatedAt, "from": m.From},
					Ops: []string{"delete " + m.To}})
			}
			return nil
		})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
