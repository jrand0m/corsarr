package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

func emptyQuality() State {
	return State{Quality: QualityState{Formats: map[string]string{}, Profiles: map[string]string{}}}
}

func qualityItem(id int, name string, members ...string) SrcQualityItem {
	return SrcQualityItem{ID: id, Name: name, Group: len(members) > 1, Allowed: true, Members: members}
}

// hd1080p is a Sonarr profile with the content of the built-in HD-1080p.
func hd1080p(id int) SrcProfile {
	return SrcProfile{ID: id, Name: "HD-1080p", Upgrades: true, CutoffID: 3, Cutoff: "Bluray-1080p", Items: []SrcQualityItem{
		qualityItem(1, "HDTV-1080p", "HDTV-1080p"), qualityItem(1001, "WEB 1080p", "WEBRip-1080p", "WEBDL-1080p"),
		qualityItem(3, "Bluray-1080p", "Bluray-1080p")}}
}

func TestPlan_AProfileLikeTheBuiltinUsesIt(t *testing.T) {
	snap := Snapshot{Source: SourceInfo{App: Sonarr}, Profiles: map[int]SrcProfile{4: hd1080p(4)}}
	rep := BuildPlan([]Snapshot{snap}, emptyQuality(), Options{})
	e := entry(t, rep, "sonarr:profile:4")
	if e.Target != "profile:hd1080p" || e.Outcome != OutUnchanged || e.Reason != "same as the built-in profile hd1080p" || e.op != nil {
		t.Fatalf("%s %s %s %v", e.Target, e.Outcome, e.Reason, e.op)
	}
	n := 0
	for _, e := range rep.Entries {
		if e.ID == "sonarr:profile:4" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the profile is planned %d times", n)
	}
}

func TestPlan_ACustomProfileInForceIsNotABuiltin(t *testing.T) {
	mine := decide.ProfileDoc{Key: "mine", Name: "Mine", Kinds: []domain.MediaKind{domain.KindTV},
		Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-1080p"}}}, Cutoff: "webdl-1080p", MinUpgradeFormatScore: 1}
	cat, errs := decide.BuildCatalog(nil, []decide.ProfileDoc{mine})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	defer decide.WithCatalog(cat)()
	snap := Snapshot{Source: SourceInfo{App: Sonarr}, Profiles: map[int]SrcProfile{1: {ID: 1, Name: "Mine", CutoffID: 1,
		Items: []SrcQualityItem{qualityItem(1, "WEBDL-1080p", "WEBDL-1080p")}}}}
	e := entry(t, BuildPlan([]Snapshot{snap}, emptyQuality(), Options{}), "sonarr:profile:1")
	if e.Target != "profile:mine" || e.Outcome != OutCreate || e.op == nil {
		t.Fatalf("%s %s %s", e.Target, e.Outcome, e.Reason)
	}
}

func TestPlan_KeysNeverRepeatOrTakeABuiltinKey(t *testing.T) {
	res := []SrcSpec{spec("Resolution", map[string]string{"value": "1080"}, nil)}
	snap := Snapshot{Source: SourceInfo{App: Sonarr},
		CustomFormats: []SrcFormat{{ID: 1, Name: "Mine", Specs: res}, {ID: 2, Name: "HD-1080p", Specs: res}},
		Profiles: map[int]SrcProfile{1: {ID: 1, Name: "Mine", CutoffID: 1, Items: []SrcQualityItem{qualityItem(1, "WEBDL-720p", "WEBDL-720p")},
			Scores: []SrcScore{{Format: 1, Name: "Mine", Score: 10}}}}}
	rep := BuildPlan([]Snapshot{snap}, emptyQuality(), Options{})
	want := map[string]string{"sonarr:customformat:1": "customformat:mine", "sonarr:customformat:2": "customformat:hd1080p2",
		"sonarr:profile:1": "profile:mine2"}
	for id, target := range want {
		if e := entry(t, rep, id); e.Target != target || e.Outcome != OutCreate {
			t.Errorf("%s: %s %s %s", id, e.Target, e.Outcome, e.Reason)
		}
	}
}

func TestPlan_AFormatWithNothingCompilableIsUnsupported(t *testing.T) {
	snap := Snapshot{Source: SourceInfo{App: Sonarr}, CustomFormats: []SrcFormat{{ID: 1, Name: "Not DTS",
		Specs: []SrcSpec{spec("ReleaseTitle", map[string]string{"value": `^(?!.*DTS).*$`}, nil)}}}}
	e := entry(t, BuildPlan([]Snapshot{snap}, emptyQuality(), Options{}), "sonarr:customformat:1")
	if e.Outcome != OutUnsupported || !strings.Contains(e.Reason, "title condition was left out") {
		t.Fatalf("%s %s", e.Outcome, e.Reason)
	}
}

func TestPlan_AProfileThatDoesNotCompileIsUnsupported(t *testing.T) {
	long := strings.Repeat("Long name ", 9)
	res := []SrcSpec{spec("Resolution", map[string]string{"value": "1080"}, nil)}
	snap := Snapshot{Source: SourceInfo{App: Sonarr}, CustomFormats: []SrcFormat{{ID: 1, Name: long, Specs: res}},
		Profiles: map[int]SrcProfile{1: {ID: 1, Name: "Scored", CutoffID: 1, Items: []SrcQualityItem{qualityItem(1, "WEBDL-720p", "WEBDL-720p")},
			Scores: []SrcScore{{Format: 1, Name: long, Score: 10}}}}}
	rep := BuildPlan([]Snapshot{snap}, emptyQuality(), Options{})
	p := entry(t, rep, "sonarr:profile:1")
	if p.Outcome != OutUnsupported || !strings.Contains(p.Reason, "the profile could not be carried over: ") || p.op != nil {
		t.Fatalf("%s %s", p.Outcome, p.Reason)
	}
	f := entry(t, rep, "sonarr:customformat:1")
	if op, ok := f.op.(qualityOp); !ok || len(op.formats) != 1 || len(op.formats[0].Conditions) != 1 {
		t.Fatalf("the format keeps its condition: %+v", f.op)
	}
}

func TestPlan_LaterIdenticalObjectsNameTheFirstSource(t *testing.T) {
	rep := BuildPlan([]Snapshot{ownerShaped(Sonarr, 1, 1), ownerShaped(Radarr, 1, 1)}, emptyQuality(), Options{})
	for _, id := range []string{"radarr:customformat:101", "radarr:profile:1"} {
		if e := entry(t, rep, id); e.Outcome != OutUnchanged || !strings.Contains(e.Reason, "identical to the Sonarr ") {
			t.Errorf("%s: %s %s", id, e.Outcome, e.Reason)
		}
	}
}

// qualityTarget records quality writes and fails the table it is told to.
type qualityTarget struct {
	Target
	fail  store.QualityTable
	saved []string
}

func (q *qualityTarget) SaveQuality(_ context.Context, t store.QualityTable, o store.QualityObject, _ time.Time) error {
	if t == q.fail {
		return errors.New("disk full")
	}
	q.saved = append(q.saved, string(t)+":"+o.Key)
	return nil
}

func TestQualityOpWritesFormatsThenTheProfile(t *testing.T) {
	op := qualityOp{formats: []decide.FormatDoc{{Key: "a", Name: "A"}, {Key: "b", Name: "B"}}, profile: &decide.ProfileDoc{Key: "p", Name: "P"}}
	ctx := context.Background()
	tgt := &qualityTarget{}
	target, err := op.apply(ctx, &applier{t: tgt})
	want := string(store.TableFormats) + ":a," + string(store.TableFormats) + ":b," + string(store.TableProfiles) + ":p"
	if err != nil || target != "profile:p" || strings.Join(tgt.saved, ",") != want {
		t.Fatalf("%q %v %v", target, err, tgt.saved)
	}
	if target, err := (qualityOp{formats: op.formats}).apply(ctx, &applier{t: &qualityTarget{}}); err != nil || target != "customformat:b" {
		t.Fatalf("formats only: %q %v", target, err)
	}
	for _, table := range []store.QualityTable{store.TableFormats, store.TableProfiles} {
		tgt := &qualityTarget{fail: table}
		if target, err := op.apply(ctx, &applier{t: tgt}); err == nil || target != "" {
			t.Errorf("%s fails: %q %v", table, target, err)
		}
	}
}
