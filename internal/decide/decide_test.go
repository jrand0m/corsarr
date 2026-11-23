package decide

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

var now = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func profile(t *testing.T, name string) QualityProfile {
	t.Helper()
	for _, p := range DefaultProfiles(domain.KindMovie) {
		if p.Spec().Name == name {
			return p
		}
	}
	t.Fatalf("no default profile %q", name)
	return QualityProfile{}
}

// input builds a happy-path movie decision input; tests then change one thing.
func input(t *testing.T, title string, size int64) Input {
	return Input{
		Release: domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: size,
			PublishedAt: now.Add(-48 * time.Hour), Seeders: 50,
			Indexers: []domain.IndexerRef{{Key: "idx", Name: "idx", Priority: 25}}},
		Parsed:  parse.ParseRelease(title),
		Item:    Item{Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 1}, Monitored: true, Available: true, Profile: profile(t, "Any")},
		Context: Context{Now: now},
	}
}

func ruleIDs(d domain.Decision) []string {
	ids := []string{}
	for _, r := range d.Reasons {
		ids = append(ids, r.RuleID)
	}
	return ids
}

func TestDecide_60GB3DISO_RejectedByIndependentRules(t *testing.T) {
	iso := input(t, "The.Quiet.Valley.2019.1080p.3D.BluRay.AVC.ISO", 60*gb)
	web := input(t, "The.Quiet.Valley.2019.1080p.WEB-DL.DDP5.1.H.264-GRP", 6*gb)
	di, dw := Decide(iso), Decide(web)

	if di.Accepted {
		t.Fatal("the 60 GB 3D ISO must be rejected")
	}
	want := []string{"disc_image", "three_d", "quality.not_allowed", "size.max"}
	if got := ruleIDs(di); !reflect.DeepEqual(got, want) {
		t.Fatalf("reasons %v, want %v", got, want)
	}
	size := di.Reasons[3]
	if size.Message != "size 60.0 GB exceeds max 8.4 GB for Disc-1080p at 120 min (default runtime, item runtime unknown)" {
		t.Errorf("size message: %s", size.Message)
	}
	if size.Category != domain.ReasonPermanent || di.Override != domain.OverrideConfirm {
		t.Errorf("category %s override %s", size.Category, di.Override)
	}
	if !dw.Accepted {
		t.Fatalf("6 GB 1080p WEB release should be accepted: %v", dw.Reasons)
	}
	if ranked := Rank([]domain.Decision{di, dw}); ranked[0].ReleaseID != web.Release.ID {
		t.Fatal("the WEB release must rank first")
	}
}

func TestDecide_ScrMalware_IsSafetyAndNeverOverridable(t *testing.T) {
	in := input(t, "The.Quiet.Valley.2019.1080p.WEB-DL.H264-GRP", 2_000_000)
	in.Release.FileList = []string{"The.Quiet.Valley.2019.1080p.WEB-DL.H264-GRP/The.Quiet.Valley.2019.1080p.scr", "readme.txt"}
	d := Decide(in)
	if d.Accepted || d.Override != domain.OverrideNever {
		t.Fatalf("accepted %v override %s", d.Accepted, d.Override)
	}
	if d.Reasons[0].RuleID != "payload.dangerous" || d.Reasons[0].Category != domain.ReasonSafety ||
		!strings.Contains(d.Reasons[0].Message, ".scr") {
		t.Fatalf("first reason %+v", d.Reasons[0])
	}
	// every rule still runs: the 2 MB file is also too small
	if got := ruleIDs(d); !reflect.DeepEqual(got, []string{"payload.dangerous", "size.min"}) {
		t.Fatalf("reasons %v", got)
	}
}

func TestDecide_LargestFileNotVideo(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 4*gb)
	in.Files = []PayloadFile{{"movie.zip", 4 * gb}, {"sample.mkv", 1e7}}
	d := Decide(in)
	if got := ruleIDs(d); !reflect.DeepEqual(got, []string{"payload.not_video"}) || d.Override != domain.OverrideNever {
		t.Fatalf("reasons %v override %s", got, d.Override)
	}
}

func TestDecide_EveryRuleReportsWithCategory(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Input)
		rule   string
		cat    domain.ReasonCategory
	}{
		{"blocklisted", func(in *Input) { in.Context.Blocklisted = true }, "blocklist", domain.ReasonPermanent},
		{"grabbed before", func(in *Input) { in.Context.GrabbedBefore = true }, "history.grabbed", domain.ReasonPermanent},
		{"profile for another kind", func(in *Input) { in.Item.Ref.Kind, in.Release.Size = domain.KindTV, 2*gb }, "profile.kind", domain.ReasonPermanent},
		{"too small", func(in *Input) { in.Release.Size = 100_000_000 }, "size.min", domain.ReasonPermanent},
		{"resolution cap", func(in *Input) { in.Context.MaxResolution = 720 }, "search.max_resolution", domain.ReasonPreference},
		{"unmonitored", func(in *Input) { in.Item.Monitored = false }, "item.unmonitored", domain.ReasonPreference},
		{"not available", func(in *Input) { in.Item.Available = false }, "item.not_available", domain.ReasonTemporary},
		{"queued", func(in *Input) { in.Context.QueuedEqualOrBetter = true }, "queue.equal_or_better", domain.ReasonTemporary},
		{"too new", func(in *Input) { in.Context.MinAge = 72 * time.Hour }, "release.too_new", domain.ReasonTemporary},
		{"few seeders", func(in *Input) { in.Context.MinSeeders = 100 }, "seeders.min", domain.ReasonTemporary},
		{"not an upgrade", func(in *Input) { in.Item.Current = &CurrentFile{Quality: "bluray-1080p"} }, "upgrade.not_better", domain.ReasonPreference},
		{"quality outside profile", func(in *Input) { in.Item.Profile = profile(t, "Ultra-HD") }, "quality.not_allowed", domain.ReasonPermanent},
	}
	for _, c := range cases {
		in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 5*gb)
		c.change(&in)
		d := Decide(in)
		if d.Accepted || len(d.Reasons) != 1 || d.Reasons[0].RuleID != c.rule || d.Reasons[0].Category != c.cat {
			t.Errorf("%s: got %+v", c.name, d.Reasons)
			continue
		}
		if d.Override != domain.OverrideFor(d.Reasons) {
			t.Errorf("%s: override %s", c.name, d.Override)
		}
	}
}

func TestDecide_NoShortCircuit(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 100_000_000)
	in.Context.Blocklisted = true
	in.Context.QueuedEqualOrBetter = true
	in.Item.Monitored = false
	d := Decide(in)
	want := []string{"blocklist", "size.min", "item.unmonitored", "queue.equal_or_better"}
	if got := ruleIDs(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("reasons %v, want %v", got, want)
	}
	if d.Reasons[1].Values["limit"].(int64) != 720_000_000 {
		t.Errorf("size.min values %v", d.Reasons[1].Values)
	}
}

func TestDecide_AcceptedFieldsAndOverride(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 5*gb)
	d := Decide(in)
	if !d.Accepted || d.Override != domain.OverrideAllowed || d.Quality != "WEBDL-1080p" || d.Resolution != 1080 ||
		d.ReleaseID != in.Release.ID || !reflect.DeepEqual(d.ItemRefs, []domain.ItemRef{in.Item.Ref}) || d.Reasons == nil {
		t.Fatalf("%+v", d)
	}
}

func TestDecide_KnownRuntimeAndUsenetSkipsSeeders(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 7_500_000_000)
	in.Item.RuntimeMinutes = 100 // max 7.0 GB
	in.Release.Protocol = domain.ProtoUsenet
	in.Release.Seeders = 0
	in.Context.MinSeeders = 5
	d := Decide(in)
	if got := ruleIDs(d); !reflect.DeepEqual(got, []string{"size.max"}) {
		t.Fatalf("reasons %v", got)
	}
	if d.Reasons[0].Message != "size 7.5 GB exceeds max 7.0 GB for WEBDL-1080p at 100 min" {
		t.Fatalf("message %s", d.Reasons[0].Message)
	}
}

func TestDecide_FormatScoreBelowMinimum(t *testing.T) {
	bad := mustFormat(t, "Bad group", mustCond(t, "grp", `-BADGRP$`, false, false))
	spec := profile(t, "HD-1080p").Spec()
	spec.Formats = []FormatScore{{bad, -1000}}
	p, err := NewQualityProfile(spec)
	if err != nil {
		t.Fatal(err)
	}
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-BADGRP", 5*gb)
	in.Item.Profile = p
	d := Decide(in)
	if got := ruleIDs(d); !reflect.DeepEqual(got, []string{"format.min_score"}) || d.FormatScore != -1000 {
		t.Fatalf("reasons %v score %d", got, d.FormatScore)
	}
}

func TestDecide_AllowedDiscAnd3DWhenProfileSaysSo(t *testing.T) {
	spec := profile(t, "HD-1080p").Spec()
	spec.AllowDiscImage, spec.Allow3D = true, true
	spec.Groups = append(spec.Groups, QualityGroup{Qualities: []QualityID{"disc-1080p"}})
	p, err := NewQualityProfile(spec)
	if err != nil {
		t.Fatal(err)
	}
	in := input(t, "Movie.2020.1080p.3D.BluRay.AVC.ISO", 8*gb)
	in.Item.Profile = p
	if d := Decide(in); !d.Accepted {
		t.Fatalf("reasons %v", ruleIDs(d))
	}
}

func TestDecide_UpgradeAcceptedAndUnknownAgeNotDelayed(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 5*gb)
	in.Item.Current = &CurrentFile{Quality: "webdl-720p"}
	in.Release.PublishedAt = time.Time{}
	in.Context.MinAge = time.Hour
	if d := Decide(in); !d.Accepted {
		t.Fatalf("reasons %v", ruleIDs(d))
	}
}

func TestDecide_ExtensionlessLargestFileIsNotVideo(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 5*gb)
	in.Files = []PayloadFile{{"Movie/payload", 5 * gb}}
	if got := ruleIDs(Decide(in)); !reflect.DeepEqual(got, []string{"payload.not_video"}) {
		t.Fatalf("reasons %v", got)
	}
}

func TestRank_Order(t *testing.T) {
	mk := func(title string, size int64, seeders int, change func(*Input)) domain.Decision {
		in := input(t, title, size)
		in.Release.Seeders = seeders
		if change != nil {
			change(&in)
		}
		return Decide(in)
	}
	bluray := mk("Movie.2020.1080p.BluRay.x264-A", 8*gb, 10, nil)
	web := mk("Movie.2020.1080p.WEB-DL.H264-B", 5*gb, 500, nil)
	webFewSeeds := mk("Movie.2020.1080p.WEB-DL.H264-C", 5*gb, 2, nil)
	webProper := mk("Movie.2020.PROPER.1080p.WEB-DL.H264-D", 5*gb, 2, nil)
	hd720 := mk("Movie.2020.720p.BluRay.x264-E", 3*gb, 1000, nil)
	rejected := mk("Movie.2020.2160p.BluRay.x265-F", 20*gb, 1000, nil)
	usenet := mk("Movie.2020.1080p.WEB-DL.H264-G", 5*gb, 0, func(in *Input) {
		in.Release.Protocol = domain.ProtoUsenet
		in.Context.PreferredProtocol = domain.ProtoUsenet
	})

	got := Rank([]domain.Decision{rejected, hd720, webFewSeeds, web, bluray, webProper, usenet})
	var order []string
	for _, d := range got {
		order = append(order, d.ReleaseID[len(d.ReleaseID)-1:])
	}
	// A: Bluray outranks the WEB group; D: proper first within WEB; G: preferred
	// protocol; B: more seeders; C; E: lower quality; F: rejected last.
	if strings.Join(order, "") != "ADGBCEF" {
		t.Fatalf("order %v", order)
	}
}

func TestRank_TieBreakAndDoesNotMutate(t *testing.T) {
	a := domain.Decision{ReleaseID: "b", Accepted: true, RankKey: "1"}
	b := domain.Decision{ReleaseID: "a", Accepted: true, RankKey: "1"}
	in := []domain.Decision{a, b}
	out := Rank(in)
	if out[0].ReleaseID != "a" || in[0].ReleaseID != "b" {
		t.Fatalf("out %v in %v", out, in)
	}
}

func TestRankKeyParts(t *testing.T) {
	if priorityPart(nil) != 0 || priorityPart([]domain.IndexerRef{{Priority: 50}, {Priority: 10}}) != 89 {
		t.Error("priority")
	}
	if closeness(5*gb, 6*gb) <= closeness(1*gb, 6*gb) || closeness(9*gb, 6*gb) != closeness(3*gb, 6*gb) {
		t.Error("closeness")
	}
	src := domain.Release{Protocol: domain.ProtoSource}
	if seedersBucket(src) != 4 || seedersBucket(domain.Release{Protocol: domain.ProtoTorrent, Seeders: 0}) != 0 {
		t.Error("seeders")
	}
}

func TestDecide_MaxResolutionIsInclusive(t *testing.T) {
	in := input(t, "Movie.2020.1080p.WEB-DL.H264-GRP", 4*gb)
	in.Context.MaxResolution = 1080
	for _, r := range Decide(in).Reasons {
		if r.RuleID == "search.max_resolution" {
			t.Fatalf("a release at the cap must pass: %+v", r)
		}
	}
}
