package httpx

import (
	"fmt"
	"net/netip"
	"strings"
)

// egress is the outbound allow list: exact host names, "*.domain" suffixes
// (any subdomain, not the domain itself) and CIDR ranges for IP literals.
// An empty list allows every host.
type egress struct {
	hosts    map[string]bool
	suffixes []string
	nets     []netip.Prefix
}

func newEgress(entries []string) *egress {
	if len(entries) == 0 {
		return nil
	}
	e := &egress{hosts: map[string]bool{}}
	for _, s := range entries {
		s = strings.ToLower(strings.TrimSpace(s))
		if p, err := netip.ParsePrefix(s); err == nil {
			e.nets = append(e.nets, p.Masked())
		} else if rest, ok := strings.CutPrefix(s, "*."); ok {
			e.suffixes = append(e.suffixes, "."+rest)
		} else {
			e.hosts[strings.Trim(s, "[]")] = true
		}
	}
	return e
}

// allows reports whether host (a URL hostname, no port) may be contacted.
func (e *egress) allows(host string) bool {
	if e == nil {
		return true
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if e.hosts[host] {
		return true
	}
	for _, s := range e.suffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range e.nets {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ErrEgressBlocked is returned for a request to a host outside the allow list.
type ErrEgressBlocked struct{ Host string }

func (e ErrEgressBlocked) Error() string {
	return fmt.Sprintf("outbound request to %s blocked: the host is not in server.egressAllow", e.Host)
}
