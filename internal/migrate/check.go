package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/rootfolders"
)

// CheckState is the outcome of a quick source check.
type CheckState string

const (
	CheckReachable    CheckState = "reachable"    // the app answered, identity and version are fine
	CheckKeyNeeded    CheckState = "keyNeeded"    // an app answers but asks for a key (address-only check)
	CheckUnauthorised CheckState = "unauthorised" // the key was refused
	CheckWrongApp     CheckState = "wrongApp"     // something answers, but not the chosen app
	CheckUnsupported  CheckState = "unsupported"  // the version is outside the supported range
	CheckUnreachable  CheckState = "unreachable"  // no usable answer
)

// CheckResult is what a source check reports; it never carries the key.
type CheckResult struct {
	App       App        `json:"app"`
	State     CheckState `json:"state"`
	Name      string     `json:"name,omitempty"`
	Version   string     `json:"version,omitempty"`
	Supported string     `json:"supported"`
	Reason    string     `json:"reason,omitempty"`
}

// checkTimeout bounds one check, retries included.
const checkTimeout = 10 * time.Second

// parseAddress validates a source base address.
func parseAddress(a App, rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%s address %q must be an http or https URL", a, rawURL)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return u, nil
}

// ParseTarget validates a source for a check; the key may be empty.
func ParseTarget(app, rawURL string, key Secret) (Endpoint, error) {
	a, err := ParseApp(app)
	if err != nil {
		return Endpoint{}, err
	}
	u, err := parseAddress(a, rawURL)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{App: a, URL: u, Key: key}, nil
}

// Check reads the source's status once, read-only. Without a key it checks
// only that the app's API answers at the address.
func (s *Service) Check(ctx context.Context, ep Endpoint) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	st, err := NewClient(ep, s.d.HTTP).Status(ctx)
	return classify(ep, st, err)
}

// classify turns a status answer into a check result.
func classify(ep Endpoint, st wStatus, err error) CheckResult {
	r := CheckResult{App: ep.App, Supported: SupportedRange(ep.App), Name: st.AppName, Version: st.Version}
	if err == nil || errors.Is(err, ErrUnsupportedVersion) {
		r.State, r.Reason = identify(ep.App, st, err)
	} else {
		r.State, r.Reason = failure(ep, err)
	}
	return r
}

// failure explains a status request that got no usable answer.
func failure(ep Endpoint, err error) (CheckState, string) {
	label := appLabel(ep.App)
	var se *StatusError
	switch {
	case errors.Is(err, ErrUnauthorised) && ep.Key.Empty():
		return CheckKeyNeeded, "An app answers at this address. Enter its API key to confirm it is " + label + " and check its version."
	case errors.Is(err, ErrUnauthorised):
		return CheckUnauthorised, "The app refused this API key."
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		return CheckWrongApp, "Something answers at this address, but it has no " + label + " API."
	case errors.As(err, &se):
		return CheckUnreachable, "The address answered " + se.Status + "."
	case errors.Is(err, ErrUnreadable):
		return CheckWrongApp, "Something answers at this address, but not like " + label + "."
	}
	return CheckUnreachable, netReason(err)
}

// identify checks the app name and version of a status answer.
func identify(a App, st wStatus, verr error) (CheckState, string) {
	if st.AppName != "" && !strings.EqualFold(st.AppName, string(a)) {
		return CheckWrongApp, "This address is " + st.AppName + ", not " + appLabel(a) + "."
	}
	if verr != nil {
		return CheckUnsupported, fmt.Sprintf("Version %s is outside the supported range %s.", st.Version, SupportedRange(a))
	}
	return CheckReachable, ""
}

// netReason explains a transport failure without the request URL.
func netReason(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var dns *net.DNSError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("No answer within %d s.", int(checkTimeout/time.Second))
	case errors.As(err, &dns):
		return "The host name " + dns.Name + " does not resolve."
	}
	return err.Error()
}

func appLabel(a App) string {
	if a == "" {
		return ""
	}
	return strings.ToUpper(string(a[:1])) + string(a[1:])
}

// Access is what corsarr may do with a local folder.
type Access struct {
	Exists   bool `json:"exists"`
	Readable bool `json:"readable"`
	Writable bool `json:"writable"`
}

// probeDir reads a folder's access for this process.
func probeDir(p string) Access {
	pr := rootfolders.ProbePath(p)
	if !pr.Dir {
		return Access{}
	}
	a := Access{Exists: true, Writable: pr.Writable}
	if f, err := os.Open(p); err == nil {
		_, err = f.Readdirnames(1)
		a.Readable = err == nil || errors.Is(err, io.EOF)
		_ = f.Close()
	}
	return a
}

// RootCheck is one source root folder: whether the source prefix matches it,
// the path it becomes on corsarr's side and whether that exists there.
type RootCheck struct {
	Source    App    `json:"source"`
	Path      string `json:"path"`
	Matched   bool   `json:"matched"`
	Rewritten string `json:"rewritten"`
	Access
}

// PrefixCheck is the corsarr prefix's local access.
type PrefixCheck struct {
	Path string `json:"path"`
	Access
}

// SourceIssue is a source whose root folders could not be read.
type SourceIssue struct {
	App    App    `json:"app"`
	Reason string `json:"reason"`
}

// PathCheck is the outcome of a path rewrite check.
type PathCheck struct {
	Roots   []RootCheck   `json:"roots"`
	Target  *PrefixCheck  `json:"target,omitempty"`
	Sources []SourceIssue `json:"sources"`
}

// maxRoots bounds the sample of root folders a path check reports.
const maxRoots = 50

// ParsePrefix validates one side of a path rewrite ("" = none).
func ParsePrefix(name, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%s %q must be an absolute path", name, p)
	}
	return cleanPath(p), nil
}

// CheckPaths reads each source's root folders (read-only) and reports how a
// rewrite from one prefix to the other maps them; either prefix may be empty.
func (s *Service) CheckPaths(ctx context.Context, sources []Endpoint, rw Rewrite) PathCheck {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	probe := s.d.Probe
	if probe == nil {
		probe = probeDir
	}
	out := PathCheck{Roots: []RootCheck{}, Sources: []SourceIssue{}}
	if rw.To != "" {
		out.Target = &PrefixCheck{Path: rw.To, Access: probe(rw.To)}
	}
	for _, ep := range sources {
		if ep.App.Kind() == "" {
			continue
		}
		roots, err := readRootFolders(ctx, NewClient(ep, s.d.HTTP))
		if err != nil {
			out.Sources = append(out.Sources, SourceIssue{App: ep.App, Reason: classify(ep, wStatus{}, err).Reason})
			continue
		}
		for _, p := range roots {
			if len(out.Roots) >= maxRoots {
				break
			}
			out.Roots = append(out.Roots, rootCheck(ep.App, p, rw, probe))
		}
	}
	return out
}

func rootCheck(a App, p string, rw Rewrite, probe func(string) Access) RootCheck {
	matched := rw.From != "" && (p == rw.From || strings.HasPrefix(p, rw.From+"/"))
	to := p
	if matched && rw.To != "" {
		to = rw.Apply(p)
	}
	return RootCheck{Source: a, Path: p, Matched: matched, Rewritten: to, Access: probe(to)}
}

func readRootFolders(ctx context.Context, c *Client) ([]string, error) {
	var list []wRootFolder
	if err := c.get(ctx, "/rootfolder", nil, &list); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, r := range list {
		out = append(out, cleanPath(r.Path))
	}
	return out, nil
}

// gate rate-limits the quick checks: a token bucket plus a cap on checks in
// flight, so typing in a form cannot flood a source or corsarr.
type gate struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
	busy   int
}

const (
	gateBurst    = 10
	gatePerSec   = 2.0
	gateInFlight = 4
)

// enter takes a token and a slot, or reports that the caller must wait.
func (g *gate) enter(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.last.IsZero() {
		g.tokens = gateBurst
	} else {
		g.tokens = min(gateBurst, g.tokens+now.Sub(g.last).Seconds()*gatePerSec)
	}
	g.last = now
	if g.tokens < 1 || g.busy >= gateInFlight {
		return false
	}
	g.tokens--
	g.busy++
	return true
}

func (g *gate) leave() {
	g.mu.Lock()
	g.busy--
	g.mu.Unlock()
}
