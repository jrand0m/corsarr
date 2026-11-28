package plaintext

import (
	"strings"
	"testing"
	"unicode"
)

func TestAccepts(t *testing.T) {
	for _, s := range []string{
		"Host name", "Ünïcödé façade", "Größe: 10 MiB (max.)", "a < b > c & \"d\"", "日本語のラベル",
		"✅ ready", "⚠\uFE0F careful", "ℹ\uFE0F info", "⬇ down ⬆ up", "Price: €5 + 10% = ~", "x^2 | y_1 `code` $1 #tag @me",
		"", "° © ®",
	} {
		if _, err := CheckLabel(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	if _, err := CheckDescription("line one\nline two 🧪"); err != nil {
		t.Errorf("description with newline: %v", err)
	}
}

func TestNormalises(t *testing.T) {
	got, err := CheckLabel("e\u0301")
	if err != nil || got != "é" {
		t.Fatalf("got %q %v, want NFC é", got, err)
	}
}

func TestRejects(t *testing.T) {
	cases := map[string]string{
		"a\u202Eb":              "text-direction",
		"a\u2067b":              "text-direction",
		"a\u200Bb":              "zero-width",
		"a\u200Db":              "zero-width",
		"a\uFEFFb":              "zero-width",
		"a\x00b":                "control character",
		"a\tb":                  "control character",
		"a\x7fb":                "control character",
		"a\nb":                  "line break",
		"🤖 bot":                 "outside the allowed set",
		"a\uFE0F":               "variation selector",
		"a\u2028b":              "control or format",
		"a\U000E0041b":          "control or format",
		"\uE000":                "not allowed",
		strings.Repeat("a", 81): "limit is 80",
		"\xff":                  "UTF-8",
	}
	for s, want := range cases {
		_, err := CheckLabel(s)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", s, err, want)
		}
	}
	if _, err := CheckDescription(strings.Repeat("a", 501)); err == nil {
		t.Error("501-character description accepted")
	}
	if _, err := CheckDescription(strings.Repeat("a", 500)); err != nil {
		t.Errorf("500-character description: %v", err)
	}
}

func TestErrorPosition(t *testing.T) {
	_, err := CheckLabel("ab\u200Bc")
	if err == nil || err.Error() != "contains a zero-width character at position 3" {
		t.Fatalf("%v", err)
	}
}

func TestEmojiTable(t *testing.T) {
	seen := map[rune]bool{}
	for _, r := range Emoji {
		if seen[r] {
			t.Errorf("duplicate %q", r)
		}
		seen[r] = true
		if _, err := CheckLabel(string(r) + "\uFE0F"); err != nil {
			t.Errorf("%q: %v", r, err)
		}
	}
	if len(Emoji) < 50 || len(Emoji) > 80 {
		t.Errorf("allow-list has %d entries", len(Emoji))
	}
}

// FuzzCheck: accepted text never carries a rune that could change rendering.
func FuzzCheck(f *testing.F) {
	for _, s := range []string{"plain", "<script>alert(1)</script>", "a\u202Eb", "✅\uFE0F", "e\u0301", "\n\n"} {
		f.Add(s, true)
		f.Add(s, false)
	}
	f.Fuzz(func(t *testing.T, s string, desc bool) {
		kind := Label
		if desc {
			kind = Description
		}
		out, err := Check(s, kind)
		if err != nil {
			return
		}
		rs := []rune(out)
		for i, r := range rs {
			if r == presentationSelector && (i == 0 || !emojiSet[rs[i-1]]) {
				t.Fatalf("stray selector accepted in %q", out)
			}
			if r != presentationSelector && r != '\n' && (unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs) || isBidi(r) || isZeroWidth(r)) {
				t.Fatalf("rune %U accepted in %q", r, out)
			}
			if r == '\n' && !desc {
				t.Fatalf("newline accepted in a label")
			}
		}
		if again, err := Check(out, kind); err != nil || again != out {
			t.Fatalf("not idempotent: %q → %q (%v)", out, again, err)
		}
	})
}
