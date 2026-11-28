package wasm

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Downloader makes a downloader module look like a built-in download client:
// it implements plugin.Downloader and every optional capability interface,
// and reports through plugin.CapabilityGate which ones the module answers,
// so plugin.As sees exactly the module's capabilities.
type Downloader struct{ h *Handle }

var (
	_ plugin.Downloader      = (*Downloader)(nil)
	_ plugin.FileLister      = (*Downloader)(nil)
	_ plugin.PriorityLeveler = (*Downloader)(nil)
	_ plugin.GlobalPauser    = (*Downloader)(nil)
	_ plugin.SpeedLimiter    = (*Downloader)(nil)
	_ plugin.Rechecker       = (*Downloader)(nil)
	_ plugin.Reannouncer     = (*Downloader)(nil)
	_ plugin.Categoriser     = (*Downloader)(nil)
	_ plugin.ClientStats     = (*Downloader)(nil)
	_ plugin.TrackerLister   = (*Downloader)(nil)
	_ plugin.DataKeeper      = (*Downloader)(nil)
	_ plugin.CapabilityGate  = (*Downloader)(nil)
)

// downloaderCaps maps each optional interface to the operations it needs.
var downloaderCaps = map[reflect.Type][]string{
	reflect.TypeFor[plugin.FileLister]():      {abi.OpDLFiles},
	reflect.TypeFor[plugin.PriorityLeveler](): {abi.OpDLSetPriorityLevel},
	reflect.TypeFor[plugin.GlobalPauser]():    {abi.OpDLPauseAll, abi.OpDLResumeAll},
	reflect.TypeFor[plugin.SpeedLimiter]():    {abi.OpDLSpeedLimits, abi.OpDLSetSpeedLimits, abi.OpDLAltSpeed, abi.OpDLSetAltSpeed},
	reflect.TypeFor[plugin.Rechecker]():       {abi.OpDLRecheck},
	reflect.TypeFor[plugin.Reannouncer]():     {abi.OpDLReannounce},
	reflect.TypeFor[plugin.Categoriser]():     {abi.OpDLSetCategory},
	reflect.TypeFor[plugin.ClientStats]():     {abi.OpDLStats},
	reflect.TypeFor[plugin.TrackerLister]():   {abi.OpDLTrackers},
}

// Supports implements plugin.CapabilityGate.
func (d *Downloader) Supports(t reflect.Type) bool {
	ops, ok := downloaderCaps[t]
	if !ok {
		return true
	}
	for _, op := range ops {
		if !d.h.has(op) {
			return false
		}
	}
	return true
}

// ModuleHandle implements Handled.
func (d *Downloader) ModuleHandle() *Handle { return d.h }

// Close drains the instance.
func (d *Downloader) Close() error { return d.h.Close() }

// KeepsDataOnRemove implements plugin.DataKeeper.
func (d *Downloader) KeepsDataOnRemove() bool { return d.h.flag(abi.FlagRemoveKeepsData) }

// call runs an optional operation, answering ErrUnsupported when the module
// lacks it.
func (d *Downloader) call(ctx context.Context, op string, req, resp any) error {
	if !d.h.has(op) {
		return fmt.Errorf("%s: %w", op, plugin.ErrUnsupported)
	}
	return d.h.Call(ctx, op, req, resp)
}

// Test implements plugin.Tester.
func (d *Downloader) Test(ctx context.Context) error { return d.h.Test(ctx) }

// protocolsTimeout bounds Protocols, which has no context of its own.
const protocolsTimeout = 5 * time.Second

// Protocols implements plugin.Downloader.
func (d *Downloader) Protocols() []domain.Protocol {
	ctx, cancel := context.WithTimeout(context.Background(), protocolsTimeout)
	defer cancel()
	var r abi.ProtocolsResponse
	if err := d.h.Call(ctx, abi.OpDLProtocols, struct{}{}, &r); err != nil {
		return nil
	}
	out := make([]domain.Protocol, 0, len(r.Protocols))
	for _, p := range r.Protocols {
		if p == string(domain.ProtoTorrent) || p == string(domain.ProtoUsenet) || p == string(domain.ProtoSource) {
			out = append(out, domain.Protocol(p))
		}
	}
	return out
}

// Add implements plugin.Downloader.
func (d *Downloader) Add(ctx context.Context, req plugin.AddRequest) (string, error) {
	var r abi.AddResponse
	in := abi.AddRequest{Payload: abi.Payload{Kind: req.Payload.Kind, URL: req.Payload.URL, Content: req.Payload.Content,
		Name: req.Payload.Name}, Category: req.Category, SavePath: req.SavePath, Paused: req.Paused}
	if err := d.h.Call(ctx, abi.OpDLAdd, in, &r); err != nil {
		return "", err
	}
	if r.ID == "" {
		return "", plugin.Errorf(plugin.ClassParse, "module returned an empty download id")
	}
	return r.ID, nil
}

// List implements plugin.Downloader.
func (d *Downloader) List(ctx context.Context, categories []string) ([]plugin.Snapshot, error) {
	var r abi.ListResponse
	if err := d.h.Call(ctx, abi.OpDLList, abi.ListRequest{Categories: categories}, &r); err != nil {
		return nil, err
	}
	out := make([]plugin.Snapshot, 0, len(r.Downloads))
	for _, s := range r.Downloads {
		out = append(out, snapshot(s))
	}
	return out, nil
}

var moduleStates = map[string]domain.DownloadState{
	string(domain.StateQueued): domain.StateQueued, string(domain.StateDownloading): domain.StateDownloading,
	string(domain.StateStalled): domain.StateStalled, string(domain.StateCompleted): domain.StateCompleted,
	string(domain.StateFailed): domain.StateFailed,
}

func snapshot(s abi.Snapshot) plugin.Snapshot {
	state, ok := moduleStates[s.State]
	if !ok {
		state = domain.StateQueued
	}
	out := plugin.Snapshot{DownloadID: s.ID, Name: s.Name, Category: s.Category, State: state, Progress: s.Progress,
		Size: s.Size, Downloaded: s.Downloaded, Speed: s.Speed, ETASeconds: s.ETASeconds, UpSpeed: s.UpSpeed,
		Ratio: s.Ratio, Seeders: s.Seeders, Peers: s.Peers, SavePath: s.SavePath, ContentPath: s.ContentPath,
		Files: s.Files, Error: s.Error}
	if s.LastActivityAt > 0 {
		t := time.Unix(s.LastActivityAt, 0).UTC()
		out.LastActivityAt = &t
	}
	return out
}

// Remove implements plugin.Downloader.
func (d *Downloader) Remove(ctx context.Context, id string, deleteFiles bool) error {
	return d.h.Call(ctx, abi.OpDLRemove, abi.RemoveRequest{ID: id, DeleteFiles: deleteFiles}, nil)
}

// Pause implements plugin.Downloader.
func (d *Downloader) Pause(ctx context.Context, id string) error {
	return d.h.Call(ctx, abi.OpDLPause, abi.IDRequest{ID: id}, nil)
}

// Resume implements plugin.Downloader.
func (d *Downloader) Resume(ctx context.Context, id string) error {
	return d.h.Call(ctx, abi.OpDLResume, abi.IDRequest{ID: id}, nil)
}

// SetPriority implements plugin.Downloader.
func (d *Downloader) SetPriority(ctx context.Context, id string, p plugin.Priority) error {
	return d.h.Call(ctx, abi.OpDLSetPriority, abi.PriorityRequest{ID: id, Priority: string(p)}, nil)
}

// EnsureCategory implements plugin.Downloader.
func (d *Downloader) EnsureCategory(ctx context.Context, category, savePath string) error {
	return d.h.Call(ctx, abi.OpDLEnsureCategory, abi.CategoryRequest{Category: category, SavePath: savePath}, nil)
}

// Files implements plugin.FileLister.
func (d *Downloader) Files(ctx context.Context, id string) ([]plugin.FileEntry, error) {
	var r abi.FilesResponse
	if err := d.call(ctx, abi.OpDLFiles, abi.IDRequest{ID: id}, &r); err != nil {
		return nil, err
	}
	out := make([]plugin.FileEntry, len(r.Files))
	for i, f := range r.Files {
		out[i] = plugin.FileEntry{Path: f.Path, Size: f.Size}
	}
	return out, nil
}

// SetPriorityLevel implements plugin.PriorityLeveler.
func (d *Downloader) SetPriorityLevel(ctx context.Context, id string, level plugin.Priority) error {
	return d.call(ctx, abi.OpDLSetPriorityLevel, abi.PriorityRequest{ID: id, Priority: string(level)}, nil)
}

// PauseAll implements plugin.GlobalPauser.
func (d *Downloader) PauseAll(ctx context.Context) error {
	return d.call(ctx, abi.OpDLPauseAll, struct{}{}, nil)
}

// ResumeAll implements plugin.GlobalPauser.
func (d *Downloader) ResumeAll(ctx context.Context) error {
	return d.call(ctx, abi.OpDLResumeAll, struct{}{}, nil)
}

// SpeedLimits implements plugin.SpeedLimiter.
func (d *Downloader) SpeedLimits(ctx context.Context) (plugin.SpeedLimits, error) {
	var r abi.SpeedLimits
	err := d.call(ctx, abi.OpDLSpeedLimits, struct{}{}, &r)
	return plugin.SpeedLimits{DownKiB: r.DownKiB, UpKiB: r.UpKiB}, err
}

// SetSpeedLimits implements plugin.SpeedLimiter.
func (d *Downloader) SetSpeedLimits(ctx context.Context, l plugin.SpeedLimits) error {
	return d.call(ctx, abi.OpDLSetSpeedLimits, abi.SpeedLimits{DownKiB: l.DownKiB, UpKiB: l.UpKiB}, nil)
}

// AltSpeed implements plugin.SpeedLimiter.
func (d *Downloader) AltSpeed(ctx context.Context) (bool, error) {
	var r abi.Toggle
	err := d.call(ctx, abi.OpDLAltSpeed, struct{}{}, &r)
	return r.On, err
}

// SetAltSpeed implements plugin.SpeedLimiter.
func (d *Downloader) SetAltSpeed(ctx context.Context, on bool) error {
	return d.call(ctx, abi.OpDLSetAltSpeed, abi.Toggle{On: on}, nil)
}

// Recheck implements plugin.Rechecker.
func (d *Downloader) Recheck(ctx context.Context, id string) error {
	return d.call(ctx, abi.OpDLRecheck, abi.IDRequest{ID: id}, nil)
}

// Reannounce implements plugin.Reannouncer.
func (d *Downloader) Reannounce(ctx context.Context, id string) error {
	return d.call(ctx, abi.OpDLReannounce, abi.IDRequest{ID: id}, nil)
}

// SetCategory implements plugin.Categoriser.
func (d *Downloader) SetCategory(ctx context.Context, id, category string) error {
	return d.call(ctx, abi.OpDLSetCategory, abi.CategoryRequest{ID: id, Category: category}, nil)
}

// Stats implements plugin.ClientStats.
func (d *Downloader) Stats(ctx context.Context) (plugin.ClientStatus, error) {
	var r abi.Stats
	if err := d.call(ctx, abi.OpDLStats, struct{}{}, &r); err != nil {
		return plugin.ClientStatus{}, err
	}
	return plugin.ClientStatus{DownSpeed: r.DownSpeed, UpSpeed: r.UpSpeed, FreeSpace: r.FreeSpace, Paused: r.Paused,
		Limits: plugin.SpeedLimits{DownKiB: r.Limits.DownKiB, UpKiB: r.Limits.UpKiB}, AltSpeed: r.AltSpeed}, nil
}

// Trackers implements plugin.TrackerLister.
func (d *Downloader) Trackers(ctx context.Context, id string) ([]plugin.Tracker, error) {
	var r abi.TrackersResponse
	if err := d.call(ctx, abi.OpDLTrackers, abi.IDRequest{ID: id}, &r); err != nil {
		return nil, err
	}
	out := make([]plugin.Tracker, len(r.Trackers))
	for i, t := range r.Trackers {
		out[i] = plugin.Tracker{URL: t.URL, Status: t.Status, Message: t.Message, Seeds: t.Seeds, Peers: t.Peers}
	}
	return out, nil
}
