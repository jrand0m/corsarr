package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// PathChange is one file's old and new path.
type PathChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MovePlan previews moving an item folder: the folder and every library file in it.
type MovePlan struct {
	From  string       `json:"from"`
	To    string       `json:"to"`
	Files []PathChange `json:"files"`
}

// ErrMoveConflict says the destination folder already exists.
var ErrMoveConflict = errors.New("the destination folder already exists")

// rename is os.Rename; tests swap it to force the cross-filesystem path.
var rename = os.Rename

// Rebase maps a path under from onto to (pure); ok=false outside from.
func Rebase(path, from, to string) (string, bool) {
	rel, err := filepath.Rel(from, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(to, rel), true
}

// PlanMove lists what moving an item's folder from → to changes.
func (s *Service) PlanMove(ctx context.Context, item domain.ItemRef, from, to string) (MovePlan, []domain.LibraryFile, error) {
	plan := MovePlan{From: from, To: to, Files: []PathChange{}}
	files, err := s.d.Library.FilesOf(ctx, item)
	var moved []domain.LibraryFile
	for _, f := range files {
		if dest, ok := Rebase(f.Path, from, to); ok {
			plan.Files = append(plan.Files, PathChange{From: f.Path, To: dest})
			moved = append(moved, f)
		}
	}
	return plan, moved, err
}

// MoveItem moves an item folder from → to: a rename on one file system,
// else copy, verify and delete. It is idempotent (a folder already at the
// destination only gets its records updated), refuses to merge into an
// existing folder, updates the library file paths and publishes
// file.renamed per file (History; the media server refreshes both folders).
// Moving back undoes it.
func (s *Service) MoveItem(ctx context.Context, item domain.ItemRef, from, to string) (MovePlan, error) {
	if err := s.d.Modes.CheckActive(item.Kind); err != nil {
		return MovePlan{}, err
	}
	plan, files, err := s.PlanMove(ctx, item, from, to)
	if err != nil || from == to {
		return plan, err
	}
	if err := moveFolder(from, to, s.d.FreeSpace); err != nil {
		return plan, err
	}
	for i, f := range files {
		if err := s.d.Library.SetPath(ctx, f.ID, plan.Files[i].To); err != nil {
			return plan, err
		}
		s.publishMove(item, plan.Files[i])
	}
	return plan, nil
}

func (s *Service) publishMove(item domain.ItemRef, c PathChange) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvFileRenamed, Actor: "user", Kind: item.Kind,
		Items: []domain.EventItem{{Ref: item}}, Data: domain.FileEventData{Path: c.To, OldPath: c.From, Reason: "moved"}})
}

// moveFolder moves the folder; a missing source with the destination in
// place is a finished move, a missing source without it has nothing to move.
func moveFolder(from, to string, freeSpace func(string) int64) error {
	switch {
	case !exists(from):
		return nil
	case exists(to):
		return fmt.Errorf("%w: %s", ErrMoveConflict, to)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if rename(from, to) == nil {
		return nil
	}
	if err := copyTree(from, to, freeSpace); err != nil {
		_ = os.RemoveAll(to) // only what this move created: to did not exist
		return err
	}
	return os.RemoveAll(from)
}

// copyTree copies every directory and regular file, each file size-verified.
func copyTree(from, to string, freeSpace func(string) int64) error {
	return filepath.WalkDir(from, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest, _ := Rebase(p, from, to)
		switch {
		case de.IsDir():
			return os.MkdirAll(dest, 0o755)
		case de.Type().IsRegular():
			return copyFile(p, dest, freeSpace)
		}
		return nil // symlinks and devices are not part of a library folder
	})
}
