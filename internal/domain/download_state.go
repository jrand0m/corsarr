package domain

import "fmt"

// DownloadState is the queue state of a download.
type DownloadState string

const (
	StateQueued        DownloadState = "queued"
	StateDownloading   DownloadState = "downloading"
	StateStalled       DownloadState = "stalled"
	StateCompleted     DownloadState = "completed"
	StateImportPending DownloadState = "import-pending"
	StateImporting     DownloadState = "importing"
	StateImportBlocked DownloadState = "import-blocked"
	StateImported      DownloadState = "imported"
	StateFailed        DownloadState = "failed"
	StateRemoved       DownloadState = "removed"
)

// Actor names who requested a transition; the ownership table decides which
// actor may request which move.
type Actor string

const (
	ActorDownloader Actor = "downloader" // download clients, from client snapshots
	ActorImport     Actor = "import"     // the importer
	ActorAutomation Actor = "automation" // automation rules and failure follow-up
	ActorUser       Actor = "user"
	ActorSafety     Actor = "safety" // queue safety action on payload.rejected
)

type edge struct{ from, to DownloadState }

// transitions maps every legal move to the actors allowed to request it.
var transitions = map[edge][]Actor{
	{StateQueued, StateDownloading}:      {ActorDownloader},
	{StateDownloading, StateStalled}:     {ActorDownloader},
	{StateStalled, StateDownloading}:     {ActorDownloader},
	{StateQueued, StateCompleted}:        {ActorDownloader},
	{StateDownloading, StateCompleted}:   {ActorDownloader},
	{StateStalled, StateCompleted}:       {ActorDownloader},
	{StateCompleted, StateImportPending}: {ActorImport},
	{StateImportPending, StateImporting}: {ActorImport},
	{StateImportBlocked, StateImporting}: {ActorImport, ActorUser},
	{StateImporting, StateImported}:      {ActorImport},
	{StateImporting, StateImportBlocked}: {ActorImport},
	{StateImporting, StateFailed}:        {ActorImport},
}

// failable states may move to failed or removed by downloader, automation,
// user or the safety action.
var failable = []DownloadState{
	StateQueued, StateDownloading, StateStalled, StateCompleted,
	StateImportPending, StateImportBlocked,
}

func init() {
	for _, s := range failable {
		transitions[edge{s, StateFailed}] = []Actor{ActorDownloader, ActorAutomation, ActorUser, ActorSafety}
		transitions[edge{s, StateRemoved}] = []Actor{ActorDownloader, ActorAutomation, ActorUser, ActorSafety}
	}
	transitions[edge{StateImported, StateRemoved}] = []Actor{ActorDownloader, ActorAutomation, ActorUser}
	transitions[edge{StateFailed, StateRemoved}] = []Actor{ActorDownloader, ActorAutomation, ActorUser, ActorSafety}
}

// CheckTransition is the pure rule behind the download module's single
// transition operation: it returns an error for illegal moves or actors.
func CheckTransition(from, to DownloadState, by Actor) error {
	actors, ok := transitions[edge{from, to}]
	if !ok {
		return fmt.Errorf("illegal download transition %s → %s", from, to)
	}
	for _, a := range actors {
		if a == by {
			return nil
		}
	}
	return fmt.Errorf("actor %s may not move a download %s → %s", by, from, to)
}

// Terminal reports whether no further work happens for the download.
func (s DownloadState) Terminal() bool { return s == StateImported || s == StateRemoved }
