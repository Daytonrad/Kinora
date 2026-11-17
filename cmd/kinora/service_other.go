//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/ScotyLucas/kinora/internal/server"
)

func isWindowsService() bool { return false }

func runAsService(server.Config) error { return nil }

func serviceCommand([]string) int {
	fmt.Fprintln(os.Stderr, "a \"service\" parancs csak Windowson van; Linuxon: install.sh (systemd)")
	return 2
}
