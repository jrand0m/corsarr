package plugin

import "testing"

func TestParsePathRule(t *testing.T) {
	good := map[string]PathRule{
		"/downloads=/mnt/data/downloads":        {Remote: "/downloads", Local: "/mnt/data/downloads"},
		" /downloads/ = /mnt/data/downloads/ ":  {Remote: "/downloads", Local: "/mnt/data/downloads"},
		`D:\Torrents=/data/torrents`:            {Remote: "D:/Torrents", Local: "/data/torrents"},
		`\\nas\share\dl=/srv/dl`:                {Remote: "//nas/share/dl", Local: "/srv/dl"},
		"/=/remote-root":                        {Remote: "/", Local: "/remote-root"},
		"/data/torrents=D:\\Torrents\\complete": {Remote: "/data/torrents", Local: "D:/Torrents/complete"},
	}
	for in, want := range good {
		got, err := ParsePathRule(in)
		if err != nil || got != want {
			t.Errorf("ParsePathRule(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "/a", "=/b", "/a=", "/a=/b=/c", "relative=/b", "/a=relative"} {
		if _, err := ParsePathRule(in); err == nil {
			t.Errorf("ParsePathRule(%q) accepted", in)
		}
		if CheckPathRule(in) == nil {
			t.Errorf("CheckPathRule(%q) accepted", in)
		}
	}
}

func TestParsePathMapRejectsDuplicates(t *testing.T) {
	if _, err := ParsePathMap([]string{"/a=/x", "/a/=/y"}); err == nil {
		t.Fatal("duplicate remote prefix accepted")
	}
	if _, err := ParsePathMap([]string{"/a=/x", "bad"}); err == nil {
		t.Fatal("bad entry accepted")
	}
	m, err := ParsePathMap(nil)
	if err != nil || len(m) != 0 {
		t.Fatal(m, err)
	}
}

func TestPathMapToLocal(t *testing.T) {
	m, err := ParsePathMap([]string{"/downloads=/mnt/dl", "/downloads/tv=/mnt/tv", `D:\Torrents=/data/t`, "/data/torrents=E:\\T"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"/downloads", "/mnt/dl", true},
		{"/downloads/", "/mnt/dl", true},
		{"/downloads/movies/A (2020)", "/mnt/dl/movies/A (2020)", true},
		{"/downloads/tv/Show", "/mnt/tv/Show", true}, // the longest prefix wins
		{"/downloadsextra/x", "/downloadsextra/x", false},
		{"/other/x", "/other/x", false},
		{`D:\Torrents\movies\A.mkv`, "/data/t/movies/A.mkv", true},
		{`d:\torrents\B`, "/data/t/B", true}, // Windows paths match case-insensitively
		{"/data/torrents/C/c.mkv", `E:\T\C\c.mkv`, true},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := m.ToLocal(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ToLocal(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPathMapToRemote(t *testing.T) {
	m, err := ParsePathMap([]string{"/downloads=/mnt/dl", `D:\Torrents=/data/t`})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := m.ToRemote("/mnt/dl/movies"); got != "/downloads/movies" || !ok {
		t.Fatal(got, ok)
	}
	if got, ok := m.ToRemote("/data/t/x"); got != `D:\Torrents\x` || !ok {
		t.Fatal(got, ok)
	}
	if got, ok := m.ToRemote("/elsewhere"); got != "/elsewhere" || ok {
		t.Fatal(got, ok)
	}
}

func TestRootRuleMapsEverything(t *testing.T) {
	m, _ := ParsePathMap([]string{"/=/host"})
	if got, ok := m.ToLocal("/a/b"); got != "/host/a/b" || !ok {
		t.Fatal(got, ok)
	}
	if got, ok := m.ToRemote("/host/a"); got != "/a" || !ok {
		t.Fatal(got, ok)
	}
}

func TestWindowsStyle(t *testing.T) {
	for p, want := range map[string]bool{`C:\x`: true, "c:/x": true, `\\nas\s`: true, "/x": false, "x": false, "C": false} {
		if WindowsStyle(p) != want {
			t.Errorf("WindowsStyle(%q) != %v", p, want)
		}
	}
}
