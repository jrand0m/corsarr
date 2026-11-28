package catalogue

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// State is what an index entry means for this installation.
type State string

const (
	StateAvailable     State = "available"             // not installed, can be installed
	StateInstalled     State = "installed"             // this version is installed
	StateUpdate        State = "update"                // a newer version, same or fewer permissions
	StateNeedsApproval State = "update_needs_approval" // a newer version that asks for more
	StateOlder         State = "older"                 // older than the installed version
	StateIncompatible  State = "incompatible"          // this corsarr cannot run it
)

// Diff is how permissions change from one version to the next.
type Diff struct {
	AddedHosts     []string `json:"addedHosts"`
	RemovedHosts   []string `json:"removedHosts"`
	AddedSecrets   []string `json:"addedSecrets"`
	RemovedSecrets []string `json:"removedSecrets"`
}

// Grew reports whether the next version asks for anything new.
func (d Diff) Grew() bool { return len(d.AddedHosts) > 0 || len(d.AddedSecrets) > 0 }

// PermissionDiff compares two permission sets (pure).
func PermissionDiff(old, next abi.Permissions) Diff {
	d := Diff{AddedHosts: []string{}, RemovedHosts: []string{}, AddedSecrets: []string{}, RemovedSecrets: []string{}}
	d.AddedHosts, d.RemovedHosts = setDiff(old.HTTP.Hosts, next.HTTP.Hosts)
	d.AddedSecrets, d.RemovedSecrets = setDiff(old.Secrets, next.Secrets)
	return d
}

func setDiff(old, next []string) (added, removed []string) {
	added, removed = []string{}, []string{}
	for _, s := range next {
		if !slices.Contains(old, s) {
			added = append(added, s)
		}
	}
	for _, s := range old {
		if !slices.Contains(next, s) {
			removed = append(removed, s)
		}
	}
	return added, removed
}

// version is a parsed semantic version (build metadata dropped).
type version struct {
	n   [3]int
	pre string
}

func parseVersion(s string) (version, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v version
	core, pre, _ := strings.Cut(s, "-")
	v.pre = pre
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v.n[i] = n
	}
	return v, true
}

// CompareVersions orders two semantic versions (an optional leading v is
// ignored); a pre-release sorts before its release. Unparsable versions
// compare as equal.
func CompareVersions(a, b string) int {
	va, okA := parseVersion(a)
	vb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	for i := range va.n {
		if c := va.n[i] - vb.n[i]; c != 0 {
			return sign(c)
		}
	}
	switch {
	case va.pre == vb.pre:
		return 0
	case va.pre == "":
		return 1
	case vb.pre == "":
		return -1
	}
	return strings.Compare(va.pre, vb.pre)
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// Incompatible says why this corsarr cannot run e ("" when it can). A build
// without a release version (a development build) skips the version check.
func Incompatible(e Entry, corsarr string) string {
	if e.ABIVersion != abi.Version {
		return fmt.Sprintf("speaks plugin ABI v%d; this corsarr runs v%d", e.ABIVersion, abi.Version)
	}
	if _, ok := wasm.Spec(e.Kind); !ok {
		return fmt.Sprintf("is a %s plugin, which this corsarr cannot load from a module", e.Kind)
	}
	if _, released := parseVersion(corsarr); released && e.MinCorsarr != "" && CompareVersions(corsarr, e.MinCorsarr) < 0 {
		return "needs corsarr ≥ " + e.MinCorsarr
	}
	return ""
}

// Update policies for plugins.autoUpdate.
const (
	AutoOff   = "off"
	AutoPatch = "patch"
	AutoMinor = "minor"
)

// WithinPolicy reports whether moving from installed to next is an
// automatic update under policy (pure).
func WithinPolicy(policy, installed, next string) bool {
	// An unparsable version compares as equal, so it is never an update.
	a, _ := parseVersion(installed)
	b, _ := parseVersion(next)
	if CompareVersions(next, installed) <= 0 || b.pre != "" {
		return false
	}
	switch policy {
	case AutoPatch:
		return a.n[0] == b.n[0] && a.n[1] == b.n[1]
	case AutoMinor:
		return a.n[0] == b.n[0]
	}
	return false
}

// Installed is what the offer needs to know about an installed module.
type Installed struct {
	Version     string
	Permissions abi.Permissions
	Present     bool // false for a module that was uninstalled (its record is kept)
}

// Offer is one index entry as the catalogue view shows it.
type Offer struct {
	Entry     Entry  `json:"entry"`
	State     State  `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Installed string `json:"installed,omitempty"`
	Diff      *Diff  `json:"diff,omitempty"`
}

// OfferFor decides the state of one entry (pure).
func OfferFor(e Entry, inst Installed, corsarr string) Offer {
	o := Offer{Entry: e, State: StateAvailable}
	if inst.Present {
		o.Installed = inst.Version
		d := PermissionDiff(inst.Permissions, e.Permissions)
		o.Diff = &d
	}
	if reason := Incompatible(e, corsarr); reason != "" {
		o.State, o.Reason = StateIncompatible, reason
		return o
	}
	if !inst.Present {
		return o
	}
	switch c := CompareVersions(e.Version, inst.Version); {
	case c == 0:
		o.State = StateInstalled
	case c < 0:
		o.State = StateOlder
	case o.Diff.Grew():
		o.State = StateNeedsApproval
	default:
		o.State = StateUpdate
	}
	return o
}

// Offers decides the state of every entry of ix.
func Offers(ix Index, installed map[string]Installed, corsarr string) []Offer {
	out := make([]Offer, 0, len(ix.Plugins))
	for _, e := range ix.Plugins {
		out = append(out, OfferFor(e, installed[e.ID], corsarr))
	}
	return out
}

// DefaultURL is the index of the release a build comes from; a build without
// a release version reads the latest release's index.
func DefaultURL(buildVersion string) string {
	const repo = "https://github.com/jrand0m/corsarr/releases/"
	if releaseRe.MatchString(buildVersion) {
		return repo + "download/" + buildVersion + "/plugins-index.json"
	}
	return repo + "latest/download/plugins-index.json"
}
