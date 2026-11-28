package queue

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// URLRequest adds a download by hand from a magnet link or a .torrent or
// .nzb URL, linked to the items it is for.
type URLRequest struct {
	Kind  domain.MediaKind
	Items []domain.EventItem
	URL   string
	// Title names the download; the URL's name when empty.
	Title string
	// Protocol is torrent or usenet; guessed from the URL when empty.
	Protocol domain.Protocol
	// Downloader is the client to use; the first that takes the protocol
	// when empty.
	Downloader domain.ObjectKey
}

// ErrBadURL: the URL is not a magnet link or an http(s) torrent or nzb URL.
var ErrBadURL = errors.New("give a magnet link or an http(s) URL of a .torrent or .nzb file")

// AddURL sends a URL to a download client and tracks it like a grab, so it
// is polled and imported for its items. A shadow kind records the add as
// downloader.shadowed instead and a disabled one refuses.
func (s *Service) AddURL(ctx context.Context, req URLRequest) (domain.Grab, error) {
	rel, payload, err := urlRelease(req)
	if err != nil {
		return domain.Grab{}, err
	}
	switch mode := s.d.Modes.Of(req.Kind); mode {
	case domain.ModeShadow:
		s.publish(domain.EvClientShadowed, "user", req.Items, domain.ClientActionData{Downloader: req.Downloader,
			Title: rel.Title, Action: "add_url", Args: map[string]any{"protocol": rel.Protocol}})
		return domain.Grab{}, ErrShadowed
	case domain.ModeDisabled:
		return domain.Grab{}, domain.ModeError{Kind: req.Kind, Mode: mode}
	}
	return s.Grab(ctx, GrabRequest{Kind: req.Kind, Items: req.Items, Release: rel, Origin: domain.OriginInteractive,
		Decision: domain.Decision{ReleaseID: rel.ID, Accepted: true, Reasons: []domain.Reason{}}, Override: true,
		Downloader: req.Downloader, payload: &payload})
}

// urlRelease describes the URL as a release and the payload to add. The
// release id hashes the URL, so the same URL is the same release.
func urlRelease(req URLRequest) (domain.Release, plugin.Payload, error) {
	raw := strings.TrimSpace(req.URL)
	kind, proto, name, err := urlKind(raw, req.Protocol)
	if err != nil {
		return domain.Release{}, plugin.Payload{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = name
	}
	sum := sha1.Sum([]byte(raw))
	id := "url-" + hex.EncodeToString(sum[:8])
	rel := domain.Release{ID: id, GUID: id, Title: title, Protocol: proto, DownloadURL: raw}
	return rel, plugin.Payload{Kind: kind, URL: raw, Name: title}, nil
}

// urlKind tells the payload kind, protocol and a display name from the URL.
func urlKind(raw string, proto domain.Protocol) (string, domain.Protocol, string, error) {
	if strings.HasPrefix(raw, "magnet:") {
		return "magnet", domain.ProtoTorrent, magnetName(raw), nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", ErrBadURL
	}
	name, ext := path.Base(u.Path), strings.ToLower(path.Ext(u.Path))
	if p, ok := extProtocols[ext]; ok {
		return payloadKinds[p], p, strings.TrimSuffix(name, path.Ext(name)), nil
	}
	if k, ok := payloadKinds[proto]; ok {
		return k, proto, name, nil
	}
	return "", "", "", ErrBadURL
}

var (
	extProtocols = map[string]domain.Protocol{".torrent": domain.ProtoTorrent, ".nzb": domain.ProtoUsenet}
	payloadKinds = map[domain.Protocol]string{domain.ProtoTorrent: "torrent", domain.ProtoUsenet: "nzb"}
)

// magnetName is the magnet's display name, else its hash.
func magnetName(raw string) string {
	q, err := url.ParseQuery(strings.TrimPrefix(raw, "magnet:?"))
	if err != nil {
		return "magnet"
	}
	if dn := q.Get("dn"); dn != "" {
		return dn
	}
	if xt := q.Get("xt"); xt != "" {
		return strings.TrimPrefix(xt, "urn:btih:")
	}
	return "magnet"
}
