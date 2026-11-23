package parse

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// alwaysChecked fields are compared on every corpus line; a missing expectation
// means "unknown", so false positives count as errors.
var alwaysChecked = []string{"title", "year", "res", "source", "codec", "group", "ep", "3d", "disc"}

// summary renders a ParsedRelease as the flat string map the corpus uses.
func summary(p ParsedRelease) map[string]string {
	m := map[string]string{
		"title": p.Title, "year": intField(p.Year), "res": strconv.Itoa(int(p.Resolution.Value)),
		"source": string(p.Source.Value), "codec": string(p.Codec.Value), "group": p.Group.Value,
		"ep": episodeString(p.Episode), "3d": boolStr(p.ThreeD), "disc": string(p.Disc.Value),
		"hdr": joinHDR(p.HDR), "dv": intField(p.DVProfile), "audio": string(p.Audio.Value),
		"ch": p.Channels.Value, "atmos": boolStr(p.Atmos), "proper": boolStr(p.Proper),
		"repack": boolStr(p.Repack), "version": intField(p.Version), "edition": p.Edition.Value,
		"lang": strings.Join(p.Languages, ","), "aka": strings.Join(p.Titles, "|"),
	}
	if !p.Resolution.Known {
		m["res"] = ""
	}
	return m
}

func intField(f Field[int]) string {
	if !f.Known {
		return ""
	}
	return strconv.Itoa(f.Value)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return ""
}

func joinHDR(h []HDR) string {
	s := make([]string, len(h))
	for i, v := range h {
		s[i] = string(v)
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func episodeString(e Episode) string {
	var b strings.Builder
	switch e.Kind {
	case EpisodeDaily:
		return e.AirDate
	case EpisodeStandard:
		fmt.Fprintf(&b, "S%02d", e.Seasons[0])
		for _, n := range e.Episodes {
			fmt.Fprintf(&b, "E%02d", n)
		}
	case EpisodeSeason:
		for _, n := range e.Seasons {
			fmt.Fprintf(&b, "S%02d", n)
		}
	}
	return b.String()
}

func loadCorpus(t *testing.T) []map[string]string {
	t.Helper()
	f, err := os.Open("testdata/corpus.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []map[string]string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m, err := parseCorpusLine(line)
		if err != nil {
			t.Fatalf("corpus line %d: %v", n, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// TestCorpus parses every synthetic title and reports field accuracy; it
// fails below 95 %.
func TestCorpus(t *testing.T) {
	lines := loadCorpus(t)
	if len(lines) < 150 {
		t.Fatalf("corpus has %d titles, want >= 150", len(lines))
	}
	checks, wrong, perfect := 0, 0, 0
	for _, want := range lines {
		got := summary(ParseRelease(want["t"]))
		bad := corpusMismatches(want, got)
		checks += len(fieldsFor(want))
		wrong += len(bad)
		if len(bad) == 0 {
			perfect++
		}
		for _, b := range bad {
			t.Logf("%s\n    %s", want["t"], b)
		}
	}
	acc := 100 * float64(checks-wrong) / float64(checks)
	t.Logf("corpus: %d titles, %d field checks, %.2f%% field accuracy, %d/%d titles fully correct",
		len(lines), checks, acc, perfect, len(lines))
	if acc < 95 {
		t.Fatalf("field accuracy %.2f%% < 95%%", acc)
	}
}

func fieldsFor(want map[string]string) []string {
	fields := append([]string(nil), alwaysChecked...)
	for k := range want {
		if k != "t" && !contains(alwaysChecked, k) {
			fields = append(fields, k)
		}
	}
	return fields
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func corpusMismatches(want, got map[string]string) []string {
	var bad []string
	for _, k := range fieldsFor(want) {
		if want[k] != got[k] {
			bad = append(bad, fmt.Sprintf("%s: want %q got %q", k, want[k], got[k]))
		}
	}
	sort.Strings(bad)
	return bad
}

// parseCorpusLine reads `title|field=value;field=value`.
func parseCorpusLine(line string) (map[string]string, error) {
	title, exp, ok := strings.Cut(line, "|")
	if !ok {
		return nil, fmt.Errorf("missing '|' in %q", line)
	}
	m := map[string]string{"t": title}
	for _, kv := range strings.Split(exp, ";") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("bad field %q", kv)
		}
		m[k] = v
	}
	return m, nil
}
