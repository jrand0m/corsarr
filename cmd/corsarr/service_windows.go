//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceWait = 30 * time.Second

func (c cli) service(ctx context.Context, action string) int {
	var err error
	switch action {
	case "install":
		err = c.serviceInstall()
	case "uninstall":
		err = c.serviceUninstall()
	case "start":
		err = withService(func(s *mgr.Service) error { return startService(s, c.stdout) })
	case "stop":
		err = withService(func(s *mgr.Service) error { return stopService(s, c.stdout) })
	case "status":
		return c.serviceStatus()
	case "run":
		return c.serviceRun(ctx)
	default:
		return serviceUsage(c.stderr)
	}
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr service "+action+":", explainServiceErr(err))
		return 1
	}
	return 0
}

func explainServiceErr(err error) error {
	switch {
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("%w (run it from an elevated, administrator shell)", err)
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return errors.New("the corsarr service is not installed (corsarr service install)")
	case errors.Is(err, windows.ERROR_SERVICE_EXISTS):
		return errors.New("the corsarr service is already installed (corsarr service uninstall first)")
	}
	return err
}

func (c cli) serviceInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	plan := planService(programData, c.flags["account"], c.cfgArgs)
	if underProfile(exe, os.Getenv("USERPROFILE")) {
		fmt.Fprintf(c.stderr, "warning: %s is inside your user profile, which %s usually cannot read; "+
			"install corsarr machine-wide (scoop install -g, winget install --scope machine)\n", exe, plan.Account)
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: serviceDisplayName, Description: serviceDescription,
		StartType: mgr.StartAutomatic, DelayedAutoStart: true, ServiceStartName: plan.Account,
	}, plan.Args...)
	if err != nil {
		return err
	}
	defer s.Close()
	// restart after a crash: 5s, 30s, then every minute; the count resets after a day
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}, 24*60*60); err != nil {
		return rollback(s, err)
	}
	_ = eventlog.Remove(serviceName) // a source left behind by an earlier install
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return rollback(s, err)
	}
	for _, d := range plan.Dirs {
		if err := grantModify(d, plan.Account); err != nil {
			return rollback(s, fmt.Errorf("granting %s access to %s: %w", plan.Account, d, err))
		}
	}
	fmt.Fprintf(c.stdout, "installed service %s: %s %v as %s\nstart it with: corsarr service start\n",
		serviceName, exe, plan.Args, plan.Account)
	return nil
}

func rollback(s *mgr.Service, cause error) error {
	_ = s.Delete()
	return cause
}

// grantModify creates dir and gives account inheritable modify rights on it.
func grantModify(dir, account string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	old, _, err := sd.DACL()
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_READ | windows.GENERIC_WRITE | windows.GENERIC_EXECUTE | windows.DELETE,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_NAME,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromString(account),
		},
	}}, old)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

func (c cli) serviceUninstall() error {
	return withService(func(s *mgr.Service) error {
		if st, err := s.Query(); err == nil && st.State != svc.Stopped {
			if err := stopService(s, c.stdout); err != nil {
				return err
			}
		}
		if err := s.Delete(); err != nil {
			return err
		}
		_ = eventlog.Remove(serviceName)
		fmt.Fprintln(c.stdout, "removed service corsarr; its data directory is kept")
		return nil
	})
}

func withService(f func(s *mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()
	return f(s)
}

func startService(s *mgr.Service, out io.Writer) error {
	if err := s.Start(); err != nil {
		return err
	}
	if err := waitState(s, svc.Running); err != nil {
		return err
	}
	fmt.Fprintln(out, "corsarr service running; open http://localhost:8090")
	return nil
}

func stopService(s *mgr.Service, out io.Writer) error {
	if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return err
	}
	if err := waitState(s, svc.Stopped); err != nil {
		return err
	}
	fmt.Fprintln(out, "corsarr service stopped")
	return nil
}

func waitState(s *mgr.Service, want svc.State) error {
	deadline := time.Now().Add(serviceWait)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == want {
			return nil
		}
		if st.State == svc.Stopped && want == svc.Running {
			return fmt.Errorf("the service stopped during start (exit code %d); see the corsarr event log entries", st.ServiceSpecificExitCode)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("still %s after %s", stateName(st.State), serviceWait)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func stateName(s svc.State) string {
	switch s {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	case svc.Running:
		return "running"
	case svc.Paused, svc.PausePending, svc.ContinuePending:
		return "paused"
	}
	return "unknown"
}

// serviceStatus prints the state; exit 0 running, 3 not running, 4 not installed.
func (c cli) serviceStatus() int {
	code := 3
	err := withService(func(s *mgr.Service) error {
		st, err := s.Query()
		if err != nil {
			return err
		}
		fmt.Fprintln(c.stdout, "corsarr service:", stateName(st.State))
		if st.State == svc.Running {
			code = 0
		}
		return nil
	})
	switch {
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		fmt.Fprintln(c.stdout, "corsarr service: not installed")
		return 4
	case err != nil:
		fmt.Fprintln(c.stderr, "corsarr service status:", explainServiceErr(err))
		return 1
	}
	return code
}

// serviceRun is what the Service Control Manager starts: serve, logging to the
// event log and to corsarr.log in the data dir.
func (c cli) serviceRun(ctx context.Context) int {
	if ok, err := svc.IsWindowsService(); err != nil || !ok {
		fmt.Fprintln(c.stderr, "corsarr service run is started by the Service Control Manager; run corsarr serve in a console")
		return 2
	}
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return 1
	}
	defer elog.Close()
	events := &lineWriter{emit: func(line string) {
		switch logLevel(line) {
		case 2:
			_ = elog.Error(3, line)
		case 1:
			_ = elog.Warning(2, line)
		default:
			_ = elog.Info(1, line)
		}
	}}
	defer events.Flush()
	c.stdout, c.stderr = events, events
	if cfg, ok := c.load(); ok {
		if f, err := openServiceLog(cfg.Data.Dir); err == nil {
			defer f.Close()
			c.stdout, c.stderr = io.MultiWriter(f, events), io.MultiWriter(f, events)
		} else {
			_ = elog.Warning(2, "cannot open the log file in the data dir: "+err.Error())
		}
	}
	h := &serviceHandler{c: c, ctx: ctx}
	if err := svc.Run(serviceName, h); err != nil {
		_ = elog.Error(3, "service run: "+err.Error())
		return 1
	}
	return h.code
}

type serviceHandler struct {
	c    cli
	ctx  context.Context
	code int
}

// Execute runs serve until the Service Control Manager asks to stop. A
// failing serve reports its exit code as service-specific, so the recovery
// actions restart it.
func (h *serviceHandler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending, WaitHint: uint32(serviceWait / time.Millisecond)}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	done := make(chan int, 1)
	go func() {
		done <- h.c.serve(ctx, func(string) { status <- running })
	}()
	for {
		select {
		case h.code = <-done:
			if h.code != 0 {
				return true, uint32(h.code)
			}
			return false, 0
		case r := <-req:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: uint32((ShutdownGrace + 5*time.Second) / time.Millisecond)}
				cancel()
			}
		}
	}
}
