package music

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/jrand0m/corsarr/internal/domain"
)

// VariousArtistsMBID is the MusicBrainz artist that credits compilations
// made of many artists.
const VariousArtistsMBID = "89ad4ac3-39f7-470e-963a-56509c546377"

// variousQuery is how indexers name a various-artists compilation.
const variousQuery = "VA"

// IsVarious reports whether an artist stands for many artists.
func IsVarious(ar domain.Artist) bool {
	return ar.MBID == VariousArtistsMBID || strings.EqualFold(strings.TrimSpace(ar.Name), "Various Artists")
}

// letterFolds spell letters that have no decomposed form in plain Latin.
var letterFolds = strings.NewReplacer("ø", "o", "Ø", "O", "æ", "ae", "Æ", "Ae", "œ", "oe", "Œ", "Oe",
	"ß", "ss", "ł", "l", "Ł", "L", "đ", "d", "Đ", "D", "þ", "th", "Þ", "Th", "ı", "i")

// fold removes accents and diacritics: "Björk" becomes "Bjork".
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(letterFolds.Replace(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// apostrophes join the word they sit in ("Don't" becomes "Dont").
var apostrophes = strings.NewReplacer("'", "", "’", "", "‘", "", "`", "", "´", "", "ʼ", "")

// words folds a name and splits it into its runs of letters and digits.
func words(s string) []string {
	return strings.FieldsFunc(apostrophes.Replace(fold(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// withoutArticle drops a leading "The" when other words follow it.
func withoutArticle(ws []string) []string {
	if len(ws) > 1 && strings.EqualFold(ws[0], "the") {
		return ws[1:]
	}
	return ws
}

// QueryText cleans a name for indexer search engines: accents folded,
// apostrophes joined, other punctuation turned into spaces and a leading
// "The" dropped.
func QueryText(s string) string {
	return strings.Join(withoutArticle(words(s)), " ")
}

// queryArtist is the artist as searched: "VA" for a compilation.
func queryArtist(ar domain.Artist) string {
	if IsVarious(ar) {
		return variousQuery
	}
	return QueryText(ar.Name)
}

// matchKey is a name reduced for comparison: folded, lower case, "&" read
// as "and", no leading "The" and only letters and digits.
func matchKey(s string) string {
	return strings.ToLower(strings.Join(withoutArticle(words(strings.ReplaceAll(s, "&", " and "))), ""))
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// nameSimilarity is how close two match keys must be to name the same thing.
const nameSimilarity = 0.85

// SameName reports whether two names name the same artist or album: equal
// once reduced, or nearly so with the same numbers in both (so volume and
// year differences still tell names apart).
func SameName(a, b string) bool {
	ka, kb := matchKey(a), matchKey(b)
	if ka == "" || kb == "" {
		return false
	}
	return ka == kb || (digitsOf(ka) == digitsOf(kb) && similarity(ka, kb) >= nameSimilarity)
}

// similarity is one minus the edit distance over the longer length.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	longest := max(len(ra), len(rb))
	if longest == 0 {
		return 1
	}
	return 1 - float64(editDistance(ra, rb))/float64(longest)
}

// editDistance counts the single-rune insertions, deletions and
// substitutions that turn a into b.
func editDistance(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ra := range a {
		cur := make([]int, len(b)+1)
		cur[0] = i + 1
		for j, rb := range b {
			cur[j+1] = min(prev[j+1]+1, cur[j]+1, prev[j]+substitution(ra, rb))
		}
		prev = cur
	}
	return prev[len(b)]
}

func substitution(a, b rune) int {
	if a == b {
		return 0
	}
	return 1
}
