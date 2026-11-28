// Package downloader helps write download-client modules: implement
// Downloader (and any optional interfaces your client supports), then call
// Serve from init. Serve registers exactly the operations the value
// implements, so corsarr offers exactly those actions.
package downloader

import (
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// Downloader is the core every download-client module implements.
type Downloader interface {
	Test() error
	Protocols() []string
	// Add starts a payload and returns its id; adding the same payload again
	// returns the same id.
	Add(req abi.AddRequest) (string, error)
	List(categories []string) ([]abi.Snapshot, error)
	Remove(id string, deleteFiles bool) error
	Pause(id string) error
	Resume(id string) error
	// SetPriority moves a download: top, up, down or bottom.
	SetPriority(id, move string) error
	EnsureCategory(category, savePath string) error
}

// Optional capabilities.
type (
	FileLister interface {
		Files(id string) ([]abi.File, error)
	}
	PriorityLeveler interface{ SetPriorityLevel(id, level string) error }
	GlobalPauser    interface {
		PauseAll() error
		ResumeAll() error
	}
	SpeedLimiter interface {
		SpeedLimits() (abi.SpeedLimits, error)
		SetSpeedLimits(abi.SpeedLimits) error
		AltSpeed() (bool, error)
		SetAltSpeed(on bool) error
	}
	Rechecker   interface{ Recheck(id string) error }
	Reannouncer interface{ Reannounce(id string) error }
	Categoriser interface {
		SetCategory(id, category string) error
	}
	ClientStats   interface{ Stats() (abi.Stats, error) }
	TrackerLister interface {
		Trackers(id string) ([]abi.Tracker, error)
	}
	// DataKeeper is implemented by clients that cannot delete files on
	// remove; corsarr then removes without data.
	DataKeeper interface{ KeepsDataOnRemove() bool }
)

type empty = plugin.Empty

func id(fn func(string) error) func(abi.IDRequest) (empty, error) {
	return func(r abi.IDRequest) (empty, error) { return empty{}, fn(r.ID) }
}

func noArgs(fn func() error) func(empty) (empty, error) {
	return func(empty) (empty, error) { return empty{}, fn() }
}

// Serve registers d's operations.
func Serve(d Downloader) {
	plugin.SetKind(abi.KindDownloader)
	plugin.OnTest(d.Test)
	plugin.Op(abi.OpDLProtocols, func(empty) (abi.ProtocolsResponse, error) {
		return abi.ProtocolsResponse{Protocols: d.Protocols()}, nil
	})
	plugin.Op(abi.OpDLAdd, func(r abi.AddRequest) (abi.AddResponse, error) {
		got, err := d.Add(r)
		return abi.AddResponse{ID: got}, err
	})
	plugin.Op(abi.OpDLList, func(r abi.ListRequest) (abi.ListResponse, error) {
		l, err := d.List(r.Categories)
		return abi.ListResponse{Downloads: l}, err
	})
	plugin.Op(abi.OpDLRemove, func(r abi.RemoveRequest) (empty, error) { return empty{}, d.Remove(r.ID, r.DeleteFiles) })
	plugin.Op(abi.OpDLPause, id(d.Pause))
	plugin.Op(abi.OpDLResume, id(d.Resume))
	plugin.Op(abi.OpDLSetPriority, func(r abi.PriorityRequest) (empty, error) { return empty{}, d.SetPriority(r.ID, r.Priority) })
	plugin.Op(abi.OpDLEnsureCategory, func(r abi.CategoryRequest) (empty, error) {
		return empty{}, d.EnsureCategory(r.Category, r.SavePath)
	})
	serveOptional(d)
}

func serveOptional(d Downloader) {
	for _, reg := range optional {
		reg(d)
	}
	if k, ok := d.(DataKeeper); ok && k.KeepsDataOnRemove() {
		plugin.AddFlag(abi.FlagRemoveKeepsData)
	}
}

var optional = []func(Downloader){
	func(d Downloader) {
		if f, ok := d.(FileLister); ok {
			plugin.Op(abi.OpDLFiles, func(r abi.IDRequest) (abi.FilesResponse, error) {
				files, err := f.Files(r.ID)
				return abi.FilesResponse{Files: files}, err
			})
		}
	},
	func(d Downloader) {
		if p, ok := d.(PriorityLeveler); ok {
			plugin.Op(abi.OpDLSetPriorityLevel, func(r abi.PriorityRequest) (empty, error) {
				return empty{}, p.SetPriorityLevel(r.ID, r.Priority)
			})
		}
	},
	func(d Downloader) {
		if g, ok := d.(GlobalPauser); ok {
			plugin.Op(abi.OpDLPauseAll, noArgs(g.PauseAll))
			plugin.Op(abi.OpDLResumeAll, noArgs(g.ResumeAll))
		}
	},
	serveSpeed,
	func(d Downloader) {
		if r, ok := d.(Rechecker); ok {
			plugin.Op(abi.OpDLRecheck, id(r.Recheck))
		}
		if r, ok := d.(Reannouncer); ok {
			plugin.Op(abi.OpDLReannounce, id(r.Reannounce))
		}
	},
	func(d Downloader) {
		if c, ok := d.(Categoriser); ok {
			plugin.Op(abi.OpDLSetCategory, func(r abi.CategoryRequest) (empty, error) { return empty{}, c.SetCategory(r.ID, r.Category) })
		}
	},
	func(d Downloader) {
		if s, ok := d.(ClientStats); ok {
			plugin.Op(abi.OpDLStats, func(empty) (abi.Stats, error) { return s.Stats() })
		}
	},
	func(d Downloader) {
		if t, ok := d.(TrackerLister); ok {
			plugin.Op(abi.OpDLTrackers, func(r abi.IDRequest) (abi.TrackersResponse, error) {
				list, err := t.Trackers(r.ID)
				return abi.TrackersResponse{Trackers: list}, err
			})
		}
	},
}

func serveSpeed(d Downloader) {
	s, ok := d.(SpeedLimiter)
	if !ok {
		return
	}
	plugin.Op(abi.OpDLSpeedLimits, func(empty) (abi.SpeedLimits, error) { return s.SpeedLimits() })
	plugin.Op(abi.OpDLSetSpeedLimits, func(l abi.SpeedLimits) (empty, error) { return empty{}, s.SetSpeedLimits(l) })
	plugin.Op(abi.OpDLAltSpeed, func(empty) (abi.Toggle, error) {
		on, err := s.AltSpeed()
		return abi.Toggle{On: on}, err
	})
	plugin.Op(abi.OpDLSetAltSpeed, func(t abi.Toggle) (empty, error) { return empty{}, s.SetAltSpeed(t.On) })
}
