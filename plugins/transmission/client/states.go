package client

// State is a torrent's state in the host's vocabulary.
type State string

// States a torrent can be reported in. QueueState folds them onto the
// host's download queue states (queued, downloading, stalled, completed,
// failed).
const (
	StateQueued      State = "queued"
	StateDownloading State = "downloading"
	StateStalled     State = "stalled"
	StatePaused      State = "paused"
	StateCompleted   State = "completed"
	StateSeeding     State = "seeding"
	StateChecking    State = "checking"
	StateFailed      State = "failed"
)

// Transmission torrent status codes.
const (
	statusStopped      = 0
	statusCheckWait    = 1
	statusCheck        = 2
	statusDownloadWait = 3
	statusDownload     = 4
	statusSeedWait     = 5
	statusSeed         = 6
)

// Transmission torrent error codes.
const (
	errNone           = 0
	errTrackerWarning = 1
	errTrackerError   = 2
	errLocal          = 3
)

// LocalErrorHint explains a torrent with a local error. Such a torrent is
// reported as stalled with this reason, never as failed: a full disk, a
// missing mount or a permission problem needs a fix outside the host, and a
// failed download would be blocklisted and searched again.
const LocalErrorHint = "transmission reports a local error for this torrent (often a full disk, a missing drive or a permission problem); fix it and verify or start the torrent in Transmission"

// done reports whether every wanted byte is present.
func (t rawTorrent) done() bool {
	return (t.LeftUntilDone == 0 && t.SizeWhenDone > 0) || t.PercentDone >= 1
}

// stateOf maps a torrent onto a State; unknown status codes count as queued.
func stateOf(t rawTorrent) State {
	if t.Error == errLocal {
		return StateStalled
	}
	switch t.Status {
	case statusStopped:
		return stoppedState(t)
	case statusCheckWait, statusCheck:
		return StateChecking
	case statusDownload:
		return downloadingState(t)
	case statusSeedWait:
		return StateCompleted
	case statusSeed:
		return StateSeeding
	}
	return StateQueued
}

func stoppedState(t rawTorrent) State {
	if t.done() {
		return StateCompleted
	}
	return StatePaused
}

// downloadingState tells an active transfer from one no peer feeds.
func downloadingState(t rawTorrent) State {
	if t.RateDownload == 0 && t.PeersSendingToUs == 0 {
		return StateStalled
	}
	return StateDownloading
}

// QueueState folds a State onto the host's queue states: paused counts as
// queued, seeding as completed, and checking as downloading until every
// wanted byte is present.
func QueueState(s State, done bool) State {
	switch s {
	case StatePaused:
		return StateQueued
	case StateSeeding:
		return StateCompleted
	case StateChecking:
		if done {
			return StateCompleted
		}
		return StateDownloading
	}
	return s
}
