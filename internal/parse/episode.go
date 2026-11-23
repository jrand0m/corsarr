package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Every episode pattern has the whole marker as submatch 1 (its start ends
// the title) and the numbers as later submatches.
var (
	sxeRe         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(s(\d{1,2})[ ._-]?e(\d{1,3}))`)
	nxnRe         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])((\d{1,2})x(\d{2,3}))`)
	moreEpRe      = regexp.MustCompile(`(?i)^(?:[ ._]?e|-[ ]?e?|x)(\d{1,3})`)
	seasonRangeRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(s(\d{1,2})[ ._]?-[ ._]?s?(\d{1,2}))(?:$|[^a-z0-9])`)
	seasonWordRe  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(season[ ._-]?(\d{1,2}))(?:$|[^0-9])`)
	seasonPackRe  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(s(\d{1,2}))(?:$|[^a-z0-9])`)
	dailyRe       = regexp.MustCompile(`(?:^|[^0-9])(((?:19|20)\d{2})[ ._-](0[1-9]|1[0-2])[ ._-](0[1-9]|[12]\d|3[01]))(?:$|[^0-9])`)
)

type episodeFinder func(s string) (Episode, int, bool)

var episodeFinders = []episodeFinder{
	findNumbered(sxeRe),
	findNumbered(nxnRe),
	findSeasonRange,
	findSeason(seasonWordRe),
	findSeason(seasonPackRe),
	findDaily,
}

// findEpisode returns the TV address and the index where it starts (-1 if none).
func findEpisode(s string) (Episode, int) {
	for _, f := range episodeFinders {
		if ep, idx, ok := f(s); ok {
			return ep, idx
		}
	}
	return Episode{}, -1
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func findNumbered(re *regexp.Regexp) episodeFinder {
	return func(s string) (Episode, int, bool) {
		m := re.FindStringSubmatchIndex(s)
		if m == nil || isDigitAt(s, m[1]) {
			return Episode{}, -1, false
		}
		eps := moreEpisodes(s, m[1], []int{atoi(s[m[6]:m[7]])})
		return Episode{Kind: EpisodeStandard, Seasons: []int{atoi(s[m[4]:m[5]])}, Episodes: eps}, m[2], true
	}
}

// moreEpisodes reads the multi-episode tail after the first episode:
// E01E02 (list), E01-E03 / E01-03 (range), 1x01x02.
func moreEpisodes(s string, pos int, eps []int) []int {
	for {
		m := moreEpRe.FindStringSubmatchIndex(s[pos:])
		if m == nil || isDigitAt(s, pos+m[1]) || isResolutionSuffix(s, pos+m[1]) {
			return eps
		}
		n := atoi(s[pos+m[2] : pos+m[3]])
		eps = addEpisode(eps, n, strings.HasPrefix(s[pos:], "-"))
		pos += m[1]
	}
}

func addEpisode(eps []int, n int, isRange bool) []int {
	last := eps[len(eps)-1]
	if !isRange || n <= last {
		if n > last {
			eps = append(eps, n)
		}
		return eps
	}
	for e := last + 1; e <= n; e++ {
		eps = append(eps, e)
	}
	return eps
}

func isDigitAt(s string, i int) bool { return i < len(s) && s[i] >= '0' && s[i] <= '9' }

func isResolutionSuffix(s string, i int) bool {
	return i < len(s) && strings.ContainsRune("pPiI", rune(s[i]))
}

func findSeasonRange(s string) (Episode, int, bool) {
	m := seasonRangeRe.FindStringSubmatchIndex(s)
	if m == nil {
		return Episode{}, -1, false
	}
	from, to := atoi(s[m[4]:m[5]]), atoi(s[m[6]:m[7]])
	if to < from {
		return Episode{}, -1, false
	}
	var seasons []int
	for n := from; n <= to; n++ {
		seasons = append(seasons, n)
	}
	return Episode{Kind: EpisodeSeason, Seasons: seasons}, m[2], true
}

func findSeason(re *regexp.Regexp) episodeFinder {
	return func(s string) (Episode, int, bool) {
		m := re.FindStringSubmatchIndex(s)
		if m == nil {
			return Episode{}, -1, false
		}
		return Episode{Kind: EpisodeSeason, Seasons: []int{atoi(s[m[4]:m[5]])}}, m[2], true
	}
}

func findDaily(s string) (Episode, int, bool) {
	m := dailyRe.FindStringSubmatchIndex(s)
	if m == nil {
		return Episode{}, -1, false
	}
	date := fmt.Sprintf("%s-%s-%s", s[m[4]:m[5]], s[m[6]:m[7]], s[m[8]:m[9]])
	return Episode{Kind: EpisodeDaily, AirDate: date}, m[2], true
}
