package wasm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// hostRules are a module's declared HTTP hosts.
type hostRules struct {
	exact    map[string]bool
	suffixes []string
	settings []string // setting keys whose host is allowed
}

func newHostRules(patterns []string) hostRules {
	r := hostRules{exact: map[string]bool{}}
	for _, p := range patterns {
		if key, ok := strings.CutPrefix(p, abi.SettingHostPrefix); ok {
			r.settings = append(r.settings, key)
			continue
		}
		p = strings.ToLower(p)
		if rest, ok := strings.CutPrefix(p, "*"); ok && strings.HasPrefix(rest, ".") {
			r.suffixes = append(r.suffixes, rest)
		} else {
			r.exact[p] = true
		}
	}
	return r
}

// configuredHosts are the hosts named by the instance's bound settings.
func (r hostRules) configuredHosts(settings map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, k := range r.settings {
		if h := settingHost(settings[k]); h != "" {
			out[h] = true
		}
	}
	return out
}

func settingHost(v any) string {
	s, _ := v.(string)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return strings.ToLower(u.Hostname())
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(strings.Trim(s, "[]"))
}

// verdict decides whether host may be contacted; configured hosts may be on
// a private network, declared public hosts may not.
func (r hostRules) verdict(host string, configured map[string]bool) (allowed, private bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if configured[host] {
		return true, true
	}
	if r.exact[host] {
		return true, false
	}
	for _, s := range r.suffixes {
		if strings.HasSuffix(host, s) {
			return true, false
		}
	}
	return false, false
}

// restricted reports addresses a module may not reach unless an instance
// setting names them.
func restricted(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsUnspecified() || a.IsMulticast() || a.IsInterfaceLocalMulticast() || cgnat.Contains(a)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func systemResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// ── dial-time check ──────────────────────────────────────────────────────────

type guardKey struct{}

// dialGuard rides on a module's request context: the guarded transport checks
// every address it is about to dial.
type dialGuard struct{ allowPrivate bool }

// GuardedTransport is the shared outbound transport with a dial-time check:
// for requests made by a plugin module, every resolved address is checked
// again right before connecting, so a name that resolves to a private
// address after the first check is still refused. Other requests dial as
// usual.
func GuardedTransport() *http.Transport {
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 16, MaxIdleConnsPerHost: 4,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ForceAttemptHTTP2: true,
		DialContext: guardedDial(d.DialContext, systemResolve)}
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

func guardedDial(dial dialFunc, resolve func(context.Context, string) ([]netip.Addr, error)) dialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		g, ok := ctx.Value(guardKey{}).(dialGuard)
		if !ok || g.allowPrivate {
			return dial(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !restricted(ip) {
				return dial(ctx, network, net.JoinHostPort(ip.String(), port))
			}
		}
		return nil, deny("%s resolves only to private or local addresses", host)
	}
}

// ── http_do ──────────────────────────────────────────────────────────────────

func hostHTTP(ctx context.Context, st *callState, req []byte) (any, error) {
	var r abi.HTTPRequest
	if err := decodeReq(req, &r); err != nil {
		return nil, err
	}
	if st.pool == nil || st.pool.b.HTTP == nil {
		return nil, deny("this instance has no network access")
	}
	u, guard, err := st.pool.check(ctx, r)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(context.WithValue(ctx, guardKey{}, guard), r.Method, u.String(), bytes.NewReader(r.Body))
	if err != nil {
		return nil, deny("bad request: %v", err)
	}
	for k, vs := range r.Headers {
		for _, v := range vs {
			hreq.Header.Add(k, v)
		}
	}
	resp, err := st.pool.b.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readResponse(resp, st.pool.m.Limits.MaxMessageBytes)
}

var methods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

// check enforces the module's declared hosts and the private-network rule.
func (p *Pool) check(ctx context.Context, r abi.HTTPRequest) (*url.URL, dialGuard, error) {
	u, err := parseTarget(r)
	if err != nil {
		return nil, dialGuard{}, err
	}
	allowed, private := p.m.hosts.verdict(u.Hostname(), p.m.hosts.configuredHosts(p.b.Settings))
	if !allowed {
		return nil, dialGuard{}, deny("host %s is not declared in permissions.http.hosts", u.Hostname())
	}
	if !private {
		if err := p.publicHost(ctx, u.Hostname()); err != nil {
			return nil, dialGuard{}, err
		}
	}
	return u, dialGuard{allowPrivate: private}, nil
}

func parseTarget(r abi.HTTPRequest) (*url.URL, error) {
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, deny("only http:// and https:// URLs without credentials are allowed")
	}
	if !methods[r.Method] {
		return nil, deny("method %q is not allowed", r.Method)
	}
	return u, nil
}

func (p *Pool) publicHost(ctx context.Context, host string) error {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if restricted(ip) {
			return deny("%s is a private or local address; only a host set in the instance settings may be", host)
		}
		return nil
	}
	ips, err := p.m.eng.o.Resolve(ctx, host)
	if err != nil {
		return plugin.Wrap(plugin.ClassUnreachable, "resolve "+host, err)
	}
	for _, ip := range ips {
		if restricted(ip) {
			return deny("%s resolves to a private or local address; only a host set in the instance settings may", host)
		}
	}
	return nil
}

func readResponse(resp *http.Response, limit int) (abi.HTTPResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return abi.HTTPResponse{}, plugin.Wrap(plugin.ClassUnreachable, "read response", err)
	}
	if len(body) > limit {
		return abi.HTTPResponse{}, plugin.Errorf(plugin.ClassParse, "response is larger than %d bytes", limit)
	}
	return abi.HTTPResponse{Status: resp.StatusCode, Headers: resp.Header, Body: body}, nil
}

// classOfHTTP maps transport failures to ABI classes for the module.
func classOfHTTP(err error) string {
	if c := plugin.ClassOf(err); c != "" {
		return string(c)
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
		return string(plugin.ClassifyTransport(err))
	}
	return ""
}
