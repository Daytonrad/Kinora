//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/ScotyLucas/kinora/internal/selfupdate"
	"github.com/ScotyLucas/kinora/internal/server"
)

const serviceName = "Kinora"

func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

type service struct{ cfg server.Config }

// Execute is the Windows service control loop: run Kinora until Stop/Shutdown.
func (s *service) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	// a service has no console: log to <data>\kinora.log (rotated at 10 MB on start)
	os.MkdirAll(s.cfg.DataDir, 0o755)
	logPath := filepath.Join(s.cfg.DataDir, "kinora.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 10<<20 {
		os.Rename(logPath, logPath+".1")
	}
	var w io.Writer = io.Discard
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		defer f.Close()
		w = f
	}
	log := newLogger(w)
	// self-update: put a failed update back before anything opens the database (see internal/selfupdate)
	install := exeDir()
	var exit, probation bool
	if install != "" {
		selfupdate.Enable(install)
		exit, probation = selfupdate.Guard(install, s.cfg.DataDir, server.Version)
	}
	if exit {
		log.Error("the update did not start properly – previous version restored, restarting")
		restartProcess()
	}
	if probation {
		log.Info("new version on probation after the update", "version", server.Version)
		go func() {
			time.Sleep(selfupdate.ProbationTime)
			if selfupdate.Pending(install) {
				log.Error("the new version did not start serving in time, restarting")
				restartProcess()
			}
		}()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(ctx, s.cfg, log) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if errors.Is(err, selfupdate.ErrRestart) {
				to := selfupdate.StagedTag()
				if err := selfupdate.Swap(install, s.cfg.DataDir, server.Version, to); err != nil {
					selfupdate.Log(install, "HIBA: "+err.Error())
					log.Error("update swap failed, restarting the current version", "err", err)
				} else {
					log.Info("update swapped in, restarting", "to", to)
				}
				restartProcess()
			}
			if err != nil {
				log.Error("kinora stopped", "err", err)
				return false, 1
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(20 * time.Second):
				}
				return false, 0
			}
		}
	}
}

func runAsService(cfg server.Config) error { return svc.Run(serviceName, &service{cfg: cfg}) }

// serviceCommand: kinora service install [kinora flags…] | uninstall | start | stop
func serviceCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "használat: kinora service install [--data DIR --addr :8096 …] | uninstall | start | stop")
		return 2
	}
	m, err := mgr.Connect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "a szolgáltatáskezelő nem érhető el (rendszergazdaként futtasd):", err)
		return 1
	}
	defer m.Disconnect()
	switch args[0] {
	case "install":
		exe, _ := os.Executable()
		if s, err := m.OpenService(serviceName); err == nil { // reinstall: replace the arguments
			stopService(s)
			s.Delete()
			s.Close()
			time.Sleep(time.Second)
		}
		s, err := m.CreateService(serviceName, exe, mgr.Config{
			DisplayName:      "Kinora médiaszerver",
			Description:      "Saját médiaszerver filmekhez és sorozatokhoz – http://localhost:8096",
			StartType:        mgr.StartAutomatic,
			DelayedAutoStart: true,
		}, args[1:]...)
		if err != nil {
			fmt.Fprintln(os.Stderr, "a szolgáltatás nem hozható létre:", err)
			return 1
		}
		defer s.Close()
		// restart after a crash; a self-update restarts this way too
		setRecovery(s)
		if err := s.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "a szolgáltatás nem indult el:", err)
			return 1
		}
		fmt.Println("Kinora szolgáltatás telepítve és elindítva:", strings.Join(args[1:], " "))
	case "uninstall":
		s, err := m.OpenService(serviceName)
		if err != nil {
			return 0
		}
		stopService(s)
		err = s.Delete()
		s.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "eltávolítás sikertelen:", err)
			return 1
		}
	case "start", "stop":
		s, err := m.OpenService(serviceName)
		if err != nil {
			fmt.Fprintln(os.Stderr, "a Kinora szolgáltatás nincs telepítve")
			return 1
		}
		defer s.Close()
		if args[0] == "start" {
			err = s.Start()
		} else {
			err = stopService(s)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "ismeretlen parancs:", args[0])
		return 2
	}
	return 0
}

// setRecovery makes the service manager restart Kinora whenever the process ends unexpectedly. The last
// action repeats for every further failure, so a self-update is never left stopped.
func setRecovery(s *mgr.Service) {
	s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 2 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 86400)
	s.SetRecoveryActionsOnNonCrashFailures(true)
}

// restartProcess ends the process without reporting a stop, so the service manager's recovery action
// starts kinora.exe again (the new one after a swap, the old one after a rollback). It first makes sure the
// recovery actions are set: installs made by older versions had only two of them.
func restartProcess() {
	if m, err := mgr.Connect(); err == nil {
		if s, err := m.OpenService(serviceName); err == nil {
			setRecovery(s)
			s.Close()
		}
		m.Disconnect()
	}
	os.Exit(3)
}

// exeDir is the folder of the running kinora.exe.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

func stopService(s *mgr.Service) error {
	st, err := s.Control(svc.Stop)
	if err != nil {
		return nil // not running
	}
	for i := 0; i < 40 && st.State != svc.Stopped; i++ {
		time.Sleep(500 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			break
		}
	}
	return nil
}
