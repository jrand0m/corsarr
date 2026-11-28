package client

import "context"

// RPC versions that changed what this package sends.
const (
	rpcLabels    = 16 // 3.00: torrent labels readable and settable
	rpcAddLabels = 17 // 4.0: labels accepted when adding
)

// defaultSpeedBytes is Transmission's speed unit (kB = 1000 bytes) when the
// session does not report one.
const defaultSpeedBytes = 1000

type sessionInfo struct {
	Version       string `json:"version"`
	RPCVersion    int    `json:"rpc-version"`
	DownloadDir   string `json:"download-dir"`
	SpeedDown     int64  `json:"speed-limit-down"`
	SpeedDownOn   bool   `json:"speed-limit-down-enabled"`
	SpeedUp       int64  `json:"speed-limit-up"`
	SpeedUpOn     bool   `json:"speed-limit-up-enabled"`
	AltSpeedOn    bool   `json:"alt-speed-enabled"`
	SpeedUnitSize struct {
		SpeedBytes int64 `json:"speed-bytes"`
	} `json:"units"`
}

func (s sessionInfo) speedBytes() int64 {
	if s.SpeedUnitSize.SpeedBytes > 0 {
		return s.SpeedUnitSize.SpeedBytes
	}
	return defaultSpeedBytes
}

// sessionGet reads the session settings and remembers the server version.
func (c *Client) sessionGet(ctx context.Context) (sessionInfo, error) {
	var s sessionInfo
	if err := c.call(ctx, "session-get", nil, &s); err != nil {
		return s, err
	}
	c.mu.Lock()
	c.info = &serverInfo{Version: s.Version, RPCVersion: s.RPCVersion}
	c.mu.Unlock()
	return s, nil
}

// server returns what is known about the server, asking it once if needed.
func (c *Client) server(ctx context.Context) (serverInfo, error) {
	c.mu.Lock()
	info := c.info
	c.mu.Unlock()
	if info != nil {
		return *info, nil
	}
	s, err := c.sessionGet(ctx)
	return serverInfo{Version: s.Version, RPCVersion: s.RPCVersion}, err
}

func (c *Client) hasLabels(ctx context.Context) (bool, error) {
	info, err := c.server(ctx)
	return info.RPCVersion >= rpcLabels, err
}

// TestResult is what a connection test learned.
type TestResult struct {
	Version     string // for example "4.0.6 (38c164933e)"
	RPCVersion  int
	DownloadDir string // default download directory, host view
	FreeSpace   int64  // bytes free in the default download directory
}

// Test reads the session (version and default download directory) and the
// free space there.
func (c *Client) Test(ctx context.Context) (TestResult, error) {
	s, err := c.sessionGet(ctx)
	if err != nil {
		return TestResult{}, err
	}
	free, err := c.freeSpace(ctx, s.DownloadDir)
	return TestResult{Version: s.Version, RPCVersion: s.RPCVersion, DownloadDir: c.Local(s.DownloadDir), FreeSpace: free}, err
}

// FreeSpace returns the bytes free in a directory given in the host's view;
// "" means the default download directory.
func (c *Client) FreeSpace(ctx context.Context, dir string) (int64, error) {
	remote := c.Remote(dir)
	if dir == "" {
		s, err := c.sessionGet(ctx)
		if err != nil {
			return 0, err
		}
		remote = s.DownloadDir
	}
	return c.freeSpace(ctx, remote)
}

func (c *Client) freeSpace(ctx context.Context, remoteDir string) (int64, error) {
	var res struct {
		SizeBytes int64 `json:"size-bytes"`
	}
	err := c.call(ctx, "free-space", map[string]any{"path": remoteDir}, &res)
	return res.SizeBytes, err
}

// SpeedLimits are global transfer limits in KiB/s; 0 is unlimited.
type SpeedLimits struct {
	DownKiB int64
	UpKiB   int64
}

func limitsOf(s sessionInfo) SpeedLimits {
	var l SpeedLimits
	if s.SpeedDownOn {
		l.DownKiB = toKiB(s.SpeedDown, s.speedBytes())
	}
	if s.SpeedUpOn {
		l.UpKiB = toKiB(s.SpeedUp, s.speedBytes())
	}
	return l
}

func toKiB(v, unit int64) int64 { return v * unit / 1024 }

// fromKiB converts KiB/s into the session's unit, never rounding a limit to 0.
func fromKiB(kib, unit int64) int64 { return max((kib*1024+unit/2)/unit, 1) }

// SpeedLimits reads the global download and upload limits.
func (c *Client) SpeedLimits(ctx context.Context) (SpeedLimits, error) {
	s, err := c.sessionGet(ctx)
	return limitsOf(s), err
}

// SetSpeedLimits sets the global download and upload limits (0 = unlimited).
func (c *Client) SetSpeedLimits(ctx context.Context, l SpeedLimits) error {
	if l.DownKiB < 0 || l.UpKiB < 0 {
		return errorf(ClassConfig, "speed limits must be 0 (unlimited) or more")
	}
	s, err := c.sessionGet(ctx)
	if err != nil {
		return err
	}
	args := map[string]any{"speed-limit-down-enabled": l.DownKiB > 0, "speed-limit-up-enabled": l.UpKiB > 0}
	if l.DownKiB > 0 {
		args["speed-limit-down"] = fromKiB(l.DownKiB, s.speedBytes())
	}
	if l.UpKiB > 0 {
		args["speed-limit-up"] = fromKiB(l.UpKiB, s.speedBytes())
	}
	return c.call(ctx, "session-set", args, nil)
}

// AltSpeed reports whether the alternative speed limits (turtle mode) are on.
func (c *Client) AltSpeed(ctx context.Context) (bool, error) {
	s, err := c.sessionGet(ctx)
	return s.AltSpeedOn, err
}

// SetAltSpeed turns the alternative speed limits on or off.
func (c *Client) SetAltSpeed(ctx context.Context, on bool) error {
	return c.call(ctx, "session-set", map[string]any{"alt-speed-enabled": on}, nil)
}

// PauseAll stops every torrent.
func (c *Client) PauseAll(ctx context.Context) error { return c.call(ctx, "torrent-stop", nil, nil) }

// ResumeAll starts every torrent.
func (c *Client) ResumeAll(ctx context.Context) error { return c.call(ctx, "torrent-start", nil, nil) }

// Status is the client-wide state at one moment.
type Status struct {
	DownSpeed int64 // bytes/s
	UpSpeed   int64 // bytes/s
	FreeSpace int64 // bytes free in the default download directory; -1 unknown
	Paused    bool  // every torrent is stopped
	Limits    SpeedLimits
	AltSpeed  bool
	Torrents  int
}

type sessionStats struct {
	DownloadSpeed      int64 `json:"downloadSpeed"`
	UploadSpeed        int64 `json:"uploadSpeed"`
	TorrentCount       int   `json:"torrentCount"`
	PausedTorrentCount int   `json:"pausedTorrentCount"`
}

// Stats reads speeds, free space, limits and whether every torrent is
// stopped. Free space that cannot be read is reported as -1.
func (c *Client) Stats(ctx context.Context) (Status, error) {
	var st sessionStats
	if err := c.call(ctx, "session-stats", nil, &st); err != nil {
		return Status{}, err
	}
	s, err := c.sessionGet(ctx)
	if err != nil {
		return Status{}, err
	}
	free, err := c.freeSpace(ctx, s.DownloadDir)
	if err != nil {
		free = -1
	}
	return Status{DownSpeed: st.DownloadSpeed, UpSpeed: st.UploadSpeed, FreeSpace: free,
		Paused: st.TorrentCount > 0 && st.PausedTorrentCount == st.TorrentCount,
		Limits: limitsOf(s), AltSpeed: s.AltSpeedOn, Torrents: st.TorrentCount}, nil
}
