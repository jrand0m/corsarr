package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseObjectKey(t *testing.T) {
	for _, ok := range []string{"qbit", "knaben2", "a"} {
		if _, err := ParseObjectKey(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "my_qbit", "myQbit", "2qbit", "q-bit"} {
		if _, err := ParseObjectKey(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestParseMediaKind(t *testing.T) {
	if k, err := ParseMediaKind("movie"); err != nil || k != KindMovie {
		t.Fatalf("movie: %v %v", k, err)
	}
	if _, err := ParseMediaKind("comic"); err == nil {
		t.Fatal("comic should be rejected")
	}
}

func TestCheckTransition(t *testing.T) {
	cases := []struct {
		from, to DownloadState
		by       Actor
		ok       bool
	}{
		{StateQueued, StateDownloading, ActorDownloader, true},
		{StateDownloading, StateCompleted, ActorDownloader, true},
		{StateCompleted, StateImportPending, ActorImport, true},
		{StateImporting, StateImported, ActorImport, true},
		{StateImportBlocked, StateImporting, ActorUser, true},
		{StateDownloading, StateRemoved, ActorSafety, true},
		{StateStalled, StateFailed, ActorAutomation, true},
		{StateQueued, StateStalled, ActorDownloader, true}, // a client warning before any progress
		{StateStalled, StateQueued, ActorDownloader, true},
		// wrong actor: automation cannot import, downloader cannot import
		{StateCompleted, StateImportPending, ActorDownloader, false},
		{StateImporting, StateImported, ActorAutomation, false},
		// illegal edges
		{StateImported, StateDownloading, ActorDownloader, false},
		{StateRemoved, StateQueued, ActorUser, false},
		{StateImported, StateFailed, ActorUser, false},
		{StateFailed, StateImportPending, ActorUser, true},
		{StateFailed, StateImportPending, ActorImport, false},
	}
	for _, c := range cases {
		err := CheckTransition(c.from, c.to, c.by)
		if (err == nil) != c.ok {
			t.Errorf("%s→%s by %s: got err=%v, want ok=%v", c.from, c.to, c.by, err, c.ok)
		}
	}
}

func TestOverrideFor(t *testing.T) {
	if got := OverrideFor(nil); got != OverrideAllowed {
		t.Errorf("no reasons: %s", got)
	}
	if got := OverrideFor([]Reason{{Category: ReasonPreference}, {Category: ReasonPermanent}}); got != OverrideConfirm {
		t.Errorf("permanent: %s", got)
	}
	if got := OverrideFor([]Reason{{Category: ReasonPermanent}, {Category: ReasonSafety}}); got != OverrideNever {
		t.Errorf("safety: %s", got)
	}
}

func TestTerminal(t *testing.T) {
	if !StateImported.Terminal() || !StateRemoved.Terminal() || StateFailed.Terminal() {
		t.Fatal("terminal states wrong")
	}
}

func TestOnHold(t *testing.T) {
	for _, c := range []struct {
		d    Download
		want bool
	}{{Download{}, false}, {Download{Held: true}, true}, {Download{Category: HoldCategory}, true}, {Download{Category: "movies"}, false}} {
		if c.d.OnHold() != c.want {
			t.Errorf("%+v: %v", c.d, !c.want)
		}
	}
}

func TestModes(t *testing.T) {
	var none Modes
	if !none.Active(KindMovie) || none.CheckActive(KindBook) != nil {
		t.Fatal("nil Modes: everything active")
	}
	m := Modes(func(k MediaKind) KindMode {
		return map[MediaKind]KindMode{KindMovie: ModeShadow, KindBook: ModeDisabled, KindTV: "bogus"}[k]
	})
	if m.Of(KindMovie) != ModeShadow || m.Of(KindBook) != ModeDisabled || m.Of(KindTV) != ModeActive || m.Of(KindMusic) != ModeActive {
		t.Fatal("Of")
	}
	err := m.CheckActive(KindMovie)
	var me ModeError
	if !errors.As(err, &me) || me.Mode != ModeShadow || !strings.Contains(err.Error(), "shadow") {
		t.Fatalf("CheckActive: %v", err)
	}
	mixed := []EventItem{{Ref: ItemRef{Kind: KindTV}}, {Ref: ItemRef{Kind: KindMovie}}}
	if m.AllActive(mixed) || !m.AllActive(mixed[:1]) || !m.AllActive(nil) {
		t.Fatal("AllActive")
	}
}

func TestArtLink(t *testing.T) {
	var none ArtLink
	if none.Of(ArtPoster, "https://a/p.jpg") != "https://a/p.jpg" {
		t.Fatal("nil link must keep the address")
	}
	link := ArtLink(func(r ArtRole, o string) string { return string(r) + ":" + o })
	if link.Of(ArtBackdrop, "x") != "b:x" || link.Of(ArtPoster, "") != "" {
		t.Fatal("link")
	}
}
