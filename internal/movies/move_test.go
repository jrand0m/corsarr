package movies

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

type fakeFolders struct {
	moves []string
	err   error
}

func (f *fakeFolders) Move(_ context.Context, _ domain.ItemRef, from, to string) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.moves = append(f.moves, from+" > "+to)
	return nil, nil
}

func (f *fakeFolders) Plan(_ context.Context, _ domain.ItemRef, from, to string) (any, error) {
	return map[string]string{"from": from, "to": to}, nil
}

func TestRootFolderChangeMovesTheFolder(t *testing.T) {
	e := newEnv(t)
	other := t.TempDir()
	e.svc.d.RootFolders = func() []string { return []string{e.root, other} }
	f := &fakeFolders{}
	e.svc.d.Folders = f
	m, _, _ := e.svc.Add(ctx, AddRequest{TMDBID: 603})
	plan, err := e.svc.PreviewMove(ctx, m.ID, other)
	if err != nil || plan.(map[string]string)["to"] != filepath.Join(other, "The Matrix (1999)") || len(f.moves) != 0 {
		t.Fatalf("preview: %v %v", plan, err)
	}
	if p, _ := e.svc.PreviewMove(ctx, m.ID, e.root); p.(map[string]any)["to"] != m.Path {
		t.Fatal("same root: nothing moves")
	}
	got, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &other})
	want := filepath.Join(other, "The Matrix (1999)")
	if err != nil || got.RootFolder != other || got.Path != want || len(f.moves) != 1 || f.moves[0] != m.Path+" > "+want {
		t.Fatalf("%+v %v %v", got, err, f.moves)
	}
	up := e.rec.Of(domain.EvItemUpdated)
	if ch := up[len(up)-1].Data.(domain.ItemEventData).Changed; len(ch) != 2 || ch[0] != "rootFolder" {
		t.Fatalf("changed %v", ch)
	}
	if _, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &other}); err != nil || len(f.moves) != 1 {
		t.Fatal("same root folder again: no move")
	}
	bad := "/not/a/root"
	var fe FieldError
	if _, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &bad}); !errors.As(err, &fe) || fe.Field != "rootFolder" {
		t.Fatalf("unknown root: %v", err)
	}
	f.err = errors.New("disk on fire")
	if _, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &e.root}); err == nil {
		t.Fatal("a failed move fails the edit")
	}
	if still, _ := e.svc.Get(ctx, m.ID); still.Path != want {
		t.Fatal("a failed move keeps the path")
	}
	e.svc.d.Modes = func(domain.MediaKind) domain.KindMode { return domain.ModeShadow }
	var me domain.ModeError
	if _, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &e.root}); !errors.As(err, &me) {
		t.Fatalf("shadow: %v", err)
	}
	e.svc.d.Modes, e.svc.d.Folders = nil, nil
	if _, err := e.svc.Update(ctx, m.ID, Patch{RootFolder: &e.root}); !errors.Is(err, ErrNoMover) {
		t.Fatal(err)
	}
	if _, err := e.svc.PreviewMove(ctx, 99, e.root); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
