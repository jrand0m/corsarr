package imagecache

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/store"
)

func TestBuckets(t *testing.T) {
	cases := []struct{ w, stored, want int }{
		{0, 600, 0}, {-5, 600, 0}, {40, 600, 96}, {96, 600, 96}, {97, 600, 160}, {500, 600, 0}, {600, 600, 0},
		{700, 1280, 800}, {2000, 4000, 0}, {100, 120, 0},
	}
	for _, c := range cases {
		if got := Bucket(c.w, c.stored); got != c.want {
			t.Errorf("Bucket(%d, %d) = %d, want %d", c.w, c.stored, got, c.want)
		}
	}
}

func TestShrinkSteps(t *testing.T) {
	d := Degrade{QualityFloor: 60, QualityStep: 10}
	for in, want := range map[int]int{82: 72, 72: 62, 62: 60, 60: 60, 50: 50} {
		if got := d.nextQuality(in); got != want {
			t.Errorf("nextQuality(%d) = %d, want %d", in, got, want)
		}
	}
	if (Degrade{QualityFloor: 60}).nextQuality(80) != 80 {
		t.Error("a zero step never changes quality")
	}
	for _, c := range []struct {
		w, floor int
		step     float64
		want     int
	}{{600, 300, 0.75, 450}, {450, 300, 0.75, 338}, {338, 300, 0.75, 300}, {300, 300, 0.75, 300}, {600, 300, 1, 600}, {600, 300, 0, 600}} {
		if got := nextWidth(c.w, c.floor, c.step); got != c.want {
			t.Errorf("nextWidth(%d) = %d, want %d", c.w, got, c.want)
		}
	}
	s := testSettings("")
	if s.widthFloor(RolePoster) != 300 || s.widthFloor(RoleBackdrop) != 640 || s.maxWidth(RoleBackdrop) != 1280 {
		t.Fatal("role widths")
	}
	s.MaxSize = 1000
	if s.high() != 900 || s.low() != 800 {
		t.Fatal(s.high(), s.low())
	}
}

func TestEvictionOrderKeepsLibraryArtworkLast(t *testing.T) {
	lru := []store.ImageEntry{
		{Key: "lib-old", Origin: "L1"}, {Key: "other-old", Origin: "O1"}, {Key: "var", Origin: "L1", Variant: 160},
		{Key: "lib-new", Origin: "L2"}, {Key: "other-new", Origin: "O2"},
	}
	got := evictionOrder(lru, map[string]bool{"L1": true, "L2": true})
	want := []string{"var", "other-old", "other-new", "lib-old", "lib-new"}
	for i, e := range got {
		if e.Key != want[i] {
			t.Fatalf("order %v", got)
		}
	}
	if lru[0].Key != "lib-old" {
		t.Fatal("input reordered")
	}
}
