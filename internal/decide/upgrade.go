package decide

import (
	"fmt"
	"strings"
)

// Candidate is a release considered as a replacement for the current file.
type Candidate struct {
	Quality     QualityID
	FormatScore int
	Group       string
	Revision    int
	Proper      bool // PROPER may come from any group; REPACK/version only from the same group
}

// IsUpgrade decides whether a release upgrades the current file: true only when upgrades are on
// and the candidate is a qualifying proper/repack of the same quality, or the
// current file has not met both cutoffs and the candidate is better by
// quality rank or by at least the minimum score increment. The string says why not.
func IsUpgrade(p QualityProfile, cur CurrentFile, cand Candidate) (bool, string) {
	s := p.Spec()
	candRank, ok := p.Rank(cand.Quality)
	if !ok {
		return false, fmt.Sprintf("%s is not allowed", cand.Quality)
	}
	if !s.Upgrades {
		return false, "upgrades are off in this profile"
	}
	curRank := currentRank(p, cur)
	if candRank == curRank && isQualifyingRevision(cur, cand) {
		return true, ""
	}
	if cutoffsMet(p, curRank, cur.FormatScore) {
		return false, "current file already meets the quality and score cutoffs"
	}
	if candRank > curRank || (candRank == curRank && cand.FormatScore-cur.FormatScore >= s.MinUpgradeIncrement) {
		return true, ""
	}
	return false, fmt.Sprintf("%s score %d is not better than current %s score %d",
		cand.Quality, cand.FormatScore, cur.Quality, cur.FormatScore)
}

// currentRank is -1 for a file outside the profile: any allowed quality beats it.
func currentRank(p QualityProfile, cur CurrentFile) int {
	if r, ok := p.Rank(cur.Quality); ok {
		return r
	}
	return -1
}

func cutoffsMet(p QualityProfile, curRank, curScore int) bool {
	cutRank, _ := p.Rank(p.spec.Cutoff)
	return curRank >= cutRank && curScore >= p.spec.CutoffFormatScore
}

func isQualifyingRevision(cur CurrentFile, cand Candidate) bool {
	if cand.Revision <= max(cur.Revision, 1) {
		return false
	}
	return cand.Proper || strings.EqualFold(cand.Group, cur.Group)
}
