// Package plaintext validates text that a plugin supplies for display: labels,
// descriptions, help, group titles, action results. Accepted text is plain
// Unicode, NFC-normalised, bounded in length, made of letters, digits,
// punctuation, spaces (newline only in descriptions), common ASCII and
// currency/math symbols, and emoji from one fixed allow-list. Everything that
// can change how text renders or reads is rejected: control characters, bidi
// overrides and isolates, zero-width and other format characters, variation
// selectors outside an allowed emoji, private-use and unassigned code points,
// and any other symbol or emoji. "<" and ">" are allowed: the UI only ever
// binds plugin text as text.
package plaintext

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Kind selects the length limit and whether newlines are allowed.
type Kind int

const (
	// Label is a short one-line text: field labels, option labels, names.
	Label Kind = iota
	// Description is a longer text that may contain newlines.
	Description
)

// Limits in runes after normalisation.
const (
	MaxLabel       = 80
	MaxDescription = 500
)

// Emoji is the one allow-list of emoji a plugin may use. Each entry is a single
// code point; it may be followed by the emoji presentation selector U+FE0F.
var Emoji = []rune{
	'✅', '❌', '⚠', 'ℹ', '🔒', '🔓', '🔑', '⏱', '⏳', '📁',
	'📂', '🎬', '📺', '🎵', '🎶', '📚', '📖', '⬇', '⬆', '⚡',
	'🐢', '💡', '🧪', '⭐', '✨', '🔔', '🔕', '🔍', '🔧', '⚙',
	'🛠', '📡', '🌐', '💾', '🗑', '📝', '📌', '➕', '➖', '✔',
	'✖', '❗', '❓', '🚫', '⛔', '🟢', '🟡', '🔴', '🔵', '⏸',
	'▶', '⏹', '🔄', '🔁', '📥', '📤', '🕒', '📅', '🎧', '🖥',
	'📱', '🧲', '🏷', '👍', '👎', '🙂',
}

const presentationSelector = '\uFE0F'

var emojiSet = func() map[rune]bool {
	m := make(map[rune]bool, len(Emoji))
	for _, r := range Emoji {
		m[r] = true
	}
	return m
}()

// Error describes the first offending character.
type Error struct {
	Pos    int // rune index in the normalised text
	Reason string
}

func (e *Error) Error() string {
	if e.Pos < 0 {
		return e.Reason
	}
	return fmt.Sprintf("%s at position %d", e.Reason, e.Pos+1)
}

// Check validates s for kind and returns it NFC-normalised.
func Check(s string, kind Kind) (string, error) {
	if !utf8.ValidString(s) {
		return "", &Error{Pos: -1, Reason: "is not valid UTF-8"}
	}
	s = norm.NFC.String(s)
	limit := MaxLabel
	if kind == Description {
		limit = MaxDescription
	}
	if n := utf8.RuneCountInString(s); n > limit {
		return "", &Error{Pos: -1, Reason: fmt.Sprintf("is %d characters long; the limit is %d", n, limit)}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		skip, reason := classify(rs, i, kind)
		if reason != "" {
			return "", &Error{Pos: i, Reason: reason}
		}
		i += skip
	}
	return s, nil
}

// CheckLabel is Check(s, Label).
func CheckLabel(s string) (string, error) { return Check(s, Label) }

// CheckDescription is Check(s, Description).
func CheckDescription(s string) (string, error) { return Check(s, Description) }

// classify judges rs[i]; skip is the number of following runes it consumed.
func classify(rs []rune, i int, kind Kind) (skip int, reason string) {
	r := rs[i]
	if emojiSet[r] {
		if i+1 < len(rs) && rs[i+1] == presentationSelector {
			return 1, ""
		}
		return 0, ""
	}
	if r == '\n' {
		if kind == Description {
			return 0, ""
		}
		return 0, "contains a line break"
	}
	return 0, runeReason(r)
}

// runeReason is "" for an allowed rune outside the emoji table.
func runeReason(r rune) string {
	switch {
	case r < 0x80:
		return asciiReason(r)
	case isBidi(r):
		return "contains a text-direction control character"
	case isZeroWidth(r):
		return "contains a zero-width character"
	case unicode.Is(unicode.Variation_Selector, r):
		return "contains a variation selector outside an allowed emoji"
	}
	return categoryReason(r)
}

// extraSymbols are allowed outside the letter, number and punctuation classes.
var extraSymbols = map[rune]bool{'\u00A0': true, '\u00B0': true, '\u00A9': true, '\u00AE': true}

func categoryReason(r rune) string {
	switch {
	case extraSymbols[r] || unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.Sc, unicode.Sm, unicode.Sk):
		return ""
	case unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp):
		return "contains a control or format character"
	case unicode.Is(unicode.So, r):
		return "contains a symbol or emoji outside the allowed set"
	}
	return "contains a character that is not allowed"
}

func asciiReason(r rune) string {
	if r < 0x20 || r == 0x7F {
		return "contains a control character"
	}
	return ""
}

// isBidi reports bidi embeddings, overrides, isolates and marks.
func isBidi(r rune) bool {
	return (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069') ||
		r == '\u200E' || r == '\u200F' || r == '\u061C'
}

// isZeroWidth reports zero-width spaces, joiners and the BOM.
func isZeroWidth(r rune) bool {
	return (r >= '\u200B' && r <= '\u200D') || r == '\u2060' || r == '\uFEFF' || r == '\u180E'
}
