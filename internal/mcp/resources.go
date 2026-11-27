package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Resource URIs.
const (
	URIQueue     = "corsarr://queue"
	URIHealth    = "corsarr://health"
	itemPrefix   = "corsarr://item/"
	itemTemplate = "corsarr://item/{kind}/{id}"
)

type resourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType"`
}

func itemURI(kind domain.MediaKind, id int64) string {
	return fmt.Sprintf("%s%s/%d", itemPrefix, kind, id)
}

type listParams struct {
	Cursor string `json:"cursor"`
}

// listResources lists the queue, health and then the library items, paged
// by kind: the cursor is "<kind>:<offset>".
func (s *Service) listResources(ctx context.Context, _ *Session, params json.RawMessage) (any, error) {
	var p listParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	out := []resourceInfo{}
	if p.Cursor == "" {
		out = append(out,
			resourceInfo{URI: URIQueue, Name: "queue", Title: "Download queue", Description: "Downloads in progress with their linked items.", MimeType: "application/json"},
			resourceInfo{URI: URIHealth, Name: "health", Title: "Health", Description: "Integration health, root folders and kind modes.", MimeType: "application/json"})
	}
	items, next, err := s.itemResources(ctx, p.Cursor)
	result := map[string]any{"resources": append(out, items...)}
	if next != "" {
		result["nextCursor"] = next
	}
	return result, err
}

var resourceKinds = []domain.MediaKind{domain.KindMovie, domain.KindTV, domain.KindMusic}

func (s *Service) itemResources(ctx context.Context, cursor string) ([]resourceInfo, string, error) {
	ki, offset, err := parseResourceCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	for ; ki < len(resourceKinds); ki, offset = ki+1, 0 {
		all := []LibraryItem{}
		if s.usable(resourceKinds[ki]) != nil {
			continue
		}
		if err := s.eachItem(ctx, resourceKinds[ki], func(it LibraryItem) error { all = append(all, it); return nil }); err != nil {
			return nil, "", err
		}
		if offset < len(all) {
			return pageResources(all, ki, offset)
		}
	}
	return nil, "", nil
}

func pageResources(all []LibraryItem, ki, offset int) ([]resourceInfo, string, error) {
	end := min(offset+DefaultLimit, len(all))
	out := make([]resourceInfo, 0, end-offset)
	for _, it := range all[offset:end] {
		out = append(out, resourceInfo{URI: itemURI(it.Kind, it.ID), Name: it.Title, MimeType: "application/json"})
	}
	next := fmt.Sprintf("%d:%d", ki, end)
	if end == len(all) {
		next = fmt.Sprintf("%d:0", ki+1)
	}
	if ki+1 == len(resourceKinds) && end == len(all) {
		next = ""
	}
	return out, next, nil
}

func parseResourceCursor(c string) (int, int, error) {
	if c == "" {
		return 0, 0, nil
	}
	a, b, ok := strings.Cut(c, ":")
	ki, err1 := strconv.Atoi(a)
	off, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil || ki < 0 || off < 0 {
		return 0, 0, rpcErr(codeInvalidParams, "invalid cursor")
	}
	return ki, off, nil
}

func (s *Service) listTemplates(context.Context, *Session, json.RawMessage) (any, error) {
	return map[string]any{"resourceTemplates": []map[string]string{{
		"uriTemplate": itemTemplate, "name": "item", "title": "Library item",
		"description": "A movie, series or artist: kind is movie, tv or music, id its library id.", "mimeType": "application/json",
	}}}, nil
}

type uriParams struct {
	URI string `json:"uri"`
}

func (s *Service) readResource(ctx context.Context, sess *Session, params json.RawMessage) (any, error) {
	var p uriParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	if !s.allow(sess) {
		return nil, rpcErr(codeRateLimited, "rate limit reached: at most %d calls per minute", s.cfg().RateLimit)
	}
	data, err := s.resourceData(ctx, p.URI)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"contents": []map[string]string{{"uri": p.URI, "mimeType": "application/json", "text": string(b)}}}, nil
}

func (s *Service) resourceData(ctx context.Context, uri string) (any, error) {
	switch {
	case uri == URIQueue:
		return s.queueData(ctx)
	case uri == URIHealth:
		res, err := s.getHealth(ctx, struct{}{})
		return res.Data, err
	case strings.HasPrefix(uri, itemPrefix):
		return s.itemData(ctx, uri)
	}
	return nil, rpcErr(codeNotFound, "no resource %q", uri)
}

func (s *Service) queueData(ctx context.Context) (any, error) {
	list, next, err := s.d.Queue.List(ctx, 0, MaxLimit)
	return map[string]any{"downloads": list, "more": next > 0}, err
}

func (s *Service) itemData(ctx context.Context, uri string) (any, error) {
	kind, idText, _ := strings.Cut(strings.TrimPrefix(uri, itemPrefix), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil, rpcErr(codeNotFound, "no resource %q", uri)
	}
	res, err := s.getItem(ctx, itemArgs{Kind: kind, ID: id})
	if err != nil {
		return nil, rpcErr(codeNotFound, "%s: %s", uri, errText(err))
	}
	return res.Data, nil
}

func (s *Service) subscribe(_ context.Context, sess *Session, params json.RawMessage) (any, error) {
	return s.setSubscription(sess, params, true)
}

func (s *Service) unsubscribe(_ context.Context, sess *Session, params json.RawMessage) (any, error) {
	return s.setSubscription(sess, params, false)
}

func (s *Service) setSubscription(sess *Session, params json.RawMessage, on bool) (any, error) {
	var p uriParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	if p.URI != URIQueue && p.URI != URIHealth && !strings.HasPrefix(p.URI, itemPrefix) {
		return nil, rpcErr(codeNotFound, "no resource %q", p.URI)
	}
	sess.setSub(p.URI, on)
	return struct{}{}, nil
}

// ── change notifications ─────────────────────────────────────────────────────

// Run turns bus events into resources/updated notifications for the
// sessions subscribed to them, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	sub := s.d.Bus.Subscribe(256, func(t domain.EventType) bool { return t.Streamed() })
	defer sub.Close()
	for {
		ev, err := sub.Next(ctx)
		if err != nil {
			return
		}
		s.publish(ctx, ev)
	}
}

func (s *Service) publish(ctx context.Context, ev domain.Event) {
	for _, uri := range s.urisOf(ctx, ev) {
		msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/resources/updated", "params": map[string]string{"uri": uri}})
		s.eachSession(func(sess *Session) {
			if sess.subscribed(uri) {
				sess.notify(msg)
			}
		})
	}
}

// urisOf names the resources an event changes.
func (s *Service) urisOf(ctx context.Context, ev domain.Event) []string {
	var out []string
	switch t := string(ev.Type); {
	case ev.Type == domain.EvHealthChanged:
		out = append(out, URIHealth)
	case strings.HasPrefix(t, "grab.") || strings.HasPrefix(t, "download.") || strings.HasPrefix(t, "import."):
		out = append(out, URIQueue)
	}
	for _, it := range ev.Items {
		if uri := s.ownerURI(ctx, it.Ref); uri != "" {
			out = append(out, uri)
		}
	}
	return out
}

// ownerURI maps an event's item to its resource: episodes to their series,
// albums to their artist.
func (s *Service) ownerURI(ctx context.Context, ref domain.ItemRef) string {
	switch {
	case ref.Kind == domain.KindMovie, ref.Variant == domain.VariantSeries, ref.Variant == domain.VariantArtist:
		return itemURI(ref.Kind, ref.ID)
	case ref.Kind == domain.KindTV:
		e, err := s.d.TV.Episode(ctx, ref.ID)
		return uriIf(err, domain.KindTV, e.SeriesID)
	case ref.Kind == domain.KindMusic && ref.Variant == "":
		al, err := s.d.Music.Album(ctx, ref.ID)
		return uriIf(err, domain.KindMusic, al.ArtistID)
	}
	return ""
}

func uriIf(err error, kind domain.MediaKind, id int64) string {
	if err != nil {
		return ""
	}
	return itemURI(kind, id)
}
