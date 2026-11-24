package definition

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProblemLinesFollowPaths(t *testing.T) {
	var root yaml.Node
	_ = yaml.Unmarshal([]byte("a:\n  - x: 1\n  - y: 2\nb: [3]\n"), &root)
	for path, want := range map[string]int{"a[1].y": 3, "a[0]": 2, "a[9]": 2, "a[x]": 2, "b[0].z": 4, "zz": 1, "": 1} {
		if got := lineOf(&root, path); got != want {
			t.Errorf("%q: line %d, want %d", path, got, want)
		}
	}
	if (Problem{Msg: "m"}).String() != "m" || (Problem{Path: "p", Msg: "m"}).String() != "p: m" {
		t.Error("problem text")
	}
}
